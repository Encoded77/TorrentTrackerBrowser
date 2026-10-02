package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"
)

// maxMessageFindings bounds the notification body: a 300-file job scanned
// while clamd is down must not produce a 300-line push.
const maxMessageFindings = 10

// scanAndFinish runs once the files are delivered (copy and adopt modes):
// scans them when a scanner covers the storage, quarantines infected files,
// drops the engine item as configured, then ends the job. It is also the
// entry point of a rescan and of a job resumed in the scanning state.
func (r *Runner) scanAndFinish(ctx context.Context, id string) error {
	j, ok := r.Store.Get(id)
	if !ok {
		return ErrJobNotFound
	}
	st := r.Reg.Storage(j.Storage)
	if st == nil {
		return fmt.Errorf("storage %q is not configured", j.Storage)
	}
	sc := r.Reg.ScannerFor(j.Storage)
	if sc == nil {
		r.afterDelivery(ctx, j, false)
		return r.finish(id, nil)
	}
	if err := r.setState(id, JobScanning); err != nil {
		return err
	}
	r.Store.Update(id, func(j *Job) { j.Progress, j.Speed, j.ETA = 0, 0, 0 })
	res, err := r.scan(ctx, id, st, sc)
	if err != nil {
		return err // shutdown: the job stays in scanning and resumes at start
	}
	r.afterDelivery(ctx, j, res.Status == ScanInfected)
	return r.finish(id, res)
}

// scan sends every delivered file to the scanner, in order, and moves
// infected ones to QuarantineDir/<job id>/<path>. Scanner and read failures
// become findings; only the end of ctx is returned as an error. Findings are
// saved as they are found, so a scan resumed after a restart keeps the files
// it already quarantined.
func (r *Runner) scan(ctx context.Context, id string, st Storage, sc Scanner) (*ScanResult, error) {
	j, _ := r.Store.Get(id)
	saved := map[string]ScanFinding{}
	if j.Scan != nil {
		for _, f := range j.Scan.Findings {
			saved[f.Path] = f
		}
	}
	res := &ScanResult{Findings: []ScanFinding{}}
	record := func(fd ScanFinding) {
		res.Findings = append(res.Findings, fd)
		partial := &ScanResult{Status: summarize(res.Findings), Findings: append([]ScanFinding(nil), res.Findings...)}
		r.Store.Update(id, func(j *Job) { j.Scan = partial })
	}
	for i, f := range j.Files {
		if f.State == FileQuarantined {
			fd, ok := saved[f.Path]
			if !ok {
				fd = ScanFinding{Path: f.Path, Status: ScanInfected, Reason: "quarantined before a restart",
					Quarantine: path.Join(QuarantineDir, id, f.Path)}
			}
			record(fd)
		}
		if f.State == FileDone || f.State == FileSkipped {
			v, err := r.scanFile(ctx, st, sc, f)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err != nil {
				v = Verdict{Status: ScanError, Reason: err.Error()}
			}
			if v.Status != ScanClean {
				fd := ScanFinding{Path: f.Path, Status: v.Status, Signature: v.Signature, Reason: v.Reason}
				if v.Status == ScanInfected {
					q := path.Join(QuarantineDir, id, f.Path)
					if err := st.Move(ctx, f.Path, q); err != nil {
						fd.Reason = "quarantine failed, file left in place: " + err.Error()
					} else {
						fd.Quarantine = q
						r.setFile(id, i, func(jf *JobFile) { jf.State = FileQuarantined })
					}
				}
				record(fd)
			}
		}
		r.Store.Update(id, func(j *Job) { j.Progress = float64(i+1) / float64(len(j.Files)) })
	}
	res.Status, res.ScannedAt = summarize(res.Findings), time.Now().UTC()
	return res, nil
}

func (r *Runner) scanFile(ctx context.Context, st Storage, sc Scanner, f JobFile) (Verdict, error) {
	rc, err := st.Open(ctx, f.Path)
	if err != nil {
		return Verdict{}, err
	}
	defer rc.Close()
	if limit := sc.MaxSize(); limit > 0 && f.Size > limit {
		if v, ok := scanArchive(ctx, sc, f.Path, rc, limit); ok {
			return v, nil
		}
	}
	return sc.Scan(ctx, f.Path, rc, f.Size)
}

// afterDelivery drops the engine item this app created: with its data when
// malware was found (a client would keep seeding it), otherwise only when
// the engine is configured with removeAfterCopy.
func (r *Runner) afterDelivery(ctx context.Context, j Job, infected bool) {
	if !j.Owned || j.Item.ID == "" {
		return
	}
	if !infected && !r.Reg.EngineOptions(j.Engine).RemoveAfterCopy {
		return
	}
	eng := r.Reg.Engine(j.Engine)
	if eng == nil {
		return
	}
	if err := eng.Remove(ctx, j.Item, infected); err != nil && !errors.Is(err, ErrNotFound) {
		slog.Warn("job: remove engine item after delivery", "job", j.ID, "infected", infected, "err", err)
	}
	r.invalidateExternal()
}

// outcomeMessage builds the notification of a finished job.
func (r *Runner) outcomeMessage(j Job) (string, string, Level) {
	if j.Scan == nil || j.Scan.Status == ScanClean {
		return r.notifyTitle(false), j.Name, LevelInfo
	}
	fr := r.Lang == "fr"
	lines := []string{j.Name}
	for i, f := range j.Scan.Findings {
		if i == maxMessageFindings {
			more := len(j.Scan.Findings) - maxMessageFindings
			if fr {
				lines = append(lines, fmt.Sprintf("+%d autres", more))
			} else {
				lines = append(lines, fmt.Sprintf("+%d more", more))
			}
			break
		}
		switch {
		case f.Status == ScanInfected && f.Quarantine != "":
			lines = append(lines, fmt.Sprintf("%s: %s -> %s", f.Path, f.Signature, f.Quarantine))
		case f.Status == ScanInfected:
			lines = append(lines, fmt.Sprintf("%s: %s (%s)", f.Path, f.Signature, f.Reason))
		default:
			lines = append(lines, fmt.Sprintf("%s: %s", f.Path, f.Reason))
		}
	}
	if j.Scan.Status == ScanInfected && !j.Owned {
		if fr {
			lines = append(lines, "Copie conservée sur le moteur (élément non créé par TTB)")
		} else {
			lines = append(lines, "Engine copy kept (item not created by TTB)")
		}
	}
	msg := strings.Join(lines, "\n")
	switch {
	case j.Scan.Status == ScanInfected && fr:
		return "Malware détecté", msg, LevelAlert
	case j.Scan.Status == ScanInfected:
		return "Malware detected", msg, LevelAlert
	case fr:
		return "Téléchargement terminé, non analysé", msg, LevelFailed
	}
	return "Download finished, not scanned", msg, LevelFailed
}

// Rescan scans a finished copy or adopt job again: allowed when its last
// scan did not cover every file (scanner down, file too large) or when it
// finished before a scanner was configured.
func (r *Runner) Rescan(id string) (Job, error) {
	j, ok := r.Store.Get(id)
	if !ok {
		return Job{}, ErrJobNotFound
	}
	if j.State != JobDone || j.Mode == ModeLinks {
		return j, errors.New("only a finished copy or adopt job can be rescanned")
	}
	if j.Scan != nil && j.Scan.Status == ScanClean {
		return j, errors.New("job is already scanned clean")
	}
	if r.Reg.ScannerFor(j.Storage) == nil {
		return j, errors.New("no scanner covers this storage")
	}
	j, _ = r.Store.Update(id, func(j *Job) {
		if CanTransition(j.State, JobScanning) {
			j.State, j.Scan = JobScanning, nil
		}
	})
	r.Wake()
	return j, nil
}
