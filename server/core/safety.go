package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// SafeRel joins rel under root and returns the absolute path, or an error
// when rel is empty, absolute, or escapes the root through ".." or a symlink.
// Both "/" and "\" are treated as separators on every platform.
func SafeRel(root, rel string) (string, error) {
	if rel == "" || strings.ContainsRune(rel, 0) {
		return "", errors.New("empty or invalid path")
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	if strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) || filepath.VolumeName(filepath.FromSlash(rel)) != "" {
		return "", fmt.Errorf("absolute path not allowed: %q", rel)
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return "", fmt.Errorf("path escapes root: %q", rel)
		}
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %q", rel)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	full := filepath.Join(absRoot, clean)
	if err := verifyUnder(absRoot, full); err != nil {
		return "", err
	}
	return full, nil
}

// verifyUnder resolves symlinks of the deepest existing ancestor of path and
// checks it still sits under root (which must exist).
func verifyUnder(root, path string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("storage root: %w", err)
	}
	probe := path
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return errors.New("no existing ancestor")
		}
		probe = parent
	}
	realProbe, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return err
	}
	relPath, err := filepath.Rel(realRoot, realProbe)
	if err != nil || relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes root through a link: %q", path)
	}
	return nil
}

// MaxSubdirLen bounds a user-supplied subfolder name.
const MaxSubdirLen = 120

// SafeSubdir validates a user-supplied subfolder name: letters, digits,
// space and - _ . ( ) [ ] only, at most MaxSubdirLen runes, no leading dot.
// An empty name is allowed and means "no subfolder".
func SafeSubdir(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if len([]rune(s)) > MaxSubdirLen {
		return "", fmt.Errorf("subfolder name longer than %d characters", MaxSubdirLen)
	}
	if s[0] == '.' {
		return "", errors.New("subfolder name cannot start with a dot")
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || strings.ContainsRune("-_.()[]", r) {
			continue
		}
		return "", fmt.Errorf("subfolder name contains an invalid character %q", r)
	}
	return s, nil
}
