package core

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// scanOne runs a job persisted in scanning with a single delivered file and
// returns it once it is terminal.
func scanOne(t *testing.T, sc *fakeScanner, name string, content []byte) Job {
	t.Helper()
	st := newMemStorage("s")
	st.files[name] = content
	reg := NewRegistry()
	reg.AddEngine(newFakeEngine("e"), EngineOptions{})
	reg.AddStorage(st)
	reg.AddScanner(sc, nil)
	store := NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 50)
	store.Add(&Job{ID: "j_a", Name: "Arch", Engine: "e", Storage: "s", Mode: ModeCopy, State: JobScanning,
		Files: []JobFile{{Path: name, Size: int64(len(content)), Done: int64(len(content)), State: FileDone}}})
	r := NewRunner(reg, store, &recordingNotifier{}, Limits{Jobs: 1, FilesPerJob: 1})
	r.PollEvery = 10 * time.Millisecond
	r.Start(context.Background())
	t.Cleanup(r.Stop)
	deadline := time.Now().Add(5 * time.Second)
	for {
		j, _ := store.Get("j_a")
		if j.State.Terminal() {
			return j
		}
		if time.Now().After(deadline) {
			t.Fatalf("job still %s (error %q)", j.State, j.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOversizedZipIsScannedEntryByEntry(t *testing.T) {
	sc := &fakeScanner{maxSize: 60}
	z := makeZip(t, map[string]string{"setup.exe": "MZ setup", "data/lib.dll": "MZ lib"})
	j := scanOne(t, sc, "Game/big.zip", z)
	if j.State != JobDone || j.Scan.Status != ScanClean {
		t.Fatalf("state %s scan %+v", j.State, j.Scan)
	}
	seen := strings.Join(sc.seenPaths(), " ")
	if !strings.Contains(seen, "Game/big.zip!setup.exe") || !strings.Contains(seen, "Game/big.zip!data/lib.dll") {
		t.Errorf("entries not scanned one by one: %v", sc.seenPaths())
	}
}

func TestInfectedEntryQuarantinesTheWholeArchive(t *testing.T) {
	sc := &fakeScanner{maxSize: 60, bad: map[string]bool{"Game/big.zip!data/crack.exe": true}}
	z := makeZip(t, map[string]string{"setup.exe": "MZ setup", "data/crack.exe": "MZ crack"})
	j := scanOne(t, sc, "Game/big.zip", z)
	if j.State != JobInfected || len(j.Scan.Findings) != 1 {
		t.Fatalf("state %s scan %+v", j.State, j.Scan)
	}
	f := j.Scan.Findings[0]
	if f.Path != "Game/big.zip" || f.Quarantine != ".quarantine/j_a/Game/big.zip" || !strings.Contains(f.Signature, "data/crack.exe") {
		t.Errorf("finding = %+v", f)
	}
}

func TestOversizedEntryInsideArchiveIsSkippedOthersScanned(t *testing.T) {
	sc := &fakeScanner{maxSize: 60}
	z := makeZip(t, map[string]string{"small.exe": "MZ", "huge.bin": strings.Repeat("x", 100)})
	j := scanOne(t, sc, "big.zip", z)
	if j.Scan.Status != ScanSkipped || !strings.Contains(j.Scan.Findings[0].Reason, "huge.bin") {
		t.Fatalf("scan %+v", j.Scan)
	}
	if !strings.Contains(strings.Join(sc.seenPaths(), " "), "big.zip!small.exe") {
		t.Errorf("small entry not scanned: %v", sc.seenPaths())
	}
}

func TestOversized7zIsScanned(t *testing.T) {
	sc := &fakeScanner{maxSize: 5000}
	j := scanOne(t, sc, "pack.7z", fixture(t, "lzma2.7z"))
	if j.State != JobDone || j.Scan.Status != ScanClean || len(sc.seenPaths()) != 10 {
		t.Fatalf("state %s scan %+v seen %v", j.State, j.Scan, sc.seenPaths())
	}
}

func TestUnreadableArchivesAreSkippedNotCrashing(t *testing.T) {
	for name, file := range map[string]string{
		"encrypted.7z":           "aes7z.7z",
		"multivolume.part01.rar": "multivolume.part01.rar", // rardecode panics on it
	} {
		sc := &fakeScanner{maxSize: 200}
		j := scanOne(t, sc, name, fixture(t, file))
		if j.State != JobDone || j.Scan.Status != ScanSkipped || !strings.Contains(j.Scan.Findings[0].Reason, "archive") {
			t.Errorf("%s: state %s scan %+v", name, j.State, j.Scan)
		}
	}
}

func TestOversizedNonArchiveStaysTooLarge(t *testing.T) {
	sc := &fakeScanner{maxSize: 10}
	j := scanOne(t, sc, "movie.mkv", bytes.Repeat([]byte{0x1a, 0x45}, 50))
	if j.Scan.Status != ScanSkipped || j.Scan.Findings[0].Reason != "too large" {
		t.Fatalf("scan %+v", j.Scan)
	}
}
