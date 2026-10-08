package service

import "strings"

// hyphenReplacer strips hyphen-like characters so search is
// hyphen-insensitive on both sides:
//
//   - U+002D hyphen-minus ("contraindo-os", as users type)
//   - U+00AD soft hyphen ("contraindo\u00ados", as stored from PDFs)
//   - U+2010/U+2011 hyphen and non-breaking hyphen
//
// En/em dashes are NOT stripped: they separate clauses, not syllables.
// Regex/wildcard characters (%, _, .) are preserved.
var hyphenReplacer = strings.NewReplacer(
	"-", "",
	"\u00ad", "",
	"\u2010", "",
	"\u2011", "",
)

// StripHyphens removes hyphen-like characters from a search term.
// The pages column gets the same treatment in SQL (regexp_replace),
// so "contraindo-os" matches "contraindoos", "contraindo-os" and
// "contraindo<soft-hyphen>os" alike.
func StripHyphens(s string) string {
	return hyphenReplacer.Replace(s)
}
