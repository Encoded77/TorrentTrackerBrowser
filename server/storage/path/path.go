// Package path implements core.Storage over one directory root.
package path

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

func init() {
	core.RegisterStorage("path", func(cfg map[string]any) (core.Storage, error) {
		return New(core.StringOpt(cfg, "id"), core.StringOpt(cfg, "label"), core.StringOpt(cfg, "root"))
	})
}

// Storage is a directory root; every path goes through core.SafeRel.
type Storage struct {
	id, label, root string
}

// New validates the root, creating it when its parent exists (an empty
// download folder is routinely removed by tidying scripts; that must not keep
// the whole app from starting).
func New(id, label, root string) (*Storage, error) {
	if root == "" {
		return nil, errors.New("missing root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if errors.Is(err, os.ErrNotExist) {
		if _, perr := os.Stat(filepath.Dir(abs)); perr == nil {
			if err = os.Mkdir(abs, 0o775); err == nil {
				st, err = os.Stat(abs)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("root: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("root %q is not a directory", abs)
	}
	if label == "" {
		label = id
	}
	return &Storage{id: id, label: label, root: abs}, nil
}

func (s *Storage) ID() string    { return s.id }
func (s *Storage) Label() string { return s.label }
func (s *Storage) Root() string  { return s.root }

// Free reports the free bytes of the filesystem holding the root.
func (s *Storage) Free(ctx context.Context) (int64, error) { return freeBytes(s.root) }

// Partial returns the size of <rel>.part, 0 when absent.
func (s *Storage) Partial(ctx context.Context, rel string) (int64, error) {
	dst, err := core.SafeRel(s.root, rel)
	if err != nil {
		return 0, err
	}
	st, err := os.Stat(dst + ".part")
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// Put streams into <rel>.part, resuming from its current size, fsyncs and
// renames it to rel. progress receives the cumulative byte count.
func (s *Storage) Put(ctx context.Context, rel string, size int64, open core.OpenAt, progress func(int64)) error {
	dst, err := core.SafeRel(s.root, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	part := dst + ".part"
	var offset int64
	if st, err := os.Stat(part); err == nil {
		offset = st.Size()
		if size >= 0 && offset > size {
			// A .part larger than the file cannot be a prefix of it: restart.
			if err := os.Remove(part); err != nil {
				return err
			}
			offset = 0
		}
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if offset < size || size < 0 {
		rc, err := open(ctx, offset)
		if err != nil {
			return err
		}
		written, err := copyWithProgress(ctx, f, rc, offset, progress)
		rc.Close()
		if err != nil {
			return err
		}
		if size >= 0 && written != size {
			return core.Retryable(fmt.Errorf("short download: %d of %d bytes", written, size))
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(part, dst)
}

func copyWithProgress(ctx context.Context, w io.Writer, r io.Reader, start int64, progress func(int64)) (int64, error) {
	buf := make([]byte, 1<<20)
	total := start
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, rerr := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
			if progress != nil {
				progress(total)
			}
		}
		if rerr == io.EOF {
			return total, nil
		}
		if rerr != nil {
			return total, rerr
		}
	}
}

// Adopt moves localPath to rel: rename, else hardlink, else copy and remove.
func (s *Storage) Adopt(ctx context.Context, localPath, rel string) error {
	dst, err := core.SafeRel(s.root, rel)
	if err != nil {
		return err
	}
	src, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}
	if src == dst {
		_, err := os.Stat(dst)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := s.Put(ctx, rel, -1, func(ctx context.Context, off int64) (io.ReadCloser, error) {
		if _, err := in.Seek(off, io.SeekStart); err != nil {
			return nil, err
		}
		return io.NopCloser(in), nil
	}, nil); err != nil {
		return err
	}
	return os.Remove(src)
}

// Exists reports whether rel is present with the given size (size < 0
// means any size).
func (s *Storage) Exists(ctx context.Context, rel string, size int64) (bool, error) {
	dst, err := core.SafeRel(s.root, rel)
	if err != nil {
		return false, err
	}
	st, err := os.Stat(dst)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !st.IsDir() && (size < 0 || st.Size() == size), nil
}

// Remove deletes the .part file of rel (cancel cleanup).
func (s *Storage) Remove(ctx context.Context, rel string) error {
	dst, err := core.SafeRel(s.root, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(dst + ".part"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
