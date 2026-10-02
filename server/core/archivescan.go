package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mholt/archives"
)

// maxListed bounds the inner paths quoted in one verdict.
const maxListed = 5

// scanArchive handles a file too big for the scanner: when it is a ZIP, RAR
// or 7z archive, every entry is streamed to the scanner on its own, named
// "<archive>!<entry>". ok is false when the file is not such an archive.
// The verdict is for the archive as a whole: infected when any entry is,
// skipped when an entry or the archive itself could not be read.
func scanArchive(ctx context.Context, sc Scanner, name string, rc io.Reader, limit int64) (v Verdict, ok bool) {
	format, stream, err := archives.Identify(ctx, name, rc)
	if err != nil {
		return Verdict{}, false
	}
	switch format.(type) {
	case archives.Zip, archives.Rar, archives.SevenZip:
	default:
		return Verdict{}, false
	}
	if s, seekable := rc.(io.Seeker); seekable {
		if _, err := s.Seek(0, io.SeekStart); err == nil {
			stream = rc // ZIP and 7z need the ReaderAt of the file itself
		}
	}
	var infected, skipped []string
	var scanErr error
	walk := func() (err error) {
		// rardecode panics on some damaged or partial volumes.
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("archive reader crashed: %v", p)
			}
		}()
		return format.(archives.Extractor).Extract(ctx, stream, func(ctx context.Context, fi archives.FileInfo) error {
			if fi.IsDir() {
				return nil
			}
			if fi.Size() > limit {
				skipped = append(skipped, fi.NameInArchive+" (too large)")
				return nil
			}
			f, err := fi.Open()
			if err != nil {
				skipped = append(skipped, fi.NameInArchive+": "+err.Error())
				return nil
			}
			src := &readRecorder{r: f}
			v, err := sc.Scan(ctx, name+"!"+fi.NameInArchive, src, fi.Size())
			f.Close()
			switch {
			case ctx.Err() != nil:
				return ctx.Err()
			case err != nil && src.err != nil:
				skipped = append(skipped, fi.NameInArchive+": "+src.err.Error()) // undecodable entry
			case err != nil:
				scanErr = err // scanner down: stop, the job can be rescanned
				return err
			case v.Status == ScanInfected:
				infected = append(infected, fi.NameInArchive+": "+v.Signature)
			case v.Status == ScanSkipped:
				skipped = append(skipped, fi.NameInArchive+": "+v.Reason)
			}
			return nil
		})
	}
	walkErr := walk()
	switch {
	case len(infected) > 0:
		return Verdict{Status: ScanInfected, Signature: listed(infected)}, true
	case scanErr != nil:
		return Verdict{Status: ScanError, Reason: scanErr.Error()}, true
	case walkErr != nil && !errors.Is(walkErr, context.Canceled):
		return Verdict{Status: ScanSkipped, Reason: "archive could not be read (encrypted, multi-part or damaged): " + walkErr.Error()}, true
	case len(skipped) > 0:
		return Verdict{Status: ScanSkipped, Reason: "inside the archive: " + listed(skipped)}, true
	}
	return Verdict{Status: ScanClean}, true
}

// readRecorder remembers a read failure of an archive entry, so a decoding
// error is not mistaken for the scanner being unreachable.
type readRecorder struct {
	r   io.Reader
	err error
}

func (r *readRecorder) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}

func listed(items []string) string {
	if len(items) <= maxListed {
		return strings.Join(items, "; ")
	}
	return strings.Join(items[:maxListed], "; ") + fmt.Sprintf("; +%d more", len(items)-maxListed)
}
