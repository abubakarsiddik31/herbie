package rag

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	pdftext "github.com/ledongthuc/pdf"
)

const docxMime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// Section is one headed span of a document. Page is 1-based for PDFs,
// 0 when the format has no pages. Heading is "" when unknown.
type Section struct {
	Heading string
	Page    int
	Text    string
}

// EstTokens approximates token count at 4 chars per token, matching the
// ledger's estimate convention. Minimum 1 so empty math never divides.
func EstTokens(s string) int {
	if n := len(s) / 4; n > 1 {
		return n
	}
	return 1
}

// ExtractSections pulls headed spans out of an upload. Plain text is one
// section; markdown splits on ATX headings; PDFs split per page; docx
// splits on Heading-styled paragraphs.
func ExtractSections(mime string, r io.Reader) ([]Section, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read document: %w", err)
	}
	switch mime {
	case "text/plain":
		return []Section{{Text: string(buf)}}, nil
	case "text/markdown":
		return markdownSections(string(buf)), nil
	case "application/pdf":
		return pdfSections(buf)
	case docxMime:
		return docxSections(buf)
	default:
		return nil, fmt.Errorf("unsupported mime %q", mime)
	}
}

// ExtractText pulls plain text out of an uploaded document by joining the
// structured sections with blank lines.
func ExtractText(mime string, r io.Reader) (string, error) {
	sections, err := ExtractSections(mime, r)
	if err != nil {
		return "", err
	}
	texts := make([]string, 0, len(sections))
	for _, s := range sections {
		if s.Text != "" {
			texts = append(texts, s.Text)
		}
	}
	return strings.Join(texts, "\n\n"), nil
}

func markdownSections(text string) []Section {
	var out []Section
	var cur *Section
	flush := func() {
		if cur == nil {
			return
		}
		if cur.Heading == "" && strings.TrimSpace(cur.Text) == "" {
			cur = nil
			return
		}
		out = append(out, *cur)
		cur = nil
	}
	ensure := func() *Section {
		if cur == nil {
			cur = &Section{}
		}
		return cur
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "#") {
			flush()
			cur = &Section{Heading: strings.TrimSpace(strings.TrimLeft(trimmed, "#"))}
			continue
		}
		s := ensure()
		if s.Text != "" {
			s.Text += "\n"
		}
		s.Text += line
	}
	flush()
	for i := range out {
		out[i].Text = strings.TrimSpace(out[i].Text)
	}
	kept := out[:0]
	for _, s := range out {
		if s.Heading == "" && s.Text == "" {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

func pdfSections(buf []byte) ([]Section, error) {
	doc, err := pdftext.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	var out []Section
	for i := 1; i <= doc.NumPage(); i++ {
		p := doc.Page(i)
		if p.V.String() == "" {
			continue
		}
		txt, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		if trimmed := strings.TrimSpace(txt); trimmed != "" {
			out = append(out, Section{Page: i, Text: trimmed})
		}
	}
	return out, nil
}

func docxSections(buf []byte) ([]Section, error) {
	zr, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open document.xml: %w", err)
		}
		defer rc.Close()
		return docxSectionList(rc)
	}
	return nil, fmt.Errorf("docx missing word/document.xml")
}

func docxSectionList(r io.Reader) ([]Section, error) {
	dec := xml.NewDecoder(r)
	var out []Section
	cur := &Section{}
	inText := false
	inPara := false
	paraHeading := false
	var paraText strings.Builder
	flushPara := func() error {
		text := strings.TrimSpace(paraText.String())
		paraText.Reset()
		if paraHeading {
			if cur.Heading != "" || strings.TrimSpace(cur.Text) != "" {
				out = append(out, *cur)
			}
			cur = &Section{Heading: text}
		} else if text != "" {
			if cur.Text != "" {
				cur.Text += "\n"
			}
			cur.Text += text
		}
		paraHeading = false
		inPara = false
		return nil
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse document.xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				inPara = true
				paraHeading = false
				paraText.Reset()
			case "pStyle":
				for _, a := range t.Attr {
					if a.Name.Local == "val" && strings.Contains(a.Value, "Heading") {
						paraHeading = true
					}
				}
			case "t":
				inText = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				if inPara {
					if err := flushPara(); err != nil {
						return nil, err
					}
				}
			}
		case xml.CharData:
			if inText {
				paraText.Write(t)
			}
		}
	}
	if cur.Heading != "" || strings.TrimSpace(cur.Text) != "" {
		out = append(out, *cur)
	}
	for i := range out {
		out[i].Text = strings.TrimSpace(out[i].Text)
	}
	return out, nil
}
