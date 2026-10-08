package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildTestPDF writes a minimal valid PDF with one text line per page.
func buildTestPDF(t *testing.T, lines []string) string {
	t.Helper()
	var body strings.Builder
	offsets := []int{}
	addObj := func(n int, content string) {
		offsets = append(offsets, body.Len())
		fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", n, content)
	}

	body.WriteString("%PDF-1.4\n")
	kids := []string{}
	obj := 3
	pageObjs := []int{}
	for range lines {
		pageObjs = append(pageObjs, obj)
		obj += 2
	}
	for _, p := range pageObjs {
		kids = append(kids, fmt.Sprintf("%d 0 R", p))
	}
	fontObj := obj
	addObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	addObj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(lines)))
	for i, line := range lines {
		escaped := strings.ReplaceAll(strings.ReplaceAll(line, `\`, `\\`), ")", `\)`)
		stream := fmt.Sprintf("BT /F1 24 Tf 100 700 Td (%s) Tj ET", escaped)
		addObj(pageObjs[i], fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>", pageObjs[i]+1, fontObj))
		addObj(pageObjs[i]+1, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	addObj(fontObj, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	xrefPos := body.Len()
	total := fontObj + 1
	fmt.Fprintf(&body, "xref\n0 %d\n", total)
	body.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&body, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&body, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", total, xrefPos)

	path := filepath.Join(t.TempDir(), "test.pdf")
	if err := os.WriteFile(path, []byte(body.String()), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractPDFPages(t *testing.T) {
	path := buildTestPDF(t, []string{"Hello World", "Segunda pagina"})
	texts, err := extractPDFPages(path)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(texts) != 2 {
		t.Fatalf("got %d pages, want 2", len(texts))
	}
	if !strings.Contains(texts[0], "Hello") || !strings.Contains(texts[0], "World") {
		t.Fatalf("page 1 text = %q", texts[0])
	}
	if !strings.Contains(texts[1], "Segunda") {
		t.Fatalf("page 2 text = %q", texts[1])
	}
}

func TestExtractPDFPagesMissing(t *testing.T) {
	if _, err := extractPDFPages(filepath.Join(t.TempDir(), "nope.pdf")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
