package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newScanRunner(t *testing.T, eng *fakeEngine, st *memStorage, sc *fakeScanner, storages []string, opts EngineOptions) (*Runner, *recordingNotifier) {
	t.Helper()
	reg := NewRegistry()
	reg.AddEngine(eng, opts)
	reg.AddStorage(st)
	reg.AddScanner(sc, storages)
	store := NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 50)
	n := &recordingNotifier{}
	r := NewRunner(reg, store, n, Limits{Jobs: 2, FilesPerJob: 2})
	r.PollEvery = 10 * time.Millisecond
	r.Start(context.Background())
	t.Cleanup(r.Stop)
	return r, n
}

func copyJob() *Job {
	return &Job{Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, Payload: payload()}
}

func lastNotification(t *testing.T, n *recordingNotifier) (string, Level, string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		if k := len(n.calls); k > 0 {
			defer n.mu.Unlock()
			return n.calls[k-1], n.levels[k-1], n.bodies[k-1]
		}
		n.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no notification")
	return "", 0, ""
}

func TestScanCleanJobEndsDone(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{}
	r, n := newScanRunner(t, eng, st, sc, nil, EngineOptions{})
	j := r.Submit(copyJob())
	done := waitState(t, r, j.ID, JobDone)
	if done.Scan == nil || done.Scan.Status != ScanClean || len(done.Scan.Findings) != 0 || done.Scan.ScannedAt.IsZero() {
		t.Fatalf("scan = %+v", done.Scan)
	}
	if got := sc.seenPaths(); len(got) != 2 || got[0] != "Root/a.txt" || got[1] != "Root/b.txt" {
		t.Errorf("scanned = %v", got)
	}
	sc.mu.Lock()
	readB := sc.read["Root/b.txt"]
	sc.mu.Unlock()
	if readB != 20 {
		t.Errorf("scanner read %d bytes of b.txt, want 20", readB)
	}
	if title, level, _ := lastNotification(t, n); title != "Download finished" || level != LevelInfo {
		t.Errorf("notification = %q %v", title, level)
	}
}

func TestScanInfectedQuarantinesAndDropsEngineData(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{bad: map[string]bool{"Root/b.txt": true}}
	r, n := newScanRunner(t, eng, st, sc, nil, EngineOptions{})
	j := r.Submit(copyJob())
	inf := waitState(t, r, j.ID, JobInfected)
	q := ".quarantine/" + j.ID + "/Root/b.txt"
	st.mu.Lock()
	_, stillThere := st.files["Root/b.txt"]
	_, quarantined := st.files[q]
	_, cleanKept := st.files["Root/a.txt"]
	st.mu.Unlock()
	if stillThere || !quarantined || !cleanKept {
		t.Errorf("storage after quarantine: there=%v quarantined=%v cleanKept=%v", stillThere, quarantined, cleanKept)
	}
	if inf.Files[1].State != FileQuarantined || inf.Files[0].State != FileDone {
		t.Errorf("files = %+v", inf.Files)
	}
	f := inf.Scan.Findings
	if inf.Scan.Status != ScanInfected || len(f) != 1 || f[0].Signature != "Eicar-Test-Signature" || f[0].Quarantine != q {
		t.Errorf("scan = %+v", inf.Scan)
	}
	eng.mu.Lock()
	removed := append([]bool(nil), eng.removed...)
	eng.mu.Unlock()
	if len(removed) != 1 || !removed[0] {
		t.Errorf("engine item must be removed with its data, got %v", removed)
	}
	title, level, body := lastNotification(t, n)
	if title != "Malware detected" || level != LevelAlert || !strings.Contains(body, q) {
		t.Errorf("notification = %q %v %q", title, level, body)
	}
	if _, err := r.Retry(j.ID); err == nil {
		t.Error("an infected job must not be retryable")
	}
}

func TestScanErrorEndsDoneWithWarning(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{err: errors.New("dial tcp clamd:3310: connection refused")}
	r, n := newScanRunner(t, eng, st, sc, nil, EngineOptions{RemoveAfterCopy: true})
	j := r.Submit(copyJob())
	done := waitState(t, r, j.ID, JobDone)
	if done.Scan.Status != ScanError || len(done.Scan.Findings) != 2 || !strings.Contains(done.Scan.Findings[0].Reason, "refused") {
		t.Errorf("scan = %+v", done.Scan)
	}
	if title, level, _ := lastNotification(t, n); title != "Download finished, not scanned" || level != LevelFailed {
		t.Errorf("notification = %q %v", title, level)
	}
	eng.mu.Lock()
	defer eng.mu.Unlock()
	if len(eng.removed) != 1 || eng.removed[0] {
		t.Errorf("removeAfterCopy keeps removing without data: %v", eng.removed)
	}
}

func TestScanSkipsLargeFiles(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{maxSize: 15}
	r, _ := newScanRunner(t, eng, st, sc, nil, EngineOptions{})
	j := r.Submit(copyJob())
	done := waitState(t, r, j.ID, JobDone)
	if done.Scan.Status != ScanSkipped || len(done.Scan.Findings) != 1 || done.Scan.Findings[0].Path != "Root/b.txt" {
		t.Errorf("scan = %+v", done.Scan)
	}
}

func TestScanMissingFileIsErrorFinding(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{block: make(chan struct{})}
	r, _ := newScanRunner(t, eng, st, sc, nil, EngineOptions{})
	j := r.Submit(copyJob())
	waitState(t, r, j.ID, JobScanning)
	st.mu.Lock()
	delete(st.files, "Root/b.txt") // the user moved it before the scan reached it
	st.mu.Unlock()
	close(sc.block)
	done := waitState(t, r, j.ID, JobDone)
	if done.Scan.Status != ScanError || len(done.Scan.Findings) != 1 || done.Scan.Findings[0].Path != "Root/b.txt" {
		t.Errorf("scan = %+v", done.Scan)
	}
	if got := sc.seenPaths(); len(got) != 1 || got[0] != "Root/a.txt" {
		t.Errorf("scanned = %v", got)
	}
}

func TestScanOnlyCoveredStoragesAndNeverLinks(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{}
	r, _ := newScanRunner(t, eng, st, sc, []string{"other"}, EngineOptions{})
	j := r.Submit(copyJob())
	if done := waitState(t, r, j.ID, JobDone); done.Scan != nil {
		t.Errorf("storage not covered: scan = %+v", done.Scan)
	}
	l := r.Submit(&Job{Name: "Root", Engine: "e", Mode: ModeLinks, Payload: payload()})
	if done := waitState(t, r, l.ID, JobDone); done.Scan != nil {
		t.Errorf("links mode: scan = %+v", done.Scan)
	}
	if len(sc.seenPaths()) != 0 {
		t.Errorf("scanner was called: %v", sc.seenPaths())
	}
}

func TestRescan(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{err: errors.New("clamd loading")}
	r, _ := newScanRunner(t, eng, st, sc, nil, EngineOptions{})
	j := r.Submit(copyJob())
	waitState(t, r, j.ID, JobDone)
	sc.setErr(nil)
	if _, err := r.Rescan(j.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := r.Store.Get(j.ID)
		if got.State == JobDone && got.Scan != nil && got.Scan.Status == ScanClean {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("rescan did not finish clean: %s %+v", got.State, got.Scan)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := r.Rescan(j.ID); err == nil {
		t.Error("a clean job cannot be rescanned")
	}
	if _, err := r.Rescan("j_nope"); !errors.Is(err, ErrJobNotFound) {
		t.Errorf("unknown job err = %v", err)
	}
}

func TestScanResumesAfterRestart(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{}
	st.files["Root/a.txt"] = []byte("aaaaaaaaaa")
	reg := NewRegistry()
	reg.AddEngine(eng, EngineOptions{})
	reg.AddStorage(st)
	reg.AddScanner(sc, nil)
	store := NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 50)
	store.Add(&Job{ID: "j_resume", Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, State: JobScanning,
		Owned: true, Item: Item{ID: "removed-by-removeAfterCopy"},
		Files: []JobFile{{Path: "Root/a.txt", Size: 10, Done: 10, State: FileDone}}})
	r := NewRunner(reg, store, &recordingNotifier{}, Limits{Jobs: 1, FilesPerJob: 1})
	r.PollEvery = 10 * time.Millisecond
	r.Start(context.Background())
	t.Cleanup(r.Stop)
	done := waitState(t, r, "j_resume", JobDone)
	if done.Scan == nil || done.Scan.Status != ScanClean {
		t.Errorf("scan = %+v (error %q)", done.Scan, done.Error)
	}
}

func TestCancelRefusedWhileScanning(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{block: make(chan struct{})}
	r, _ := newScanRunner(t, eng, st, sc, nil, EngineOptions{})
	j := r.Submit(copyJob())
	waitState(t, r, j.ID, JobScanning)
	if _, err := r.Cancel(context.Background(), j.ID); err == nil {
		t.Error("cancel must be refused during the scan")
	}
	close(sc.block)
	waitState(t, r, j.ID, JobDone)
}

func TestOutcomeMessageCapsLines(t *testing.T) {
	r := &Runner{Lang: "fr"}
	j := Job{Name: "Repack", Scan: &ScanResult{Status: ScanError}}
	for i := 0; i < 15; i++ {
		j.Scan.Findings = append(j.Scan.Findings, ScanFinding{Path: fmt.Sprintf("f%d.bin", i), Status: ScanError, Reason: "refused"})
	}
	title, msg, level := r.outcomeMessage(j)
	lines := strings.Split(msg, "\n")
	if title != "Téléchargement terminé, non analysé" || level != LevelFailed {
		t.Errorf("title/level = %q %v", title, level)
	}
	if len(lines) != 12 || lines[0] != "Repack" || !strings.Contains(lines[11], "+5") {
		t.Errorf("message = %q", msg)
	}
}

// A restart after some files were quarantined must not lose them: the job
// still ends infected and the engine data is still dropped.
func TestScanResumeKeepsQuarantinedFiles(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{}
	item, _ := eng.Add(context.Background(), payload(), AddOpts{})
	st.files[".quarantine/j_r/Root/a.exe"] = []byte("bad")
	st.files[".quarantine/j_r/Root/c.dll"] = []byte("bad")
	st.files["Root/b.bin"] = []byte("good")
	reg := NewRegistry()
	reg.AddEngine(eng, EngineOptions{})
	reg.AddStorage(st)
	reg.AddScanner(sc, nil)
	store := NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 50)
	store.Add(&Job{ID: "j_r", Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, State: JobScanning,
		Owned: true, Item: item,
		Files: []JobFile{
			{Path: "Root/a.exe", Size: 3, Done: 3, State: FileQuarantined},
			{Path: "Root/b.bin", Size: 4, Done: 4, State: FileDone},
			{Path: "Root/c.dll", Size: 3, Done: 3, State: FileQuarantined},
		},
		// a.exe's finding was saved before the restart, c.dll's was not.
		Scan: &ScanResult{Status: ScanInfected, Findings: []ScanFinding{
			{Path: "Root/a.exe", Status: ScanInfected, Signature: "Win.Trojan.A", Quarantine: ".quarantine/j_r/Root/a.exe"},
		}}})
	n := &recordingNotifier{}
	r := NewRunner(reg, store, n, Limits{Jobs: 1, FilesPerJob: 1})
	r.PollEvery = 10 * time.Millisecond
	r.Start(context.Background())
	t.Cleanup(r.Stop)
	inf := waitState(t, r, "j_r", JobInfected)
	byPath := map[string]ScanFinding{}
	for _, f := range inf.Scan.Findings {
		byPath[f.Path] = f
	}
	if f := byPath["Root/a.exe"]; f.Signature != "Win.Trojan.A" || f.Quarantine != ".quarantine/j_r/Root/a.exe" {
		t.Errorf("a.exe finding = %+v", f)
	}
	if f := byPath["Root/c.dll"]; f.Status != ScanInfected || f.Quarantine != ".quarantine/j_r/Root/c.dll" {
		t.Errorf("c.dll finding = %+v", f)
	}
	if _, ok := byPath["Root/b.bin"]; ok || len(inf.Scan.Findings) != 2 {
		t.Errorf("findings = %+v", inf.Scan.Findings)
	}
	eng.mu.Lock()
	removed := append([]bool(nil), eng.removed...)
	eng.mu.Unlock()
	if len(removed) != 1 || !removed[0] {
		t.Errorf("engine data must be dropped, got %v", removed)
	}
	if title, level, _ := lastNotification(t, n); level != LevelAlert {
		t.Errorf("notification = %q %v", title, level)
	}
}

// Rescan starts from a clean slate: findings of the previous scan are not
// carried over.
func TestRescanDropsPreviousFindings(t *testing.T) {
	eng, st, sc := newFakeEngine("e"), newMemStorage("s"), &fakeScanner{err: errors.New("clamd loading")}
	r, _ := newScanRunner(t, eng, st, sc, nil, EngineOptions{})
	j := r.Submit(copyJob())
	waitState(t, r, j.ID, JobDone)
	sc.setErr(nil)
	sc.mu.Lock()
	sc.block = make(chan struct{})
	sc.mu.Unlock()
	if _, err := r.Rescan(j.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Store.Get(j.ID); got.Scan != nil {
		t.Errorf("rescan must clear the previous result, got %+v", got.Scan)
	}
	close(sc.block)
}
