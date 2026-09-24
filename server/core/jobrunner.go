package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Runner executes jobs: a bounded worker pool over the JobStore, the
// engine/storage pipeline, cancel/retry/delete semantics and the external
// item view.
type Runner struct {
	Reg       *Registry
	Store     *JobStore
	Notifier  Notifier
	Lang      string // language of notification titles: "en" or "fr"
	Limits    Limits
	PollEvery time.Duration

	mu      sync.Mutex
	running map[string]context.CancelFunc
	wake    chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	extMu   sync.Mutex
	extAt   time.Time
	extJobs []Job
}

// NewRunner wires a runner; call Start to begin scheduling.
func NewRunner(reg *Registry, store *JobStore, n Notifier, limits Limits) *Runner {
	return &Runner{Reg: reg, Store: store, Notifier: n, Limits: limits, PollEvery: 3 * time.Second,
		running: map[string]context.CancelFunc{}, wake: make(chan struct{}, 1)}
}

// Start reconciles persisted jobs with the engines and starts the scheduler.
func (r *Runner) Start(ctx context.Context) {
	r.ctx, r.cancel = context.WithCancel(ctx)
	r.reconcile(r.ctx)
	r.wg.Add(1)
	go r.loop()
}

// Stop cancels every running job goroutine and waits for them.
func (r *Runner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

// Wake asks the scheduler to look for startable jobs.
func (r *Runner) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runner) loop() {
	defer r.wg.Done()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-r.wake:
		case <-t.C:
		}
		r.schedule()
	}
}

// schedule starts non-terminal jobs that are not running, oldest first,
// while a worker slot is free.
func (r *Runner) schedule() {
	jobs := r.Store.List()
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(jobs) - 1; i >= 0 && len(r.running) < r.Limits.Jobs; i-- {
		j := jobs[i]
		if j.State.Terminal() || r.running[j.ID] != nil {
			continue
		}
		ctx, cancel := context.WithCancel(r.ctx)
		r.running[j.ID] = cancel
		r.wg.Add(1)
		go func(id string) {
			defer r.wg.Done()
			r.run(ctx, id)
			r.mu.Lock()
			delete(r.running, id)
			r.mu.Unlock()
			r.Wake()
		}(j.ID)
	}
}

// Submit stores a new job and wakes the scheduler.
func (r *Runner) Submit(j *Job) Job {
	now := time.Now().UTC()
	j.ID = NewJobID()
	j.CreatedAt, j.UpdatedAt = now, now
	if j.State == "" {
		j.State = JobQueued
	}
	if j.Files == nil {
		j.Files = []JobFile{}
	}
	r.Store.Add(j)
	r.Wake()
	return *j
}

// Cancel stops a job: the running goroutine cleans up, otherwise we do it.
func (r *Runner) Cancel(ctx context.Context, id string) (Job, error) {
	j, ok := r.Store.Get(id)
	if !ok {
		return Job{}, ErrJobNotFound
	}
	if j.State.Terminal() {
		return j, fmt.Errorf("job is already %s", j.State)
	}
	j, _ = r.Store.SetState(id, JobCancelled)
	r.mu.Lock()
	cancel := r.running[id]
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	} else {
		r.cleanupCancelled(ctx, j)
	}
	return j, nil
}

// Retry requeues a failed retryable job.
func (r *Runner) Retry(id string) (Job, error) {
	j, ok := r.Store.Get(id)
	if !ok {
		return Job{}, ErrJobNotFound
	}
	if j.State != JobFailed || !j.Retryable {
		return j, errors.New("job is not retryable")
	}
	j, _ = r.Store.Update(id, func(j *Job) {
		j.State, j.Error, j.Retryable, j.Speed, j.ETA = JobQueued, "", false, 0, 0
		for i := range j.Files {
			if j.Files[i].State == FileFailed || j.Files[i].State == FileCopying {
				j.Files[i].State = FilePending
			}
		}
	})
	r.Wake()
	return j, nil
}

// Delete removes a job (cancelling it first) and its engine item when this
// app created it. deleteFiles is passed to the engine (client engines only).
func (r *Runner) Delete(ctx context.Context, id string, deleteFiles bool) error {
	if strings.HasPrefix(id, "x_") {
		eng, item, err := r.externalItem(ctx, id)
		if err != nil {
			return err
		}
		r.invalidateExternal()
		return eng.Remove(ctx, item, deleteFiles)
	}
	j, ok := r.Store.Get(id)
	if !ok {
		return ErrJobNotFound
	}
	r.mu.Lock()
	cancel := r.running[id]
	r.mu.Unlock()
	r.Store.Remove(id)
	if cancel != nil {
		cancel()
	}
	if j.Owned && j.Item.ID != "" {
		if eng := r.Reg.Engine(j.Engine); eng != nil {
			if err := eng.Remove(ctx, j.Item, deleteFiles); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
	}
	r.invalidateExternal()
	return nil
}

// cleanupCancelled removes partial files and the engine item of a cancelled
// job. Partial engine data is deleted; a finished fetch keeps its files on
// client engines.
func (r *Runner) cleanupCancelled(ctx context.Context, j Job) {
	if _, still := r.Store.Get(j.ID); !still {
		return
	}
	if st := r.Reg.Storage(j.Storage); st != nil {
		for _, f := range j.Files {
			if f.State == FileCopying {
				_ = st.Remove(ctx, f.Path)
			}
		}
	}
	if j.Owned && j.Item.ID != "" {
		if eng := r.Reg.Engine(j.Engine); eng != nil {
			deleteFiles := j.State != JobCopying && j.State != JobReady
			if err := eng.Remove(ctx, j.Item, deleteFiles); err != nil && !errors.Is(err, ErrNotFound) {
				slog.Warn("job: remove engine item after cancel", "job", j.ID, "err", err)
			}
		}
	}
	r.invalidateExternal()
}

// reconcile adapts persisted jobs to the world after a restart.
func (r *Runner) reconcile(ctx context.Context) {
	listed := map[string]map[string]bool{}
	for _, j := range r.Store.List() {
		if j.State.Terminal() {
			continue
		}
		if j.Item.ID == "" {
			r.Store.Update(j.ID, func(j *Job) { j.State = JobQueued })
			continue
		}
		eng := r.Reg.Engine(j.Engine)
		if eng == nil {
			r.fail(j.ID, fmt.Errorf("engine %q is no longer configured", j.Engine), false)
			continue
		}
		ids, seen := listed[j.Engine]
		if !seen {
			items, err := eng.List(ctx)
			if err != nil {
				slog.Warn("reconcile: list engine items", "engine", j.Engine, "err", err)
				continue
			}
			ids = map[string]bool{}
			for _, it := range items {
				ids[it.ID] = true
			}
			listed[j.Engine] = ids
		}
		if !ids[j.Item.ID] {
			r.Store.Update(j.ID, func(j *Job) { j.Item = Item{} })
			r.fail(j.ID, errors.New("engine item vanished while the app was stopped"), true)
			continue
		}
		if j.State == JobAdding || j.State == JobQueued {
			r.Store.Update(j.ID, func(j *Job) { j.State = JobFetching })
		}
	}
}

// run is the pipeline for one job. It returns when the job is terminal.
func (r *Runner) run(ctx context.Context, id string) {
	err := r.pipeline(ctx, id)
	j, ok := r.Store.Get(id)
	if !ok {
		return
	}
	if ctx.Err() != nil || j.State == JobCancelled {
		if j.State != JobCancelled {
			// Shutdown, not a user cancel: leave the job resumable.
			return
		}
		r.cleanupCancelled(context.Background(), j)
		return
	}
	if err != nil {
		r.fail(id, err, IsRetryable(err))
		r.Notifier.Notify(context.Background(), r.notifyTitle(true), j.Name+": "+err.Error(), true)
	}
}

func (r *Runner) fail(id string, err error, retryable bool) {
	r.Store.Update(id, func(j *Job) {
		if j.State.Terminal() {
			return
		}
		j.State, j.Error, j.Retryable, j.Speed, j.ETA = JobFailed, err.Error(), retryable, 0, 0
		for i := range j.Files {
			if j.Files[i].State == FileCopying {
				j.Files[i].State = FileFailed
			}
		}
	})
}

// errStopped aborts the pipeline once the job reached a terminal state
// (cancelled from the API) while a step was running.
var errStopped = errors.New("job stopped")

// setState advances the job when the transition is allowed, silently keeps
// the current state otherwise (a job sent from an external item starts in
// copying and skips ready), and reports errStopped on a terminal job.
func (r *Runner) setState(id string, to JobState) error {
	stopped := false
	r.Store.Update(id, func(j *Job) {
		if j.State.Terminal() {
			stopped = true
			return
		}
		if j.State != to && CanTransition(j.State, to) {
			j.State = to
		}
	})
	if stopped {
		return errStopped
	}
	return nil
}

func (r *Runner) pipeline(ctx context.Context, id string) error {
	j, ok := r.Store.Get(id)
	if !ok {
		return ErrJobNotFound
	}
	eng := r.Reg.Engine(j.Engine)
	if eng == nil {
		return fmt.Errorf("engine %q is not configured", j.Engine)
	}
	var st Storage
	if j.Mode != ModeLinks {
		if st = r.Reg.Storage(j.Storage); st == nil {
			return fmt.Errorf("storage %q is not configured", j.Storage)
		}
	}

	if j.Item.ID == "" {
		if err := r.setState(id, JobAdding); err != nil {
			return err
		}
		opts := AddOpts{Files: j.Selection, Label: "ttb"}
		if eng.Caps().SavePath && j.Mode == ModeAdopt {
			opts.SavePath = filepath.Join(st.Root(), j.Subdir)
		}
		item, err := eng.Add(ctx, j.Payload, opts)
		if err != nil {
			return fmt.Errorf("add to %s: %w", eng.Name(), err)
		}
		j, _ = r.Store.Update(id, func(j *Job) {
			j.Item, j.Owned = item, true
			if j.InfoHash == "" {
				j.InfoHash = item.InfoHash
			}
			if j.Name == "" {
				j.Name = item.Name
			}
		})
		r.invalidateExternal()
	}

	if err := r.waitReady(ctx, id, eng, j); err != nil {
		return err
	}
	j, _ = r.Store.Get(id)
	if j.State.Terminal() {
		return nil
	}
	files, err := eng.Files(ctx, j.Item)
	if err != nil {
		return fmt.Errorf("list files: %w", err)
	}
	files = selectFiles(files, j.Payload, j.Selection)
	if len(files) == 0 {
		return errors.New("no file to deliver")
	}
	if err := r.setState(id, JobReady); err != nil {
		return err
	}
	switch j.Mode {
	case ModeLinks:
		r.Store.Update(id, func(j *Job) {
			j.Files = nil
			for _, f := range files {
				j.Files = append(j.Files, JobFile{Path: f.Path, Size: f.Size, Done: f.Size, State: FileDone, URL: FileURL(j.ID, f.Path)})
			}
			j.Progress = 1
		})
		return r.finish(id)
	case ModeAdopt:
		if err := r.setState(id, JobCopying); err != nil {
			return err
		}
		if err := r.adopt(ctx, id, st, j.Subdir, files); err != nil {
			return err
		}
	default:
		if err := r.setState(id, JobCopying); err != nil {
			return err
		}
		if err := r.copy(ctx, id, st, j.Subdir, files); err != nil {
			return err
		}
	}
	if r.Reg.EngineOptions(j.Engine).RemoveAfterCopy && j.Owned {
		if err := eng.Remove(ctx, j.Item, false); err != nil && !errors.Is(err, ErrNotFound) {
			slog.Warn("job: remove engine item after copy", "job", id, "err", err)
		}
		r.invalidateExternal()
	}
	return r.finish(id)
}

func (r *Runner) finish(id string) error {
	j, ok := r.Store.Update(id, func(j *Job) {
		if CanTransition(j.State, JobDone) {
			j.State, j.Progress, j.Speed, j.ETA = JobDone, 1, 0, 0
		}
	})
	if ok && j.State == JobDone {
		r.Notifier.Notify(context.Background(), r.notifyTitle(false), j.Name, false)
	}
	return nil
}

// waitReady polls the engine until the item is ready, handling selection.
func (r *Runner) waitReady(ctx context.Context, id string, eng Engine, j Job) error {
	selected := false
	for {
		s, err := eng.Status(ctx, j.Item)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				r.Store.Update(id, func(j *Job) { j.Item = Item{} })
				return Retryable(errors.New("engine item vanished"))
			}
			return fmt.Errorf("status: %w", err)
		}
		switch s.State {
		case ItemFailed:
			msg := s.Error
			if msg == "" {
				msg = "engine reported a failure"
			}
			return errors.New(msg)
		case ItemReady:
			return nil
		case ItemWaitingSelection:
			if eng.Caps().Select && !selected {
				r.Store.SetState(id, JobWaitingSelection)
				if err := eng.Select(ctx, j.Item, j.Selection); err != nil {
					return fmt.Errorf("select files: %w", err)
				}
				selected = true
			}
		}
		r.Store.Update(id, func(j *Job) {
			if j.State.Terminal() {
				return
			}
			if s.State != ItemWaitingSelection && CanTransition(j.State, JobFetching) {
				j.State = JobFetching
			}
			j.Progress, j.Speed, j.ETA = s.Progress, s.Speed, s.ETA
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.PollEvery):
		}
	}
}

// selectFiles keeps the files the user picked: by path when the preview came
// from the .torrent, else by index.
func selectFiles(files []File, p Payload, sel []int) []File {
	if sel == nil {
		return files
	}
	wanted := map[string]bool{}
	if len(p.Files) > 0 {
		for _, i := range sel {
			if i >= 0 && i < len(p.Files) {
				wanted[p.Files[i].Path] = true
			}
		}
		out := []File{}
		for _, f := range files {
			if wanted[f.Path] {
				out = append(out, f)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	byIndex := map[int]bool{}
	for _, i := range sel {
		byIndex[i] = true
	}
	out := []File{}
	for _, f := range files {
		if byIndex[f.Index] {
			out = append(out, f)
		}
	}
	return out
}

// FileURL is the stream route for a job file.
func FileURL(jobID, filePath string) string {
	parts := strings.Split(filePath, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "/api/jobs/" + jobID + "/files/" + strings.Join(parts, "/")
}

func (r *Runner) adopt(ctx context.Context, id string, st Storage, subdir string, files []File) error {
	r.Store.Update(id, func(j *Job) {
		j.Files = nil
		for _, f := range files {
			j.Files = append(j.Files, JobFile{Path: path.Join(subdir, f.Path), Size: f.Size, State: FilePending})
		}
	})
	for i, f := range files {
		if f.LocalPath == "" {
			return errors.New("engine does not expose local files; use copy mode")
		}
		rel := path.Join(subdir, f.Path)
		r.setFile(id, i, func(jf *JobFile) { jf.State = FileCopying })
		if err := st.Adopt(ctx, f.LocalPath, rel); err != nil {
			r.setFile(id, i, func(jf *JobFile) { jf.State = FileFailed })
			return fmt.Errorf("adopt %s: %w", f.Path, err)
		}
		r.setFile(id, i, func(jf *JobFile) { jf.State, jf.Done = FileDone, f.Size })
		r.Store.Update(id, func(j *Job) { j.Progress = float64(i+1) / float64(len(files)) })
	}
	return nil
}

func (r *Runner) setFile(id string, i int, fn func(*JobFile)) {
	r.Store.Update(id, func(j *Job) {
		if i < len(j.Files) {
			fn(&j.Files[i])
		}
	})
}

// copy streams every selected file into the storage with a bounded pool,
// after a free-space preflight. Existing files are skipped, partial files
// resumed.
func (r *Runner) copy(ctx context.Context, id string, st Storage, subdir string, files []File) error {
	type plan struct {
		rel    string
		offset int64
		skip   bool
	}
	plans := make([]plan, len(files))
	var need, total int64
	jobFiles := make([]JobFile, len(files))
	for i, f := range files {
		rel := path.Join(subdir, f.Path)
		plans[i].rel = rel
		jobFiles[i] = JobFile{Path: rel, Size: f.Size, State: FilePending}
		total += f.Size
		if ok, err := st.Exists(ctx, rel, f.Size); err == nil && ok {
			plans[i].skip = true
			jobFiles[i].State, jobFiles[i].Done = FileSkipped, f.Size
			continue
		}
		off, _ := st.Partial(ctx, rel)
		if off > f.Size {
			off = 0
		}
		plans[i].offset = off
		jobFiles[i].Done = off
		need += f.Size - off
	}
	r.Store.Update(id, func(j *Job) { j.Files = jobFiles })
	if free, err := st.Free(ctx); err != nil {
		slog.Warn("job: free space unknown", "storage", st.ID(), "err", err)
	} else if free >= 0 && free < need {
		return Retryable(fmt.Errorf("not enough free space on %s: need %d bytes, %d free", st.Label(), need, free))
	}

	var (
		wg       sync.WaitGroup
		sem      = make(chan struct{}, r.Limits.FilesPerJob)
		errMu    sync.Mutex
		firstErr error
		doneMu   sync.Mutex
		done     = make([]int64, len(files))
	)
	for i := range files {
		done[i] = jobFiles[i].Done
	}
	cctx, stop := context.WithCancel(ctx)
	defer stop()
	go r.reportSpeed(cctx, id, total, &doneMu, done)
	for i, f := range files {
		if plans[i].skip {
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-cctx.Done():
			wg.Wait()
			return cctx.Err()
		}
		wg.Add(1)
		go func(i int, f File) {
			defer wg.Done()
			defer func() { <-sem }()
			r.setFile(id, i, func(jf *JobFile) { jf.State = FileCopying })
			open := f.Open
			if open == nil && f.LocalPath != "" {
				open = openLocal(f.LocalPath)
			}
			if open == nil {
				open = func(context.Context, int64) (io.ReadCloser, error) {
					return nil, errors.New("engine exposes neither a stream nor a local path")
				}
			}
			err := st.Put(cctx, plans[i].rel, f.Size, open, func(written int64) {
				doneMu.Lock()
				done[i] = written
				doneMu.Unlock()
				r.setFile(id, i, func(jf *JobFile) { jf.Done = written })
			})
			if err != nil {
				r.setFile(id, i, func(jf *JobFile) { jf.State = FileFailed })
				errMu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("copy %s: %w", f.Path, err)
					stop()
				}
				errMu.Unlock()
				return
			}
			doneMu.Lock()
			done[i] = f.Size
			doneMu.Unlock()
			r.setFile(id, i, func(jf *JobFile) { jf.State, jf.Done = FileDone, f.Size })
		}(i, f)
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

// reportSpeed updates progress, speed and ETA of a copying job every second.
func (r *Runner) reportSpeed(ctx context.Context, id string, total int64, mu *sync.Mutex, done []int64) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	var last int64
	lastAt := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			mu.Lock()
			var sum int64
			for _, d := range done {
				sum += d
			}
			mu.Unlock()
			elapsed := now.Sub(lastAt).Seconds()
			var speed int64
			if elapsed > 0 && last > 0 && sum >= last {
				speed = int64(float64(sum-last) / elapsed)
			}
			last, lastAt = sum, now
			r.Store.Update(id, func(j *Job) {
				if j.State != JobCopying {
					return
				}
				if total > 0 {
					j.Progress = float64(sum) / float64(total)
				}
				j.Speed = speed
				if speed > 0 {
					j.ETA = (total - sum) / speed
				} else {
					j.ETA = 0
				}
			})
		}
	}
}

// openLocal turns a local file into an OpenAt.
func openLocal(p string) OpenAt {
	return func(ctx context.Context, offset int64) (io.ReadCloser, error) {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		if offset > 0 {
			if _, err := f.Seek(offset, 0); err != nil {
				f.Close()
				return nil, err
			}
		}
		return f, nil
	}
}

// notifyTitle returns the notification title in the configured language.
func (r *Runner) notifyTitle(failed bool) string {
	if r.Lang == "fr" {
		if failed {
			return "Téléchargement échoué"
		}
		return "Téléchargement terminé"
	}
	if failed {
		return "Download failed"
	}
	return "Download finished"
}
