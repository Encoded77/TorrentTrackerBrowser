package clamav

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

// readStream reads one zINSTREAM request and returns the streamed bytes.
func readStream(c net.Conn) ([]byte, error) {
	cmd := make([]byte, len("zINSTREAM\x00"))
	if _, err := io.ReadFull(c, cmd); err != nil {
		return nil, err
	}
	if string(cmd) != "zINSTREAM\x00" {
		return nil, io.ErrUnexpectedEOF
	}
	var out bytes.Buffer
	for {
		var n uint32
		if err := binary.Read(c, binary.BigEndian, &n); err != nil {
			return nil, err
		}
		if n == 0 {
			return out.Bytes(), nil
		}
		if _, err := io.CopyN(&out, c, int64(n)); err != nil {
			return nil, err
		}
	}
}

// fakeClamd serves handle on every connection and counts them.
func fakeClamd(t *testing.T, handle func(c net.Conn)) (string, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var conns atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns.Add(1)
			go func() { defer c.Close(); handle(c) }()
		}
	}()
	return ln.Addr().String(), &conns
}

func replying(reply string, got chan<- []byte) func(net.Conn) {
	return func(c net.Conn) {
		data, err := readStream(c)
		if err != nil {
			return
		}
		if got != nil {
			got <- data
		}
		io.WriteString(c, reply+"\x00")
	}
}

func scan(t *testing.T, s *Scanner, content []byte) (core.Verdict, error) {
	t.Helper()
	return s.Scan(context.Background(), "x.exe", bytes.NewReader(content), int64(len(content)))
}

func TestScanCleanStreamsWholeFile(t *testing.T) {
	got := make(chan []byte, 1)
	addr, _ := fakeClamd(t, replying("stream: OK", got))
	s, _ := New("av", addr, "", "")
	content := bytes.Repeat([]byte("0123456789abcdef"), 200_000) // 3.2 MB: several chunks
	v, err := scan(t, s, content)
	if err != nil || v.Status != core.ScanClean {
		t.Fatalf("verdict = %+v, %v", v, err)
	}
	if !bytes.Equal(<-got, content) {
		t.Error("clamd did not receive the exact file content")
	}
}

func TestScanInfected(t *testing.T) {
	addr, _ := fakeClamd(t, replying("stream: Win.Test.EICAR_HDB-1 FOUND", nil))
	s, _ := New("av", addr, "", "")
	v, err := scan(t, s, []byte("X5O!P%@AP"))
	if err != nil || v.Status != core.ScanInfected || v.Signature != "Win.Test.EICAR_HDB-1" {
		t.Fatalf("verdict = %+v, %v", v, err)
	}
}

func TestScanOverMaxSizeSkipsWithoutConnecting(t *testing.T) {
	addr, conns := fakeClamd(t, replying("stream: OK", nil))
	s, _ := New("av", addr, "1KB", "")
	v, err := scan(t, s, make([]byte, 2048))
	if err != nil || v.Status != core.ScanSkipped || v.Reason == "" {
		t.Fatalf("verdict = %+v, %v", v, err)
	}
	if conns.Load() != 0 {
		t.Error("an oversized file must not be sent")
	}
}

func TestScanClamdSizeLimitMidStream(t *testing.T) {
	addr, _ := fakeClamd(t, func(c net.Conn) {
		buf := make([]byte, 64<<10)
		io.ReadFull(c, buf) // part of the stream, then answer like clamd does
		io.WriteString(c, "INSTREAM size limit exceeded. ERROR\x00")
		// Drain the rest instead of closing: a close with unread input sends
		// an RST that would race the reply and make the test flaky.
		c.(*net.TCPConn).CloseWrite()
		c.SetReadDeadline(time.Now().Add(5 * time.Second))
		io.Copy(io.Discard, c)
	})
	s, _ := New("av", addr, "", "")
	v, err := scan(t, s, make([]byte, 8<<20))
	if err != nil || v.Status != core.ScanSkipped {
		t.Fatalf("verdict = %+v, %v", v, err)
	}
}

func TestScanErrors(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := ln.Addr().String()
	ln.Close()
	garbage, _ := fakeClamd(t, replying("hello", nil))
	silent, _ := fakeClamd(t, func(c net.Conn) { readStream(c); time.Sleep(5 * time.Second) })
	for name, addr := range map[string]string{"refused": closed, "garbage": garbage, "timeout": silent} {
		s, _ := New("av", addr, "", "300ms")
		start := time.Now()
		if _, err := scan(t, s, []byte("abc")); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if time.Since(start) > 3*time.Second {
			t.Errorf("%s: timeout not honoured", name)
		}
	}
}

func TestScanUnreadableSourceFailsFast(t *testing.T) {
	addr, _ := fakeClamd(t, func(c net.Conn) { readStream(c); time.Sleep(5 * time.Second) })
	s, _ := New("av", addr, "", "10s")
	start := time.Now()
	_, err := s.Scan(context.Background(), "x", io.MultiReader(strings.NewReader("ab"), errReader{}), 10)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("err = %v after %s", err, time.Since(start))
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestNewAndParseSize(t *testing.T) {
	if _, err := New("av", "", "", ""); err == nil {
		t.Error("address is required")
	}
	for in, want := range map[string]int64{"4000MB": 4000 << 20, "2GB": 2 << 30, "512kb": 512 << 10, "1000": 1000} {
		if got, err := parseSize(in); err != nil || got != want {
			t.Errorf("parseSize(%q) = %d, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "MB", "-1GB", "lots"} {
		if _, err := parseSize(bad); err == nil {
			t.Errorf("parseSize(%q) should fail", bad)
		}
	}
	if _, err := New("av", "h:1", "", "soon"); err == nil {
		t.Error("bad timeout must fail")
	}
}

// ClamAV scans at most ~2 GiB of one file (internal cap 2147483645 bytes);
// bigger files must be reported as not scanned, never as clean.
func TestDefaultMaxSizeStaysUnderClamAVFileCap(t *testing.T) {
	s, err := New("av", "h:1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if s.maxSize >= 2147483645 {
		t.Errorf("default maxSize %d would let clamd silently truncate the scan", s.maxSize)
	}
	if _, err := New("av", "h:1", "3GB", ""); err == nil {
		t.Error("a maxSize over ClamAV's per-file cap must be refused")
	}
}
