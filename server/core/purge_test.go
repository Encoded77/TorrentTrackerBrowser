package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func purgeRunner(t *testing.T, st *memStorage, eng *fakeEngine, j *Job) *Runner {
	t.Helper()
	reg := NewRegistry()
	reg.AddEngine(eng, EngineOptions{})
	reg.AddStorage(st)
	store := NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 50)
	store.Add(j)
	return NewRunner(reg, store, &recordingNotifier{}, Limits{Jobs: 1, FilesPerJob: 1})
}

func TestPurgeInfectedDeletesQuarantinedAndCleanFiles(t *testing.T) {
	st, eng := newMemStorage("s"), newFakeEngine("e")
	item, _ := eng.Add(context.Background(), payload(), AddOpts{})
	st.files[".quarantine/j_i/Game/crack.dll"] = []byte("bad")
	st.files["Game/setup.exe"] = []byte("ok")
	r := purgeRunner(t, st, eng, &Job{ID: "j_i", Engine: "e", Storage: "s", Mode: ModeCopy, State: JobInfected,
		Owned: true, Item: item, Files: []JobFile{
			{Path: "Game/crack.dll", State: FileQuarantined},
			{Path: "Game/setup.exe", State: FileDone},
		}})
	if err := r.Purge(context.Background(), "j_i"); err != nil {
		t.Fatal(err)
	}
	if len(st.files) != 0 {
		t.Errorf("files left: %v", st.files)
	}
	if _, ok := r.Store.Get("j_i"); ok {
		t.Error("the job must leave the queue")
	}
	if len(eng.removed) != 1 || !eng.removed[0] {
		t.Errorf("engine item must be removed with its data: %v", eng.removed)
	}
}

func TestPurgeCancelledKeepsFilesItDidNotWrite(t *testing.T) {
	st, eng := newMemStorage("s"), newFakeEngine("e")
	st.files["Show/e01.mkv"] = []byte("copied before the cancel")
	st.files["Show/e02.mkv"] = []byte("already on the NAS, skipped by the copy")
	r := purgeRunner(t, st, eng, &Job{ID: "j_c", Engine: "e", Storage: "s", Mode: ModeCopy, State: JobCancelled,
		Files: []JobFile{
			{Path: "Show/e01.mkv", State: FileDone},
			{Path: "Show/e02.mkv", State: FileSkipped},
			{Path: "Show/e03.mkv", State: FilePending},
		}})
	if err := r.Purge(context.Background(), "j_c"); err != nil {
		t.Fatal(err)
	}
	if _, gone := st.files["Show/e01.mkv"]; gone {
		t.Error("the copied file must be deleted")
	}
	if _, kept := st.files["Show/e02.mkv"]; !kept {
		t.Error("a skipped (pre-existing) file must never be deleted")
	}
}

func TestPurgeRefusesOtherStates(t *testing.T) {
	for _, state := range []JobState{JobDone, JobCopying, JobScanning, JobFailed, JobQueued} {
		st := newMemStorage("s")
		st.files["a.exe"] = []byte("keep")
		r := purgeRunner(t, st, newFakeEngine("e"), &Job{ID: "j_x", Engine: "e", Storage: "s", Mode: ModeCopy, State: state,
			Files: []JobFile{{Path: "a.exe", State: FileDone}}})
		if err := r.Purge(context.Background(), "j_x"); !errors.Is(err, ErrNotPurgeable) {
			t.Errorf("%s: err = %v", state, err)
		}
		if _, kept := st.files["a.exe"]; !kept {
			t.Errorf("%s: file deleted", state)
		}
	}
	r := purgeRunner(t, newMemStorage("s"), newFakeEngine("e"), &Job{ID: "j_y", State: JobCancelled})
	if err := r.Purge(context.Background(), "j_nope"); !errors.Is(err, ErrJobNotFound) {
		t.Errorf("unknown job: %v", err)
	}
}

func TestPurgeKeepsJobWhenAFileCannotBeDeleted(t *testing.T) {
	st := newMemStorage("s")
	st.files["a.exe"] = []byte("x")
	st.deleteErr = errors.New("permission denied")
	r := purgeRunner(t, st, newFakeEngine("e"), &Job{ID: "j_e", Engine: "e", Storage: "s", Mode: ModeCopy, State: JobCancelled,
		Files: []JobFile{{Path: "a.exe", State: FileDone}}})
	if err := r.Purge(context.Background(), "j_e"); err == nil {
		t.Fatal("expected the delete error")
	}
	if _, ok := r.Store.Get("j_e"); !ok {
		t.Error("the job must stay so the purge can be retried")
	}
}
