package core

import "regexp"

// Language detection from release titles. Order matters: VOSTFR must be
// tested before FRENCH, MULTI before the plain French tags.
var languageRules = []struct {
	re   *regexp.Regexp
	lang Language
}{
	{regexp.MustCompile(`(?i)\bVOSTFR\b`), LangVOSTFR},
	{regexp.MustCompile(`(?i)\bMULTI\b`), LangMulti},
	{regexp.MustCompile(`(?i)\b(?:TRUEFRENCH|FRENCH|VFF|VFQ|VF2|VFI|VF)\b`), LangFR},
	{regexp.MustCompile(`(?i)\b(?:ENGLISH|VO)\b`), LangEN},
}

// DetectLanguage returns the language tag of a title, or "" when none.
func DetectLanguage(title string) Language {
	for _, r := range languageRules {
		if r.re.MatchString(title) {
			return r.lang
		}
	}
	return ""
}
