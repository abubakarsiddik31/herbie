package rag

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestPDFFixture writes testdata/tiny.pdf with correct xref offsets so
// TestExtractPDF has a deterministic, dependency-free input.
func TestPDFFixture(t *testing.T) {
	text := "BT /F1 18 Tf 72 720 Td (Hello golem chatbot) Tj ET"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(text), text),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var buf bytes.Buffer
	var offs []int
	buf.WriteString("%PDF-1.4\n")
	for i, o := range objs {
		offs = append(offs, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := buf.Len()
	buf.WriteString(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objs)+1))
	for _, off := range offs {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref))
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/tiny.pdf", buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtractPlain(t *testing.T) {
	got, err := ExtractText("text/plain", strings.NewReader("# md heading\nbody"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "# md heading\nbody" {
		t.Fatalf("plain text mangled: %q", got)
	}
	if _, err := ExtractText("text/markdown", strings.NewReader("x")); err != nil {
		t.Fatalf("markdown must ride the plain path: %v", err)
	}
}

func TestExtractDocx(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>First para</w:t></w:r></w:p><w:p><w:r><w:t>Second</w:t></w:r></w:p></w:body></w:document>`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := ExtractText("application/vnd.openxmlformats-officedocument.wordprocessingml.document", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "First para") || !strings.Contains(got, "Second") {
		t.Fatalf("docx text missing runs: %q", got)
	}
	if strings.Index(got, "First para") > strings.Index(got, "Second") {
		t.Fatalf("docx order wrong: %q", got)
	}
}

func TestExtractPDF(t *testing.T) {
	f, err := os.Open("testdata/tiny.pdf")
	if err != nil {
		t.Fatalf("run TestPDFFixture first: %v", err)
	}
	defer f.Close()
	got, err := ExtractText("application/pdf", f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Hello golem chatbot") {
		t.Fatalf("pdf text: %q", got)
	}
}

func TestExtractUnsupported(t *testing.T) {
	if _, err := ExtractText("application/zip", strings.NewReader("x")); err == nil {
		t.Fatal("want error for unsupported mime")
	}
	if _, err := ExtractText("", strings.NewReader("x")); err == nil {
		t.Fatal("want error for empty mime")
	}
}
