package core

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// ErrNotMovable is returned when a job's files may not be moved.
var ErrNotMovable = errors.New("only a finished copy job with delivered files can be moved")

// Move relocates the files a finished copy job delivered to subdir of another
// (or the same) storage, keeping their paths relative to the job's subdir; a
// job with a single file puts it directly in subdir. Files marked skipped were
// not delivered by the job and stay where they are. A rename when both sides
// share a filesystem, else a copy then delete. On failure the files already
// moved are put back.
func (r *Runner) Move(ctx context.Context, id, storage, subdir string) (Job, error) {
	j, ok := r.Store.Get(id)
	if !ok {
		return Job{}, ErrJobNotFound
	}
	if j.External || j.State != JobDone || j.Mode != ModeCopy {
		return Job{}, ErrNotMovable
	}
	src, dst := r.Reg.Storage(j.Storage), r.Reg.Storage(storage)
	if src == nil || dst == nil {
		return Job{}, ErrNotFound
	}
	plan := map[int]string{}
	for i, f := range j.Files {
		if f.State == FileDone {
			plan[i] = strings.TrimPrefix(f.Path, j.Subdir+"/")
		}
	}
	if len(plan) == 0 {
		return Job{}, ErrNotMovable
	}
	for i, rel := range plan {
		if len(plan) == 1 {
			rel = path.Base(rel)
		}
		to := path.Join(subdir, rel)
		if to == j.Files[i].Path && src == dst {
			return Job{}, fmt.Errorf("%s: already there", to)
		}
		if ok, err := dst.Exists(ctx, to, -1); err != nil || ok {
			return Job{}, fmt.Errorf("%s: already exists in %s", to, dst.Label())
		}
		plan[i] = to
	}
	move := func(from Storage, fromRel string, to Storage, toRel string) error {
		var err error
		if from == to {
			err = from.Move(ctx, fromRel, toRel)
		} else {
			err = to.Adopt(ctx, filepath.Join(from.Root(), fromRel), toRel)
		}
		if err != nil {
			return err
		}
		return from.Delete(ctx, fromRel) // the file is gone already: this prunes the folders it left empty
	}
	var moved []int
	for i, to := range plan {
		if err := move(src, j.Files[i].Path, dst, to); err != nil {
			for _, k := range moved {
				_ = move(dst, plan[k], src, j.Files[k].Path)
			}
			return Job{}, fmt.Errorf("%s: %w", j.Files[i].Path, err)
		}
		moved = append(moved, i)
	}
	j, _ = r.Store.Update(id, func(j *Job) {
		j.Storage, j.Subdir = storage, subdir
		for i, to := range plan {
			j.Files[i].Path = to
		}
	})
	r.notifyDelivered(j)
	return j, nil
}
