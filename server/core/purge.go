package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
)

// ErrNotPurgeable is returned when a job's files may not be deleted.
var ErrNotPurgeable = errors.New("only a cancelled or infected job's files can be deleted")

// Purge deletes what a cancelled or infected job left behind: the files it
// delivered (quarantined ones under QuarantineDir/<id>/), the engine item it
// created, then the job itself. Files marked skipped were already on the
// storage before the job and are never touched. When a file cannot be
// deleted the job is kept, so the purge can be retried.
func (r *Runner) Purge(ctx context.Context, id string) error {
	j, ok := r.Store.Get(id)
	if !ok {
		return ErrJobNotFound
	}
	if j.External || (j.State != JobCancelled && j.State != JobInfected) {
		return ErrNotPurgeable
	}
	if st := r.Reg.Storage(j.Storage); st != nil {
		var failed []error
		for _, f := range j.Files {
			rel := f.Path
			switch f.State {
			case FileQuarantined:
				rel = path.Join(QuarantineDir, j.ID, f.Path)
			case FileDone:
			default:
				continue
			}
			if err := st.Delete(ctx, rel); err != nil {
				failed = append(failed, fmt.Errorf("%s: %w", rel, err))
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("delete files: %w", errors.Join(failed...))
		}
	}
	if j.Owned && j.Item.ID != "" {
		if eng := r.Reg.Engine(j.Engine); eng != nil {
			if err := eng.Remove(ctx, j.Item, true); err != nil && !errors.Is(err, ErrNotFound) {
				slog.Warn("purge: remove engine item", "job", id, "err", err)
			}
		}
	}
	r.Store.Remove(id)
	r.invalidateExternal()
	return nil
}
