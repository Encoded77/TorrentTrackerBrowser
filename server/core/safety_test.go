package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSafeRelAcceptsNested(t *testing.T) {
	root := t.TempDir()
	got, err := SafeRel(root, "Dune/dune.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "Dune", "dune.mkv"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := SafeRel(root, "a\\b.txt"); err != nil {
		t.Errorf("backslash separators should be accepted: %v", err)
	}
}

func TestSafeRelRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	bad := []string{"", "..", "../x", "a/../../x", "/etc/passwd", "a/..\\..\\x", "..\\x", "a\x00b", "."}
	if runtime.GOOS == "windows" {
		bad = append(bad, `C:\Windows\x`, `\\server\share\x`)
	}
	for _, rel := range bad {
		if p, err := SafeRel(root, rel); err == nil {
			t.Errorf("%q: accepted as %q", rel, p)
		}
	}
	// "a/../b" stays under root but is rejected for containing "..".
	if _, err := SafeRel(root, "a/../b"); err == nil {
		t.Error("a/../b should be rejected")
	}
}

func TestSafeRelRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if _, err := SafeRel(root, "link/file.txt"); err == nil {
		t.Error("path through a symlink leaving the root should be rejected")
	}
	// A symlink that stays inside the root is fine.
	inside := filepath.Join(root, "inside")
	os.Mkdir(inside, 0o755)
	if err := os.Symlink(inside, filepath.Join(root, "link2")); err == nil {
		if _, err := SafeRel(root, "link2/file.txt"); err != nil {
			t.Errorf("internal symlink rejected: %v", err)
		}
	}
}

func TestSafeSubdir(t *testing.T) {
	ok := []string{"", "Dune", "Dune (2024) [1080p]", "  Séries TV  ", "a-b_c.d"}
	for _, s := range ok {
		if _, err := SafeSubdir(s); err != nil {
			t.Errorf("%q rejected: %v", s, err)
		}
	}
	bad := []string{".hidden", "a/b", "a\\b", "a:b", "x*y", strings.Repeat("a", 121), "a\nb"}
	for _, s := range bad {
		if _, err := SafeSubdir(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
	if got, _ := SafeSubdir("  Dune  "); got != "Dune" {
		t.Errorf("trim: got %q", got)
	}
}
