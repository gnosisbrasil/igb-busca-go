package service

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// ligatures maps typographic ligatures emitted by some PDF fonts to
// their plain-letter expansions.
var ligatures = strings.NewReplacer(
	"ﬁ", "fi",
	"ﬂ", "fl",
	"ﬀ", "ff",
	"ﬃ", "ffi",
	"ﬄ", "ffl",
	"ﬅ", "st",
	"ﬆ", "st",
)

// hyphenBreak matches a word split by a hyphen at a line break, either
// an ASCII hyphen ("palavra-\ncontinuação") or a soft hyphen
// ("precipitan\xad\ndose" as stored from PDFs). The join only happens
// when the next line starts with a lowercase letter, so "livro-\nNovo"
// (dash use) and "página-\n2024" are left alone.
var hyphenBreak = regexp.MustCompile("[\u00ad-][ \t]*\r?\n[ \t]*(\\p{Ll})")

// blankRuns collapses 3+ newlines into a paragraph break.
var blankRuns = regexp.MustCompile(`\n{3,}`)

// CleanPageText normalizes one page of extracted PDF text so search
// matches what the reader sees:
//
//   - Unicode NFC composition (é as one rune, not e + accent)
//   - soft hyphens (U+00AD) removed: "do\xadse" -> "dose"
//   - hyphenated line breaks joined: "precipitan-\ndo" -> "precipitando"
//   - PDF ligatures expanded: "ﬁ" -> "fi"
//   - non-breaking spaces and \r removed, blank lines collapsed,
//     trailing spaces trimmed
//
// It is idempotent (CleanPageText(CleanPageText(s)) == CleanPageText(s)),
// so reprocessing already-cleaned pages is a no-op. Single quotes are
// NOT stripped here; call sites keep the historical ReplaceAll for that.
func CleanPageText(s string) string {
	s = norm.NFC.String(s)
	s = ligatures.Replace(s)
	s = hyphenBreak.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "\u00ad", "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	s = strings.Join(lines, "\n")
	s = blankRuns.ReplaceAllString(s, "\n\n")
	return strings.Trim(s, "\n")
}
