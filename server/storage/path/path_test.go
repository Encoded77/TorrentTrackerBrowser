package path

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func opener(content string, offsets *[]int64) func(ctx context.Context, off int64) (io.ReadCloser, error) {
	return func(ctx context.Context, off int64) (io.ReadCloser, error) {
		*offsets = append(*offsets, off)
		if off > int64(len(content)) {
			return nil, errors.New("offset past end")
		}
		return io.NopCloser(strings.NewReader(content[off:])), nil
	}
}

func TestPutWritesAtomically(t *testing.T) {
	root := t.TempDir()
	s, err := New("dl", "", root)
	if err != nil {
		t.Fatal(err)
	}
	if s.Label() != "dl" || s.Root() != root {
		t.Errorf("label/root = %q/%q", s.Label(), s.Root())
	}
	ctx := context.Background()
	var offsets []int64
	var progress []int64
	err = s.Put(ctx, "Dune/dune.mkv", 12, opener("hello world!", &offsets), func(n int64) { progress = append(progress, n) })
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "Dune", "dune.mkv"))
	if err != nil || string(b) != "hello world!" {
		t.Fatalf("final file: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(root, "Dune", "dune.mkv.part")); !errors.Is(err, os.ErrNotExist) {
		t.Error(".part still present after rename")
	}
	if len(offsets) != 1 || offsets[0] != 0 || progress[len(progress)-1] != 12 {
		t.Errorf("offsets=%v progress=%v", offsets, progress)
	}
	ok, err := s.Exists(ctx, "Dune/dune.mkv", 12)
	if err != nil || !ok {
		t.Errorf("Exists = %v %v", ok, err)
	}
	if ok, _ := s.Exists(ctx, "Dune/dune.mkv", 5); ok {
		t.Error("Exists with a wrong size must be false")
	}
	if ok, _ := s.Exists(ctx, "nope", 1); ok {
		t.Error("missing file reported present")
	}
}

func TestPutResumesFromTruncatedPart(t *testing.T) {
	root := t.TempDir()
	s, _ := New("dl", "DL", root)
	ctx := context.Background()
	os.MkdirAll(filepath.Join(root, "d"), 0o755)
	os.WriteFile(filepath.Join(root, "d", "f.bin.part"), []byte("hello"), 0o644)
	if n, err := s.Partial(ctx, "d/f.bin"); err != nil || n != 5 {
		t.Fatalf("Partial = %d %v", n, err)
	}
	var offsets []int64
	if err := s.Put(ctx, "d/f.bin", 12, opener("hello world!", &offsets), nil); err != nil {
		t.Fatal(err)
	}
	if len(offsets) != 1 || offsets[0] != 5 {
		t.Errorf("resume offset = %v, want [5]", offsets)
	}
	b, _ := os.ReadFile(filepath.Join(root, "d", "f.bin"))
	if string(b) != "hello world!" {
		t.Errorf("content = %q", b)
	}
	if n, _ := s.Partial(ctx, "d/f.bin"); n != 0 {
		t.Error("Partial after completion should be 0")
	}
}

func TestPutShortReadKeepsPart(t *testing.T) {
	root := t.TempDir()
	s, _ := New("dl", "DL", root)
	ctx := context.Background()
	var offsets []int64
	err := s.Put(ctx, "f.bin", 100, opener("short", &offsets), nil)
	if err == nil {
		t.Fatal("expected a short download error")
	}
	if n, _ := s.Partial(ctx, "f.bin"); n != 5 {
		t.Errorf("Partial after short read = %d, want 5", n)
	}
	if err := s.Remove(ctx, "f.bin"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Partial(ctx, "f.bin"); n != 0 {
		t.Error("Remove should delete the .part")
	}
	if err := s.Remove(ctx, "f.bin"); err != nil {
		t.Error("Remove of a missing .part must be a no-op")
	}
}

func TestPutOversizedPartRestarts(t *testing.T) {
	root := t.TempDir()
	s, _ := New("dl", "DL", root)
	os.WriteFile(filepath.Join(root, "f.bin.part"), []byte("way too long content"), 0o644)
	var offsets []int64
	if err := s.Put(context.Background(), "f.bin", 5, opener("hello", &offsets), nil); err != nil {
		t.Fatal(err)
	}
	if offsets[0] != 0 {
		t.Errorf("oversized .part must restart from 0, got %v", offsets)
	}
	b, _ := os.ReadFile(filepath.Join(root, "f.bin"))
	if string(b) != "hello" {
		t.Errorf("content = %q", b)
	}
}

func TestPutRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	s, _ := New("dl", "DL", root)
	var offsets []int64
	for _, rel := range []string{"../x", "/abs", "", "a/../../b"} {
		if err := s.Put(context.Background(), rel, 1, opener("x", &offsets), nil); err == nil {
			t.Errorf("%q accepted", rel)
		}
	}
	if len(offsets) != 0 {
		t.Error("open must not be called for rejected paths")
	}
}

func TestAdoptAcrossDirs(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	s, _ := New("dl", "DL", root)
	src := filepath.Join(other, "movie.mkv")
	os.WriteFile(src, []byte("data"), 0o644)
	if err := s.Adopt(context.Background(), src, "Films/movie.mkv"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "Films", "movie.mkv"))
	if err != nil || string(b) != "data" {
		t.Fatalf("adopted file: %q %v", b, err)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Error("source should be gone after adopt (rename or copy+remove)")
	}
	// Adopting a file already at its destination is a no-op success.
	if err := s.Adopt(context.Background(), filepath.Join(root, "Films", "movie.mkv"), "Films/movie.mkv"); err != nil {
		t.Errorf("adopt in place: %v", err)
	}
	if err := s.Adopt(context.Background(), src, "../escape"); err == nil {
		t.Error("adopt outside root accepted")
	}
}

func TestFreeAndNew(t *testing.T) {
	root := t.TempDir()
	s, _ := New("dl", "DL", root)
	free, err := s.Free(context.Background())
	if err != nil || free <= 0 {
		t.Errorf("Free = %d %v", free, err)
	}
	if _, err := New("x", "X", filepath.Join(root, "missing", "deeper")); err == nil {
		t.Error("root without an existing parent accepted")
	}
	if _, err := New("x", "X", ""); err == nil {
		t.Error("empty root accepted")
	}
}

func TestNewCreatesMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "manual")
	s, err := New("m", "", root)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(s.Root()); err != nil || !st.IsDir() {
		t.Fatalf("root not created: %v", err)
	}
	if _, err := New("x", "", filepath.Join(t.TempDir(), "no", "parent")); err == nil {
		t.Fatal("missing parent must still fail")
	}
}
