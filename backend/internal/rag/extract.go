package rag

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/abubakarsiddik31/golem/docextract"
	"github.com/abubakarsiddik31/golem/pdfextract"
)

const (
	docxMime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	xlsxMime = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	pptxMime = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
)

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
// section; markdown splits on ATX headings; PDFs split per page; office
// documents (docx, xlsx, pptx) split per heading, sheet, or slide.
func ExtractSections(mime string, r io.Reader) ([]Section, error) {
	return ExtractSectionsWithFilename(mime, "", r)
}

// ExtractSectionsWithFilename pulls headed spans out of an upload using filename and MIME hints.
func ExtractSectionsWithFilename(mime, filename string, r io.Reader) ([]Section, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read document: %w", err)
	}
	if len(buf) == 0 {
		return nil, nil
	}

	format := resolveFormat(buf, filename, mime)
	switch format {
	case docextract.FormatPDF:
		return pdfSections(buf)
	case docextract.FormatDocx, docextract.FormatXlsx, docextract.FormatPptx, docextract.FormatCSV, docextract.FormatTSV:
		doc, err := docextract.ExtractBytes(context.Background(), buf, filenameForFormat(filename, format), docextract.Options{})
		if err != nil {
			return nil, fmt.Errorf("extract %s: %w", format, err)
		}
		return markdownSections(doc.Content), nil
	case docextract.FormatMarkdown:
		return markdownSections(string(buf)), nil
	case docextract.FormatText:
		return []Section{{Text: string(buf)}}, nil
	default:
		return nil, fmt.Errorf("unsupported mime %q", mime)
	}
}

// ExtractText pulls plain or structured markdown text out of an uploaded document.
func ExtractText(mime string, r io.Reader) (string, error) {
	return ExtractTextWithFilename(mime, "", r)
}

// ExtractTextWithFilename pulls plain or structured markdown text out of an uploaded document using filename and MIME hints.
func ExtractTextWithFilename(mime, filename string, r io.Reader) (string, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read document: %w", err)
	}
	if len(buf) == 0 {
		return "", nil
	}

	format := resolveFormat(buf, filename, mime)
	switch format {
	case docextract.FormatPDF:
		pdfDoc, err := pdfextract.ExtractBytes(context.Background(), buf, pdfextract.Options{})
		if err != nil {
			return "", fmt.Errorf("open pdf: %w", err)
		}
		return strings.TrimSpace(pdfDoc.Markdown), nil
	case docextract.FormatDocx, docextract.FormatXlsx, docextract.FormatPptx, docextract.FormatCSV, docextract.FormatTSV, docextract.FormatMarkdown:
		doc, err := docextract.ExtractBytes(context.Background(), buf, filenameForFormat(filename, format), docextract.Options{})
		if err != nil {
			return "", fmt.Errorf("extract %s: %w", format, err)
		}
		return strings.TrimSpace(doc.Content), nil
	case docextract.FormatText:
		return string(buf), nil
	default:
		return "", fmt.Errorf("unsupported mime %q", mime)
	}
}

func resolveFormat(buf []byte, filename, mime string) docextract.Format {
	if mime == "application/zip" {
		return docextract.FormatUnknown
	}

	if filename != "" {
		detected := docextract.DetectFormat(buf, filename)
		if detected != docextract.FormatUnknown {
			return detected
		}
	}

	switch strings.ToLower(mime) {
	case "application/pdf":
		return docextract.FormatPDF
	case docxMime, "application/docx", "application/msword":
		return docextract.FormatDocx
	case xlsxMime, "application/xlsx", "application/vnd.ms-excel":
		return docextract.FormatXlsx
	case pptxMime, "application/pptx", "application/vnd.ms-powerpoint":
		return docextract.FormatPptx
	case "text/markdown", "text/x-markdown":
		return docextract.FormatMarkdown
	case "text/csv":
		return docextract.FormatCSV
	case "text/tab-separated-values", "text/tsv":
		return docextract.FormatTSV
	case "text/plain", "application/json", "application/xml", "application/x-yaml", "text/html":
		return docextract.FormatText
	}

	if strings.HasPrefix(mime, "text/") {
		return docextract.FormatText
	}

	if mime != "" {
		sniffed := docextract.DetectFormat(buf, filename)
		if sniffed != docextract.FormatUnknown {
			return sniffed
		}
	}

	return docextract.FormatUnknown
}

func filenameForFormat(filename string, format docextract.Format) string {
	if filename != "" {
		return filename
	}
	switch format {
	case docextract.FormatDocx:
		return "doc.docx"
	case docextract.FormatXlsx:
		return "sheet.xlsx"
	case docextract.FormatPptx:
		return "deck.pptx"
	case docextract.FormatPDF:
		return "doc.pdf"
	case docextract.FormatCSV:
		return "data.csv"
	case docextract.FormatTSV:
		return "data.tsv"
	case docextract.FormatMarkdown:
		return "notes.md"
	default:
		return "file.txt"
	}
}

func pdfSections(buf []byte) ([]Section, error) {
	doc, err := pdfextract.ExtractBytes(context.Background(), buf, pdfextract.Options{})
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	var out []Section
	for _, p := range doc.Pages {
		trimmed := strings.TrimSpace(p.Markdown)
		if trimmed == "" {
			continue
		}
		secs := markdownSections(trimmed)
		if len(secs) == 0 {
			out = append(out, Section{Page: p.Index + 1, Text: trimmed})
		} else {
			for i := range secs {
				secs[i].Page = p.Index + 1
				out = append(out, secs[i])
			}
		}
	}
	if len(out) == 0 && strings.TrimSpace(doc.Markdown) != "" {
		secs := markdownSections(strings.TrimSpace(doc.Markdown))
		for i := range secs {
			secs[i].Page = 1
			out = append(out, secs[i])
		}
	}
	return out, nil
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
