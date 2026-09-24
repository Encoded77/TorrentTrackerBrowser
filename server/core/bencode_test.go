package core

import (
	"crypto/sha1"
	"encoding/hex"
	"testing"
)

// hand-built torrents: the info dict is bencoded by hand so the expected
// hash is sha1 of exactly those bytes.
const (
	singleInfo = "d6:lengthi1234e4:name8:file.mkv12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaae"
	multiInfo  = "d5:filesld6:lengthi10e4:pathl3:sub5:a.txteed6:lengthi20e4:pathl5:b.txteee4:name4:Root12:piece lengthi16384e6:pieces20:bbbbbbbbbbbbbbbbbbbbe"
)

func TestParseTorrentSingleFile(t *testing.T) {
	raw := "d8:announce20:http://tracker/annou4:info" + singleInfo + "e"
	ti, err := ParseTorrent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum([]byte(singleInfo))
	if ti.InfoHash != hex.EncodeToString(sum[:]) {
		t.Errorf("infoHash = %s, want sha1 of the info dict", ti.InfoHash)
	}
	if ti.Name != "file.mkv" || ti.Size != 1234 {
		t.Errorf("name/size = %q/%d", ti.Name, ti.Size)
	}
	if len(ti.Files) != 1 || ti.Files[0].Path != "file.mkv" || ti.Files[0].Size != 1234 {
		t.Errorf("files = %+v", ti.Files)
	}
}

func TestParseTorrentMultiFile(t *testing.T) {
	raw := "d4:info" + multiInfo + "e"
	ti, err := ParseTorrent([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum([]byte(multiInfo))
	if ti.InfoHash != hex.EncodeToString(sum[:]) {
		t.Errorf("infoHash = %s", ti.InfoHash)
	}
	if ti.Size != 30 || len(ti.Files) != 2 {
		t.Fatalf("size/files = %d/%d", ti.Size, len(ti.Files))
	}
	if ti.Files[0].Path != "Root/sub/a.txt" || ti.Files[1].Path != "Root/b.txt" || ti.Files[1].Index != 1 {
		t.Errorf("files = %+v", ti.Files)
	}
}

func TestParseTorrentRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "not bencode", "d4:infoi1ee", "d3:foo3:bare", "li1e", "d4:info" + singleInfo} {
		if _, err := ParseTorrent([]byte(in)); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}
