package service

import (
	"strings"
	"testing"
)

func TestCleanPageText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "alma livre", "alma livre"},
		{"soft hyphen", "do\u00adse", "dose"},
		{"hyphen break", "precipitan-\ndo\u00adse", "precipitandose"},
		{"hyphen break CRLF", "palavra-\r\ncontinuação", "palavracontinuação"},
		{"hyphen keeps uppercase", "livro-\nNovo capítulo", "livro-\nNovo capítulo"},
		{"hyphen keeps digit", "página-\n2024", "página-\n2024"},
		{"mid-line hyphen kept", "bem-estar", "bem-estar"},
		{"ligatures", "ﬁnal ﬂuxo ﬀ ﬃ ﬄ ﬅ ﬆ", "final fluxo ff ffi ffl st st"},
		{"nbsp", "a\u00a0b", "a b"},
		{"crlf", "a\r\nb\rc", "a\nb\nc"},
		{"blank collapse", "a\n\n\n\nb", "a\n\nb"},
		{"trailing spaces", "a   \nb\t", "a\nb"},
		{"nfc compose", "e\u0301", "é"},
		{"leading trailing newlines", "\n\nalma\n\n", "alma"},
		{"quotes preserved", "d'alma", "d'alma"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CleanPageText(tc.in); got != tc.want {
				t.Fatalf("CleanPageText(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Idempotency: reprocessing cleaned text is a no-op.
			if again := CleanPageText(tc.want); again != tc.want {
				t.Fatalf("CleanPageText not idempotent: %q -> %q", tc.want, again)
			}
		})
	}
}

func TestCleanPageTextRealSample(t *testing.T) {
	in := "22 | sAmAEl Aun WEor\r\nresultaram inúteis.\r\nÉ necessário saber que o órgão Kundartiguador é o fogo\r\ndesenvolvido negativamente; a serpente baixando, precipitan\u00ad\r\ndo\u00adse desde o cóccix até os infernos atômicos do homem."
	got := CleanPageText(in)
	if !strings.Contains(got, "precipitandose") {
		t.Fatalf("hyphen break not joined: %q", got)
	}
	if strings.Contains(got, "\r") || strings.Contains(got, "\u00ad") {
		t.Fatalf("control/invisible chars remain: %q", got)
	}
	if !strings.Contains(got, "órgão") || !strings.Contains(got, "cóccix") {
		t.Fatalf("accents damaged: %q", got)
	}
}
