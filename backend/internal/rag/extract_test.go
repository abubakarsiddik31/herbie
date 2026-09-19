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
	withNull, err := ExtractText("text/plain", strings.NewReader("hello\x00world"))
	if err != nil {
		t.Fatal(err)
	}
	if withNull != "helloworld" {
		t.Fatalf("expected null byte removed, got: %q", withNull)
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

func TestExtractSectionsMarkdown(t *testing.T) {
	got, err := ExtractSections("text/markdown", strings.NewReader("# Intro\nbody one\n\n## Deep\nbody two"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("sections: %+v", got)
	}
	if got[0].Heading != "Intro" || !strings.Contains(got[0].Text, "body one") {
		t.Fatalf("s0: %+v", got[0])
	}
	if got[1].Heading != "Deep" || got[1].Page != 0 {
		t.Fatalf("s1: %+v", got[1])
	}
}

func TestExtractSectionsPlain(t *testing.T) {
	got, err := ExtractSections("text/plain", strings.NewReader("just text"))
	if err != nil || len(got) != 1 || got[0].Heading != "" || got[0].Text != "just text" {
		t.Fatalf("plain: %+v %v", got, err)
	}
}

func TestExtractSectionsPDFPages(t *testing.T) {
	f, err := os.Open("testdata/tiny.pdf")
	if err != nil {
		t.Fatal("run TestMainPDFFixture first to generate it")
	}
	defer f.Close()
	got, err := ExtractSections("application/pdf", f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Page != 1 || !strings.Contains(got[0].Text, "Hello golem chatbot") {
		t.Fatalf("pdf sections: %+v", got)
	}
}

func TestExtractSectionsDocxHeadings(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	w.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Title One</w:t></w:r></w:p><w:p><w:r><w:t>para text</w:t></w:r></w:p></w:body></w:document>`))
	zw.Close()
	got, err := ExtractSections("application/vnd.openxmlformats-officedocument.wordprocessingml.document", &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Heading != "Title One" || !strings.Contains(got[0].Text, "para text") {
		t.Fatalf("docx sections: %+v", got)
	}
}

func TestExtractCSV(t *testing.T) {
	csvData := "Name,Role,Score\nAlice,Engineer,95\nBob,Designer,88"
	got, err := ExtractText("text/csv", strings.NewReader(csvData))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "| Name | Role | Score |") || !strings.Contains(got, "| Alice | Engineer | 95 |") {
		t.Fatalf("csv text: %q", got)
	}

	secs, err := ExtractSections("text/csv", strings.NewReader(csvData))
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 1 || !strings.Contains(secs[0].Text, "Alice") {
		t.Fatalf("csv sections: %+v", secs)
	}
}

func TestExtractTSV(t *testing.T) {
	tsvData := "Item\tPrice\nApple\t1.50\nBanana\t0.75"
	got, err := ExtractText("text/tab-separated-values", strings.NewReader(tsvData))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "| Item | Price |") || !strings.Contains(got, "| Apple | 1.50 |") {
		t.Fatalf("tsv text: %q", got)
	}
}

func TestExtractXLSX(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	wb, _ := zw.Create("xl/workbook.xml")
	wb.Write([]byte(`<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sales" sheetId="1" r:id="rId1"/></sheets></workbook>`))
	rels, _ := zw.Create("xl/_rels/workbook.xml.rels")
	rels.Write([]byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`))
	ws, _ := zw.Create("xl/worksheets/sheet1.xml")
	ws.Write([]byte(`<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Revenue</t></is></c><c r="B1"><v>5000</v></c></row></sheetData></worksheet>`))
	zw.Close()

	got, err := ExtractText(xlsxMime, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Revenue") || !strings.Contains(got, "5000") {
		t.Fatalf("xlsx text: %q", got)
	}

	secs, err := ExtractSections(xlsxMime, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) == 0 || !strings.Contains(secs[0].Heading, "Sales") {
		t.Fatalf("xlsx sections: %+v", secs)
	}
}

func TestExtractPPTX(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	pres, _ := zw.Create("ppt/presentation.xml")
	pres.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst>
</p:presentation>`))
	rels, _ := zw.Create("ppt/_rels/presentation.xml.rels")
	rels.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>
</Relationships>`))
	slide, _ := zw.Create("ppt/slides/slide1.xml")
	slide.Write([]byte(`<?xml version="1.0"?><p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>Quarterly Review</a:t></a:r></a:p></p:txBody></p:sp><p:sp><p:txBody><a:p><a:r><a:t>Key takeaways</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`))
	zw.Close()

	got, err := ExtractText(pptxMime, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Quarterly Review") || !strings.Contains(got, "Key takeaways") {
		t.Fatalf("pptx text: %q", got)
	}

	secs, err := ExtractSections(pptxMime, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) == 0 || !strings.Contains(secs[0].Heading, "Quarterly Review") {
		t.Fatalf("pptx sections: %+v", secs)
	}
}

func TestEstTokens(t *testing.T) {
	if EstTokens("") != 1 || EstTokens("abcd") != 1 || EstTokens("abcdefgh") != 2 {
		t.Fatalf("est: %d %d %d", EstTokens(""), EstTokens("abcd"), EstTokens("abcdefgh"))
	}
}
