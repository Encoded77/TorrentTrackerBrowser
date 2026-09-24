package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// External lists engine-side items this app did not create, rendered as
// jobs with External=true and id "x_<engine>_<itemId>". Cached for 3 s.
func (r *Runner) External(ctx context.Context) []Job {
	r.extMu.Lock()
	defer r.extMu.Unlock()
	if time.Since(r.extAt) < 3*time.Second && r.extJobs != nil {
		return r.extJobs
	}
	owned := map[string]bool{}
	for _, j := range r.Store.List() {
		if j.Item.ID != "" {
			owned[j.Engine+"\x00"+j.Item.ID] = true
		}
	}
	out := []Job{}
	for _, eng := range r.Reg.Engines {
		items, err := eng.List(ctx)
		if err != nil {
			slog.Warn("external: list", "engine", eng.ID(), "err", err)
			continue
		}
		for _, it := range items {
			if owned[eng.ID()+"\x00"+it.ID] {
				continue
			}
			out = append(out, r.externalJob(ctx, eng, it))
		}
	}
	r.extJobs, r.extAt = out, time.Now()
	return out
}

func (r *Runner) invalidateExternal() {
	r.extMu.Lock()
	r.extJobs = nil
	r.extMu.Unlock()
}

// ExternalID builds the queue id of an engine item.
func ExternalID(engine, itemID string) string { return "x_" + engine + "_" + itemID }

func (r *Runner) externalJob(ctx context.Context, eng Engine, it Item) Job {
	j := Job{ID: ExternalID(eng.ID(), it.ID), Name: it.Name, InfoHash: it.InfoHash, Engine: eng.ID(),
		Mode: ModeLinks, State: JobQueued, External: true, Item: it, Files: []JobFile{}}
	st, err := eng.Status(ctx, it)
	if err != nil {
		j.State, j.Error = JobFailed, err.Error()
		return j
	}
	j.Progress, j.Speed, j.ETA, j.Error = st.Progress, st.Speed, st.ETA, st.Error
	switch st.State {
	case ItemFetching:
		j.State = JobFetching
	case ItemReady:
		j.State, j.Progress = JobReady, 1
	case ItemFailed:
		j.State = JobFailed
	case ItemWaitingSelection:
		j.State = JobWaitingSelection
	}
	files, err := eng.Files(ctx, it)
	if err != nil {
		return j
	}
	for _, f := range files {
		jf := JobFile{Path: f.Path, Size: f.Size, State: FilePending, Done: int64(float64(f.Size) * st.Progress)}
		if st.State == ItemReady {
			jf.State, jf.Done, jf.URL = FileDone, f.Size, FileURL(j.ID, f.Path)
		}
		j.Files = append(j.Files, jf)
	}
	return j
}

// externalItem resolves "x_<engine>_<itemId>" to its engine and item.
func (r *Runner) externalItem(ctx context.Context, id string) (Engine, Item, error) {
	for _, eng := range r.Reg.Engines {
		prefix := "x_" + eng.ID() + "_"
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		itemID := strings.TrimPrefix(id, prefix)
		items, err := eng.List(ctx)
		if err != nil {
			return nil, Item{}, err
		}
		for _, it := range items {
			if it.ID == itemID {
				return eng, it, nil
			}
		}
		return nil, Item{}, ErrJobNotFound
	}
	return nil, Item{}, ErrJobNotFound
}

// SendOpts describe where an external item goes.
type SendOpts struct {
	Storage string
	Subdir  string
	Files   []int
	Mode    Mode
}

// Send creates a real job, starting in copying, for an external item.
func (r *Runner) Send(ctx context.Context, extID string, o SendOpts) (Job, error) {
	eng, it, err := r.externalItem(ctx, extID)
	if err != nil {
		return Job{}, err
	}
	if o.Mode == "" {
		o.Mode = ModeCopy
	}
	if o.Mode != ModeLinks && r.Reg.Storage(o.Storage) == nil {
		return Job{}, fmt.Errorf("storage %q is not configured", o.Storage)
	}
	j := &Job{Name: it.Name, InfoHash: it.InfoHash, Engine: eng.ID(), Storage: o.Storage, Subdir: o.Subdir,
		Mode: o.Mode, State: JobCopying, Item: it, Owned: false, Selection: o.Files}
	if o.Mode == ModeLinks {
		j.Storage, j.State = "", JobQueued
	}
	out := r.Submit(j)
	r.invalidateExternal()
	return out, nil
}

// ResolveFile finds the engine file behind a job (own or external) and a
// torrent-relative path, for the stream route.
func (r *Runner) ResolveFile(ctx context.Context, jobID, filePath string) (Engine, File, error) {
	var eng Engine
	var it Item
	if strings.HasPrefix(jobID, "x_") {
		e, item, err := r.externalItem(ctx, jobID)
		if err != nil {
			return nil, File{}, err
		}
		eng, it = e, item
	} else {
		j, ok := r.Store.Get(jobID)
		if !ok {
			return nil, File{}, ErrJobNotFound
		}
		if j.Item.ID == "" {
			return nil, File{}, errors.New("job has no engine item")
		}
		eng, it = r.Reg.Engine(j.Engine), j.Item
		if eng == nil {
			return nil, File{}, fmt.Errorf("engine %q is not configured", j.Engine)
		}
	}
	files, err := eng.Files(ctx, it)
	if err != nil {
		return nil, File{}, err
	}
	for _, f := range files {
		if f.Path == filePath {
			return eng, f, nil
		}
	}
	return nil, File{}, ErrJobNotFound
}
