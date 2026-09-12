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

// ExtractText pulls plain text out of an uploaded document: txt/md read
// as UTF-8, PDFs through ledongthuc/pdf (an unreadable page is skipped,
// not fatal), docx via the <w:t> runs of word/document.xml.
func ExtractText(mime string, r io.Reader) (string, error) {
	switch mime {
	case "text/plain", "text/markdown":
		b, err := io.ReadAll(r)
		if err != nil {
			return "", fmt.Errorf("read plain text: %w", err)
		}
		return string(b), nil
	case "application/pdf":
		return extractPDF(r)
	case docxMime:
		return extractDocx(r)
	default:
		return "", fmt.Errorf("unsupported mime %q", mime)
	}
}

func extractPDF(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read pdf: %w", err)
	}
	doc, err := pdftext.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}
	var sb strings.Builder
	for i := 1; i <= doc.NumPage(); i++ {
		p := doc.Page(i)
		if p.V.String() == "" {
			continue
		}
		txt, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(txt)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

func extractDocx(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read docx: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("open docx: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("open document.xml: %w", err)
		}
		defer rc.Close()
		return docxText(rc)
	}
	return "", fmt.Errorf("docx missing word/document.xml")
}

func docxText(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var sb strings.Builder
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse document.xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "p":
				sb.WriteString("\n")
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}
	return strings.TrimSpace(sb.String()), nil
}
