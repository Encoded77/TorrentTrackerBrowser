package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestRunner(t *testing.T, eng *fakeEngine, st *memStorage, opts EngineOptions) (*Runner, *recordingNotifier) {
	t.Helper()
	reg := NewRegistry()
	reg.AddEngine(eng, opts)
	reg.AddStorage(st)
	store := NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 50)
	n := &recordingNotifier{}
	r := NewRunner(reg, store, n, Limits{Jobs: 2, FilesPerJob: 2})
	r.PollEvery = 10 * time.Millisecond
	r.Start(context.Background())
	t.Cleanup(r.Stop)
	return r, n
}

func waitState(t *testing.T, r *Runner, id string, want JobState) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		j, ok := r.Store.Get(id)
		if ok && j.State == want {
			return j
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s: state %s (error %q), want %s", id, j.State, j.Error, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func payload() Payload {
	return Payload{Name: "Root", InfoHash: "abc", Magnet: "magnet:?xt=urn:btih:abc"}
}

func TestRunnerCopyEndToEnd(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	r, n := newTestRunner(t, eng, st, EngineOptions{RemoveAfterCopy: true})
	j := r.Submit(&Job{Name: "Root", Engine: "e", Storage: "s", Subdir: "Sub", Mode: ModeCopy, Payload: payload()})
	done := waitState(t, r, j.ID, JobDone)
	if len(done.Files) != 2 || done.Files[0].Path != "Sub/Root/a.txt" || done.Files[0].State != FileDone || done.Files[1].Done != 20 {
		t.Errorf("files = %+v", done.Files)
	}
	if string(st.files["Sub/Root/a.txt"]) != "aaaaaaaaaa" || len(st.files["Sub/Root/b.txt"]) != 20 {
		t.Errorf("storage = %v", st.files)
	}
	if done.Progress != 1 || !done.Owned || done.Item.ID == "" {
		t.Errorf("job = %+v", done)
	}
	if eng.calls["remove"] != 1 {
		t.Errorf("removeAfterCopy: remove calls = %d", eng.calls["remove"])
	}
	time.Sleep(20 * time.Millisecond)
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.calls) != 1 || n.calls[0] != "Download finished" {
		t.Errorf("notifications = %v", n.calls)
	}
}

func TestRunnerSkipsExistingAndResumesPartial(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	st.files["Root/a.txt"] = []byte("aaaaaaaaaa")
	st.parts["Root/b.txt"] = []byte("bbbbb")
	r, _ := newTestRunner(t, eng, st, EngineOptions{})
	j := r.Submit(&Job{Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, Payload: payload()})
	done := waitState(t, r, j.ID, JobDone)
	if done.Files[0].State != FileSkipped {
		t.Errorf("existing file should be skipped: %+v", done.Files[0])
	}
	if len(st.files["Root/b.txt"]) != 20 {
		t.Errorf("resumed file = %q", st.files["Root/b.txt"])
	}
	if eng.calls["remove"] != 0 {
		t.Error("engine item removed without removeAfterCopy")
	}
}

func TestRunnerSelectionByPath(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	r, _ := newTestRunner(t, eng, st, EngineOptions{})
	p := payload()
	p.Files = []FileEntry{{Index: 0, Path: "Root/a.txt", Size: 10}, {Index: 1, Path: "Root/b.txt", Size: 20}}
	j := r.Submit(&Job{Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, Payload: p, Selection: []int{1}})
	done := waitState(t, r, j.ID, JobDone)
	if len(done.Files) != 1 || done.Files[0].Path != "Root/b.txt" {
		t.Errorf("files = %+v", done.Files)
	}
	if _, ok := st.files["Root/a.txt"]; ok {
		t.Error("unselected file was copied")
	}
}

func TestRunnerLinksMode(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	r, _ := newTestRunner(t, eng, st, EngineOptions{})
	j := r.Submit(&Job{Name: "Root", Engine: "e", Mode: ModeLinks, Payload: payload()})
	done := waitState(t, r, j.ID, JobDone)
	if len(done.Files) != 2 || done.Files[0].URL != "/api/jobs/"+j.ID+"/files/Root/a.txt" {
		t.Errorf("files = %+v", done.Files)
	}
	if len(st.files) != 0 {
		t.Error("links mode must not copy")
	}
	_, f, err := r.ResolveFile(context.Background(), j.ID, "Root/b.txt")
	if err != nil || f.Size != 20 {
		t.Errorf("resolve: %v %+v", err, f)
	}
}

func TestRunnerFailureAndRetry(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	eng.addErr = Retryable(errors.New("503 from debrid"))
	r, n := newTestRunner(t, eng, st, EngineOptions{})
	j := r.Submit(&Job{Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, Payload: payload()})
	failed := waitState(t, r, j.ID, JobFailed)
	if !failed.Retryable || failed.Error == "" {
		t.Errorf("failed job = %+v", failed)
	}
	if _, err := r.Cancel(context.Background(), j.ID); err == nil {
		t.Error("cancel of a terminal job must fail")
	}
	eng.addErr = nil
	if _, err := r.Retry(j.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, r, j.ID, JobDone)
	if _, err := r.Retry(j.ID); err == nil {
		t.Error("retry of a done job must fail")
	}
	time.Sleep(20 * time.Millisecond)
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.calls) != 2 || n.calls[0] != "Download failed" {
		t.Errorf("notifications = %v", n.calls)
	}
}

func TestRunnerEngineFailureNotRetryable(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	eng.statuses = []ItemState{ItemFetching, ItemFailed}
	r, _ := newTestRunner(t, eng, st, EngineOptions{})
	j := r.Submit(&Job{Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, Payload: payload()})
	failed := waitState(t, r, j.ID, JobFailed)
	if failed.Retryable || failed.Error != "scripted failure" {
		t.Errorf("failed job = %+v", failed)
	}
}

func TestRunnerCancelRemovesOwnedItem(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	eng.statuses = []ItemState{ItemFetching} // never ready
	r, _ := newTestRunner(t, eng, st, EngineOptions{})
	j := r.Submit(&Job{Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, Payload: payload()})
	waitState(t, r, j.ID, JobFetching)
	if _, err := r.Cancel(context.Background(), j.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, r, j.ID, JobCancelled)
	deadline := time.Now().Add(2 * time.Second)
	for {
		eng.mu.Lock()
		n := len(eng.items)
		eng.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("engine item not removed after cancel")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := r.Retry(j.ID); err == nil {
		t.Error("cancelled jobs are not retryable")
	}
}

func TestRunnerFreeSpacePreflight(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	st.free = 5
	r, _ := newTestRunner(t, eng, st, EngineOptions{})
	j := r.Submit(&Job{Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, Payload: payload()})
	failed := waitState(t, r, j.ID, JobFailed)
	if !failed.Retryable || len(st.files) != 0 {
		t.Errorf("preflight: %+v files=%v", failed, st.files)
	}
	if eng.calls["remove"] != 0 {
		t.Error("copy failure must keep the engine item")
	}
}

func TestRunnerDelete(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	r, _ := newTestRunner(t, eng, st, EngineOptions{})
	j := r.Submit(&Job{Name: "Root", Engine: "e", Storage: "s", Mode: ModeCopy, Payload: payload()})
	waitState(t, r, j.ID, JobDone)
	if err := r.Delete(context.Background(), j.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Store.Get(j.ID); ok {
		t.Error("job still listed")
	}
	if eng.calls["remove"] != 1 {
		t.Errorf("owned engine item should be removed on delete: %d", eng.calls["remove"])
	}
	if err := r.Delete(context.Background(), j.ID, false); !errors.Is(err, ErrJobNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestRunnerExternalAndSend(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	eng.statuses = []ItemState{ItemReady}
	it, _ := eng.Add(context.Background(), Payload{Name: "Ext", InfoHash: "fff"}, AddOpts{})
	r, _ := newTestRunner(t, eng, st, EngineOptions{})
	ext := r.External(context.Background())
	if len(ext) != 1 || ext[0].ID != "x_e_"+it.ID || !ext[0].External || ext[0].State != JobReady || len(ext[0].Files) != 2 {
		t.Fatalf("external = %+v", ext)
	}
	if ext[0].Files[0].URL == "" {
		t.Error("external ready files should carry a url")
	}
	j, err := r.Send(context.Background(), ext[0].ID, SendOpts{Storage: "s", Subdir: "X", Files: []int{0}})
	if err != nil {
		t.Fatal(err)
	}
	if j.State != JobCopying || j.Owned {
		t.Errorf("sent job = %+v", j)
	}
	done := waitState(t, r, j.ID, JobDone)
	if len(done.Files) != 1 || string(st.files["X/Root/a.txt"]) != "aaaaaaaaaa" {
		t.Errorf("sent job files = %+v storage=%v", done.Files, st.files)
	}
	if len(r.External(context.Background())) != 0 {
		t.Error("item owned by a job must leave the external list")
	}
	if err := r.Delete(context.Background(), j.ID, false); err != nil {
		t.Fatal(err)
	}
	if eng.calls["remove"] != 0 {
		t.Error("deleting a job that did not create the item must keep it")
	}
	if err := r.Delete(context.Background(), "x_e_"+it.ID, false); err != nil {
		t.Fatal(err)
	}
	if eng.calls["remove"] != 1 {
		t.Error("deleting an external item removes it from the engine")
	}
}

func TestRunnerReconcile(t *testing.T) {
	eng, st := newFakeEngine("e"), newMemStorage("s")
	eng.statuses = []ItemState{ItemReady}
	alive, _ := eng.Add(context.Background(), payload(), AddOpts{})
	reg := NewRegistry()
	reg.AddEngine(eng, EngineOptions{})
	reg.AddStorage(st)
	store := NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 50)
	store.Add(&Job{ID: "j_gone", State: JobCopying, Engine: "e", Storage: "s", Mode: ModeCopy, Item: Item{ID: "99"}, Owned: true, Payload: payload()})
	store.Add(&Job{ID: "j_alive", State: JobFetching, Engine: "e", Storage: "s", Mode: ModeCopy, Item: alive, Owned: true, Payload: payload()})
	store.Add(&Job{ID: "j_adding", State: JobAdding, Engine: "e", Storage: "s", Mode: ModeCopy, Payload: payload()})
	store.Add(&Job{ID: "j_done", State: JobDone, Engine: "e"})
	r := NewRunner(reg, store, &recordingNotifier{}, Limits{Jobs: 2, FilesPerJob: 2})
	r.PollEvery = 10 * time.Millisecond
	r.Start(context.Background())
	t.Cleanup(r.Stop)

	gone := waitState(t, r, "j_gone", JobFailed)
	if !gone.Retryable || gone.Item.ID != "" {
		t.Errorf("vanished item: %+v", gone)
	}
	waitState(t, r, "j_alive", JobDone)
	waitState(t, r, "j_adding", JobDone)
	if eng.calls["add"] != 2 { // the setup Add plus j_adding
		t.Errorf("only the job without an item should be re-added: %d", eng.calls["add"])
	}
	if j, _ := store.Get("j_done"); j.State != JobDone {
		t.Error("terminal jobs untouched")
	}
}

func TestNotifyTitleLanguage(t *testing.T) {
	r := &Runner{Lang: "fr"}
	if r.notifyTitle(false) != "Téléchargement terminé" || r.notifyTitle(true) != "Téléchargement échoué" {
		t.Errorf("fr titles = %q / %q", r.notifyTitle(false), r.notifyTitle(true))
	}
	r.Lang = ""
	if r.notifyTitle(false) != "Download finished" {
		t.Errorf("default title = %q", r.notifyTitle(false))
	}
}
