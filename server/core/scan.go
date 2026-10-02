package core

import (
	"context"
	"io"
	"time"
)

// ScanStatus is the outcome of scanning one file or a whole job.
type ScanStatus string

const (
	ScanClean    ScanStatus = "clean"
	ScanInfected ScanStatus = "infected"
	ScanSkipped  ScanStatus = "skipped" // not sent to the scanner (over its size limit)
	ScanError    ScanStatus = "error"   // scanner unreachable, timeout, unreadable file
)

// Verdict is a scanner's answer for one file.
type Verdict struct {
	Status    ScanStatus
	Signature string // set when infected
	Reason    string // set when skipped
}

// Scanner checks file contents for malware. Scan reads r to the end, or not
// at all when size is over the scanner's limit (Status skipped). Transport
// failures are returned as errors; the runner records them as ScanError.
type Scanner interface {
	ID() string
	Scan(ctx context.Context, name string, r io.Reader, size int64) (Verdict, error)
	// MaxSize is the largest file Scan accepts; bigger ZIP/RAR/7z archives
	// are opened by the runner and their entries scanned one by one.
	MaxSize() int64
}

// ScanFinding is one delivered file that did not scan clean.
type ScanFinding struct {
	Path       string     `json:"path"`
	Status     ScanStatus `json:"status"`
	Signature  string     `json:"signature,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	Quarantine string     `json:"quarantine,omitempty"` // storage-relative path after the move
}

// ScanResult is the scan outcome of a job, persisted with it.
type ScanResult struct {
	Status    ScanStatus    `json:"status"`
	Findings  []ScanFinding `json:"findings"`
	ScannedAt time.Time     `json:"scannedAt"`
}

// QuarantineDir is the storage-relative folder infected files are moved to,
// one subfolder per job.
const QuarantineDir = ".quarantine"

// summarize folds per-file findings into the job status: infected beats
// error beats skipped beats clean.
func summarize(findings []ScanFinding) ScanStatus {
	rank := map[ScanStatus]int{ScanClean: 0, ScanSkipped: 1, ScanError: 2, ScanInfected: 3}
	out := ScanClean
	for _, f := range findings {
		if rank[f.Status] > rank[out] {
			out = f.Status
		}
	}
	return out
}
