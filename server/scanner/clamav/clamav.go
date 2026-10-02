// Package clamav implements core.Scanner over clamd's INSTREAM command:
// the file is streamed in length-prefixed chunks over TCP, so clamd needs no
// access to the storage.
package clamav

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

func init() {
	core.RegisterScanner("clamav", func(cfg map[string]any) (core.Scanner, error) {
		return New(core.StringOpt(cfg, "id"), core.StringOpt(cfg, "address"), core.StringOpt(cfg, "maxSize"), core.StringOpt(cfg, "timeout"))
	})
}

const chunkSize = 1 << 20

// clamAVFileCap is the most ClamAV scans of a single file (2 GiB - 3 bytes,
// whatever MaxFileSize says): a bigger stream is scanned partially and still
// answered OK, so such files are skipped instead.
const clamAVFileCap = 2147483645

// Scanner talks to one clamd.
type Scanner struct {
	id, addr string
	maxSize  int64
	timeout  time.Duration
}

// New builds a scanner; maxSize ("2000MB") and timeout ("15m") may be empty
// for the defaults. maxSize must stay under ClamAV's 2 GiB per-file cap and
// within clamd's StreamMaxLength.
func New(id, addr, maxSize, timeout string) (*Scanner, error) {
	if addr == "" {
		return nil, errors.New("missing address")
	}
	s := &Scanner{id: id, addr: addr, maxSize: 2000 << 20, timeout: 15 * time.Minute}
	if maxSize != "" {
		n, err := parseSize(maxSize)
		if err != nil {
			return nil, fmt.Errorf("maxSize: %w", err)
		}
		if n >= clamAVFileCap {
			return nil, fmt.Errorf("maxSize %s: ClamAV scans at most 2 GiB of a file, use 2000MB or less", maxSize)
		}
		s.maxSize = n
	}
	if timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, fmt.Errorf("timeout: %w", err)
		}
		s.timeout = d
	}
	return s, nil
}

// parseSize reads "4000MB", "2GB", "512KB" or a plain byte count (powers of 1024).
func parseSize(v string) (int64, error) {
	s := strings.ToUpper(strings.TrimSpace(v))
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		mult   int64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}} {
		if strings.HasSuffix(s, u.suffix) {
			s, mult = strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), u.mult
			break
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid size %q", v)
	}
	return n * mult, nil
}

func (s *Scanner) ID() string { return s.id }

// MaxSize is the largest file sent to clamd.
func (s *Scanner) MaxSize() int64 { return s.maxSize }

// Scan streams r to clamd. Files over maxSize are skipped without a
// connection; clamd's own size-limit answer is also a skip.
func (s *Scanner) Scan(ctx context.Context, name string, r io.Reader, size int64) (core.Verdict, error) {
	if size > s.maxSize {
		return core.Verdict{Status: core.ScanSkipped, Reason: fmt.Sprintf("larger than the %d MB scan limit", s.maxSize>>20)}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return core.Verdict{}, fmt.Errorf("clamd: %w", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.SetDeadline(time.Now()) })
	defer stop()

	if err := stream(conn, r); err != nil {
		var re readErr
		if errors.As(err, &re) {
			return core.Verdict{}, fmt.Errorf("read %s: %w", name, re.err)
		}
		// clamd closes early on its size limit but has answered first.
		if reply, rerr := readReply(conn); rerr == nil {
			return parseReply(reply)
		}
		return core.Verdict{}, fmt.Errorf("clamd: %w", err)
	}
	reply, err := readReply(conn)
	if err != nil {
		return core.Verdict{}, fmt.Errorf("clamd: read reply: %w", err)
	}
	return parseReply(reply)
}

// readErr marks a failure reading the source file (not the socket).
type readErr struct{ err error }

func (e readErr) Error() string { return e.err.Error() }

// stream sends the INSTREAM command, the chunks and the zero terminator.
func stream(w io.Writer, r io.Reader) error {
	if _, err := io.WriteString(w, "zINSTREAM\x00"); err != nil {
		return err
	}
	buf := make([]byte, 4+chunkSize)
	for {
		n, err := io.ReadFull(r, buf[4:])
		if n > 0 {
			binary.BigEndian.PutUint32(buf[:4], uint32(n))
			if _, werr := w.Write(buf[:4+n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return readErr{err}
		}
	}
	_, err := w.Write([]byte{0, 0, 0, 0})
	return err
}

// readReply reads one NUL-terminated answer (at most 4 KB).
func readReply(r io.Reader) (string, error) {
	b, err := bufio.NewReader(io.LimitReader(r, 4096)).ReadString(0)
	if err != nil && !(errors.Is(err, io.EOF) && b != "") {
		return "", err
	}
	return strings.TrimRight(b, "\x00\n"), nil
}

func parseReply(reply string) (core.Verdict, error) {
	r := strings.TrimSpace(strings.TrimPrefix(reply, "stream:"))
	switch {
	case r == "OK":
		return core.Verdict{Status: core.ScanClean}, nil
	case strings.HasSuffix(r, " FOUND"):
		return core.Verdict{Status: core.ScanInfected, Signature: strings.TrimSuffix(r, " FOUND")}, nil
	case strings.Contains(r, "size limit exceeded"):
		return core.Verdict{Status: core.ScanSkipped, Reason: "larger than clamd's StreamMaxLength"}, nil
	}
	return core.Verdict{}, fmt.Errorf("clamd: unexpected reply %q", reply)
}
