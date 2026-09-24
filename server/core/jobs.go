package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JobState is the job lifecycle exposed by the API.
type JobState string

const (
	JobQueued           JobState = "queued"
	JobAdding           JobState = "adding"
	JobWaitingSelection JobState = "waitingSelection"
	JobFetching         JobState = "fetching"
	JobReady            JobState = "ready"
	JobCopying          JobState = "copying"
	JobDone             JobState = "done"
	JobFailed           JobState = "failed"
	JobCancelled        JobState = "cancelled"
)

// Terminal reports whether no further transition is possible except retry.
func (s JobState) Terminal() bool {
	return s == JobDone || s == JobFailed || s == JobCancelled
}

// transitions lists the allowed state changes.
var transitions = map[JobState][]JobState{
	JobQueued:           {JobAdding, JobFetching, JobCopying, JobFailed, JobCancelled},
	JobAdding:           {JobWaitingSelection, JobFetching, JobReady, JobFailed, JobCancelled},
	JobWaitingSelection: {JobFetching, JobFailed, JobCancelled},
	JobFetching:         {JobWaitingSelection, JobReady, JobFailed, JobCancelled},
	JobReady:            {JobCopying, JobDone, JobFailed, JobCancelled},
	JobCopying:          {JobFetching, JobDone, JobFailed, JobCancelled},
	JobFailed:           {JobQueued},
}

// CanTransition reports whether from may become to.
func CanTransition(from, to JobState) bool {
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// FileState is the per-file delivery state.
type FileState string

const (
	FilePending FileState = "pending"
	FileCopying FileState = "copying"
	FileDone    FileState = "done"
	FileFailed  FileState = "failed"
	FileSkipped FileState = "skipped"
)

// JobFile is one file of a job as shown in the queue.
type JobFile struct {
	Path  string    `json:"path"`
	Size  int64     `json:"size"`
	Done  int64     `json:"done"`
	State FileState `json:"state"`
	URL   string    `json:"url,omitempty"`
}

// Job is Payload x Engine x Storage with a state machine. The JSON tags
// describe the persisted form; the API shape comes from View.
type Job struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Name      string    `json:"name"`
	InfoHash  string    `json:"infoHash,omitempty"`
	Engine    string    `json:"engine"`
	Storage   string    `json:"storage,omitempty"`
	Subdir    string    `json:"subdir,omitempty"`
	Mode      Mode      `json:"mode"`
	State     JobState  `json:"state"`
	Progress  float64   `json:"progress"`
	Speed     int64     `json:"speed,omitempty"`
	ETA       int64     `json:"eta,omitempty"`
	Error     string    `json:"error,omitempty"`
	Retryable bool      `json:"retryable"`
	Files     []JobFile `json:"files"`
	External  bool      `json:"external"`

	// Internal, persisted so a restart can resume.
	Payload   Payload `json:"payload"`
	Item      Item    `json:"item"`
	Owned     bool    `json:"owned"`     // this app created the engine item
	Selection []int   `json:"selection"` // payload file indexes; nil = all
}

// JobView is the exact API representation of a Job.
type JobView struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
	Name      string        `json:"name"`
	InfoHash  *string       `json:"infoHash"`
	Engine    string        `json:"engine"`
	Storage   *string       `json:"storage"`
	Subdir    *string       `json:"subdir"`
	Mode      Mode          `json:"mode"`
	State     JobState      `json:"state"`
	Progress  float64       `json:"progress"`
	Speed     *int64        `json:"speed"`
	ETA       *int64        `json:"eta"`
	Error     *string       `json:"error"`
	Retryable bool          `json:"retryable"`
	Files     []JobFileView `json:"files"`
	External  bool          `json:"external"`
}

// JobFileView is the API representation of a JobFile.
type JobFileView struct {
	Path  string    `json:"path"`
	Size  int64     `json:"size"`
	Done  int64     `json:"done"`
	State FileState `json:"state"`
	URL   *string   `json:"url"`
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optInt(n int64) *int64 {
	if n <= 0 {
		return nil
	}
	return &n
}

// View builds the API shape.
func (j Job) View() JobView {
	v := JobView{
		ID: j.ID, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, Name: j.Name,
		InfoHash: optStr(j.InfoHash), Engine: j.Engine, Storage: optStr(j.Storage), Subdir: optStr(j.Subdir),
		Mode: j.Mode, State: j.State, Progress: j.Progress, Speed: optInt(j.Speed), ETA: optInt(j.ETA),
		Error: optStr(j.Error), Retryable: j.Retryable, Files: []JobFileView{}, External: j.External,
	}
	for _, f := range j.Files {
		v.Files = append(v.Files, JobFileView{Path: f.Path, Size: f.Size, Done: f.Done, State: f.State, URL: optStr(f.URL)})
	}
	return v
}

// ErrJobNotFound is returned for unknown job ids.
var ErrJobNotFound = errors.New("job not found")

// JobStore is the in-memory job list, newest first, persisted as JSON with a
// debounce so bursts of progress updates cost one write.
type JobStore struct {
	mu      sync.Mutex
	jobs    []*Job
	path    string
	history int
	dirty   bool
	timer   *time.Timer
	saveErr func(error)
}

// NewJobStore builds a store persisted at path, keeping at most history
// terminal jobs.
func NewJobStore(path string, history int) *JobStore {
	return &JobStore{path: path, history: history, saveErr: func(error) {}}
}

// OnSaveError sets the callback for background write failures.
func (s *JobStore) OnSaveError(fn func(error)) { s.saveErr = fn }

// Load reads the state file; a missing file is not an error.
func (s *JobStore) Load() error {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var jobs []*Job
	if err := json.Unmarshal(b, &jobs); err != nil {
		return err
	}
	s.mu.Lock()
	s.jobs = jobs
	s.mu.Unlock()
	return nil
}

// Add inserts a new job at the front of the list.
func (s *JobStore) Add(j *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append([]*Job{j}, s.jobs...)
	s.pruneLocked()
	s.markLocked()
}

// Get returns a snapshot of a job.
func (s *JobStore) Get(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.findLocked(id); j != nil {
		return *j, true
	}
	return Job{}, false
}

// Update applies fn to a job under the lock and returns the new snapshot.
func (s *JobStore) Update(id string, fn func(j *Job)) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.findLocked(id)
	if j == nil {
		return Job{}, false
	}
	fn(j)
	j.UpdatedAt = time.Now().UTC()
	s.markLocked()
	return *j, true
}

// SetState moves a job to state when the transition is allowed.
func (s *JobStore) SetState(id string, to JobState) (Job, bool) {
	ok := false
	j, found := s.Update(id, func(j *Job) {
		if CanTransition(j.State, to) {
			j.State = to
			ok = true
		}
	})
	return j, found && ok
}

// Remove drops a job from the list.
func (s *JobStore) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, j := range s.jobs {
		if j.ID == id {
			s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
			s.markLocked()
			return true
		}
	}
	return false
}

// List returns snapshots of every job, newest first.
func (s *JobStore) List() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, *j)
	}
	return out
}

// Flush writes pending changes synchronously.
func (s *JobStore) Flush() error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	s.dirty = false
	b, err := json.MarshalIndent(s.jobs, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *JobStore) findLocked(id string) *Job {
	for _, j := range s.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// markLocked schedules a debounced write.
func (s *JobStore) markLocked() {
	s.dirty = true
	if s.timer == nil {
		s.timer = time.AfterFunc(time.Second, func() {
			s.mu.Lock()
			s.timer = nil
			s.mu.Unlock()
			if err := s.Flush(); err != nil {
				s.saveErr(err)
			}
		})
	}
}

// pruneLocked drops the oldest terminal jobs beyond the history cap.
func (s *JobStore) pruneLocked() {
	if len(s.jobs) <= s.history {
		return
	}
	for i := len(s.jobs) - 1; i >= 0 && len(s.jobs) > s.history; i-- {
		if s.jobs[i].State.Terminal() {
			s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
		}
	}
}

// NewJobID returns a fresh "j_..." id.
func NewJobID() string { return "j_" + randomID(8) }
