package service

import "testing"

func TestStripHyphens(t *testing.T) {
	cases := []struct{ in, want string }{
		{"alma", "alma"},
		{"contraindo-os", "contraindoos"},
		{"dilatandoos", "dilatandoos"},
		{"a-b-c", "abc"},
		{"bem–estar", "bem–estar"},   // en dash kept
		{"bem—estar", "bem—estar"},   // em dash kept
		{"alma+livre", "alma+livre"}, // wildcard kept
		{`"alma"`, `"alma"`},         // quotes kept
		{"100%certo", "100%certo"},   // LIKE wildcard kept
	}
	for _, tc := range cases {
		if got := StripHyphens(tc.in); got != tc.want {
			t.Fatalf("StripHyphens(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Invisible hyphen-likes, written as escapes.
	escapes := []struct{ in, want string }{
		{"do\u00adse", "dose"},   // soft hyphen
		{"a\u2010b", "ab"},       // U+2010
		{"a\u2011b", "ab"},       // U+2011
		{"a\u00a0b", "a\u00a0b"}, // NBSP kept
	}
	for _, tc := range escapes {
		if got := StripHyphens(tc.in); got != tc.want {
			t.Fatalf("StripHyphens(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
