package core

import "testing"

func TestDetectLanguage(t *testing.T) {
	cases := map[string]Language{
		"Dune.Part.Two.2024.MULTI.1080p.WEB.H265-XYZ": LangMulti,
		"Dune Part Two 2024 MULTi VFF 1080p":          LangMulti,
		"Show.S01E01.VOSTFR.1080p.WEB":                LangVOSTFR,
		"Show S01 VOSTFR FRENCH SUBS":                 LangVOSTFR,
		"Film.2023.FRENCH.BluRay.1080p":               LangFR,
		"Film.2023.TRUEFRENCH.1080p":                  LangFR,
		"Film 2023 VFQ 720p":                          LangFR,
		"Film 2023 VF2 720p":                          LangFR,
		"Film 2023 VF 720p":                           LangFR,
		"Film.2023.ENGLISH.1080p":                     LangEN,
		"Film.2023.VO.1080p":                          LangEN,
		"Film 2023 1080p x264-GRP":                    "",
		"Vfx Studio Documentary":                      "",
		"The.Frenchman.2020.1080p":                    "",
	}
	for title, want := range cases {
		if got := DetectLanguage(title); got != want {
			t.Errorf("%q: got %q, want %q", title, got, want)
		}
	}
}
