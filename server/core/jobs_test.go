package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStateMachine(t *testing.T) {
	allowed := [][2]JobState{
		{JobQueued, JobAdding}, {JobAdding, JobFetching}, {JobFetching, JobReady}, {JobReady, JobCopying},
		{JobCopying, JobDone}, {JobFetching, JobWaitingSelection}, {JobWaitingSelection, JobFetching},
		{JobFailed, JobQueued}, {JobQueued, JobCancelled}, {JobCopying, JobCancelled}, {JobCopying, JobFetching},
		{JobReady, JobDone}, {JobQueued, JobCopying},
	}
	for _, a := range allowed {
		if !CanTransition(a[0], a[1]) {
			t.Errorf("%s -> %s should be allowed", a[0], a[1])
		}
	}
	denied := [][2]JobState{
		{JobDone, JobQueued}, {JobCancelled, JobQueued}, {JobDone, JobCopying}, {JobQueued, JobDone},
		{JobFailed, JobFetching}, {JobAdding, JobQueued}, {JobCancelled, JobFailed},
	}
	for _, d := range denied {
		if CanTransition(d[0], d[1]) {
			t.Errorf("%s -> %s should be denied", d[0], d[1])
		}
	}
	for _, s := range []JobState{JobDone, JobFailed, JobCancelled} {
		if !s.Terminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	if JobCopying.Terminal() {
		t.Error("copying is not terminal")
	}
}

func TestJobStoreSetStateAndView(t *testing.T) {
	s := NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 10)
	j := &Job{ID: "j_1", State: JobQueued, Engine: "e", Mode: ModeCopy, Files: []JobFile{}}
	s.Add(j)
	if _, ok := s.SetState("j_1", JobDone); ok {
		t.Error("queued -> done must be refused")
	}
	if got, ok := s.SetState("j_1", JobAdding); !ok || got.State != JobAdding {
		t.Errorf("queued -> adding: %v %v", got.State, ok)
	}
	j1, _ := s.Get("j_1")
	b, _ := json.Marshal(j1.View())
	for _, want := range []string{`"storage":null`, `"error":null`, `"speed":null`, `"eta":null`, `"files":[]`, `"infoHash":null`, `"external":false`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("view %s lacks %s", b, want)
		}
	}
}

func TestJobStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "jobs.json")
	s := NewJobStore(path, 10)
	s.Add(&Job{ID: "j_a", State: JobDone, Payload: Payload{Torrent: []byte("d4:infod4:name1:xee"), Magnet: "m"}, Item: Item{ID: "42"}, Owned: true, Selection: []int{1}})
	s.Add(&Job{ID: "j_b", State: JobCopying})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("state file not written:", err)
	}
	s2 := NewJobStore(path, 10)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	list := s2.List()
	if len(list) != 2 || list[0].ID != "j_b" || list[1].ID != "j_a" {
		t.Fatalf("list = %+v", list)
	}
	a := list[1]
	if string(a.Payload.Torrent) != "d4:infod4:name1:xee" || a.Item.ID != "42" || !a.Owned || len(a.Selection) != 1 {
		t.Errorf("internal fields lost: %+v", a)
	}
}

func TestJobStoreDebounce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	s := NewJobStore(path, 10)
	s.Add(&Job{ID: "j_a", State: JobQueued})
	if _, err := os.Stat(path); err == nil {
		t.Fatal("written synchronously, expected debounce")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("debounced write never happened")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestJobStorePrunesHistory(t *testing.T) {
	s := NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 2)
	s.Add(&Job{ID: "j_1", State: JobDone})
	s.Add(&Job{ID: "j_2", State: JobCopying})
	s.Add(&Job{ID: "j_3", State: JobQueued})
	list := s.List()
	if len(list) != 2 || list[0].ID != "j_3" || list[1].ID != "j_2" {
		t.Errorf("terminal job should be pruned first: %+v", list)
	}
}

func TestScanStates(t *testing.T) {
	for _, a := range [][2]JobState{
		{JobCopying, JobScanning}, {JobReady, JobScanning}, {JobScanning, JobDone}, {JobScanning, JobInfected},
		{JobScanning, JobFailed}, {JobDone, JobScanning},
	} {
		if !CanTransition(a[0], a[1]) {
			t.Errorf("%s -> %s should be allowed", a[0], a[1])
		}
	}
	for _, d := range [][2]JobState{
		{JobInfected, JobQueued}, {JobInfected, JobScanning}, {JobScanning, JobCopying}, {JobQueued, JobScanning},
	} {
		if CanTransition(d[0], d[1]) {
			t.Errorf("%s -> %s should be denied", d[0], d[1])
		}
	}
	if !JobInfected.Terminal() || JobScanning.Terminal() {
		t.Error("infected is terminal, scanning is not")
	}
}

func TestSummarize(t *testing.T) {
	cases := []struct {
		in   []ScanStatus
		want ScanStatus
	}{
		{nil, ScanClean},
		{[]ScanStatus{ScanSkipped}, ScanSkipped},
		{[]ScanStatus{ScanSkipped, ScanError}, ScanError},
		{[]ScanStatus{ScanError, ScanInfected, ScanSkipped}, ScanInfected},
	}
	for _, c := range cases {
		var f []ScanFinding
		for _, s := range c.in {
			f = append(f, ScanFinding{Status: s})
		}
		if got := summarize(f); got != c.want {
			t.Errorf("summarize(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestViewCarriesScan(t *testing.T) {
	j := Job{ID: "j_1", State: JobDone, Files: []JobFile{}}
	b, _ := json.Marshal(j.View())
	if !strings.Contains(string(b), `"scan":null`) {
		t.Errorf("view without scan = %s", b)
	}
	j.Scan = &ScanResult{Status: ScanInfected, Findings: []ScanFinding{{Path: "a.exe", Status: ScanInfected, Signature: "Eicar", Quarantine: ".quarantine/j_1/a.exe"}}}
	b, _ = json.Marshal(j.View())
	for _, want := range []string{`"status":"infected"`, `"signature":"Eicar"`, `"quarantine":".quarantine/j_1/a.exe"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("view %s lacks %s", b, want)
		}
	}
}
