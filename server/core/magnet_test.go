package core

import "testing"

func TestParseMagnetHex(t *testing.T) {
	m, err := ParseMagnet("magnet:?xt=urn:btih:0723937B3B5D7A2C4F7E8A9B0C1D2E3F40516273&dn=Dune+Part+Two&tr=udp%3A%2F%2Ft.example%3A6969")
	if err != nil {
		t.Fatal(err)
	}
	if m.InfoHash != "0723937b3b5d7a2c4f7e8a9b0c1d2e3f40516273" {
		t.Errorf("hash = %s", m.InfoHash)
	}
	if m.Name != "Dune Part Two" || len(m.Trackers) != 1 || m.Trackers[0] != "udp://t.example:6969" {
		t.Errorf("name/trackers = %q/%v", m.Name, m.Trackers)
	}
}

func TestParseMagnetBase32(t *testing.T) {
	// base32 of 20 bytes 0x00..0x13
	m, err := ParseMagnet("magnet:?xt=urn:btih:AAAQEAYEAUDAOCAJBIFQYDIOB4IBCEQT")
	if err != nil {
		t.Fatal(err)
	}
	if m.InfoHash != "000102030405060708090a0b0c0d0e0f10111213" {
		t.Errorf("hash = %s", m.InfoHash)
	}
}

func TestParseMagnetRejects(t *testing.T) {
	for _, in := range []string{"", "http://x", "magnet:?dn=only", "magnet:?xt=urn:btih:zz", "magnet:?xt=urn:sha1:abc"} {
		if _, err := ParseMagnet(in); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}
