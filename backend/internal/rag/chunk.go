package rag

import "strings"

// Chunk is one retrieval unit: a slice of a document's text with the
// metadata the index and the citations need.
type Chunk struct {
	DocumentID string
	DocTitle   string
	Index      int
	Content    string
	Heading    string // nearest section heading, "" when none
	Page       int    // 1-based for PDFs, 0 otherwise
	Tokens     int    // EstTokens(Content) at build time
}

// splitSentences cuts s on sentence boundaries (. ! ? or newline),
// keeping the delimiter. A fragment with no delimiter is one sentence.
func splitSentences(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == '!' || s[i] == '?' || s[i] == '\n' {
			frag := strings.TrimSpace(s[start : i+1])
			if frag != "" {
				out = append(out, frag)
			}
			start = i + 1
		}
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

// splitUnits returns the recursive split ladder for one section body:
// paragraphs, then sentences inside oversize paragraphs, then words.
func splitUnits(text string, targetTokens int) []string {
	var out []string
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if EstTokens(para) <= targetTokens {
			out = append(out, para)
			continue
		}
		for _, sent := range splitSentences(para) {
			if EstTokens(sent) <= targetTokens {
				out = append(out, sent)
				continue
			}
			words := strings.Fields(sent)
			cur := ""
			for _, w := range words {
				if cur != "" && EstTokens(cur+" "+w) > targetTokens {
					out = append(out, cur)
					cur = w
					continue
				}
				if cur != "" {
					cur += " "
				}
				cur += w
			}
			if cur != "" {
				out = append(out, cur)
			}
		}
	}
	return out
}

// overlapTail returns roughly the last overlapTokens of s, cut at a word start.
func overlapTail(s string, overlapTokens int) string {
	n := overlapTokens * 4
	if n >= len(s) {
		return s
	}
	i := strings.LastIndexAny(s[:len(s)-n+1], " \n")
	if i < 0 {
		return s[len(s)-n:]
	}
	return strings.TrimSpace(s[i+1:])
}

// ChunkSections splits sections into token-budgeted chunks with word-safe
// overlap. Units never cross a section boundary, so Heading/Page stay exact.
// Units inside a chunk are space-joined so word overlap across chunk
// boundaries stays contiguous (an 8-word tail of one chunk is a literal
// substring of the next). The budget check measures the joined content
// exactly, separators included, so Tokens never overshoots targetTokens by
// more than one unit.
func ChunkSections(docID, title string, sections []Section, targetTokens, overlapTokens int) []Chunk {
	var out []Chunk
	for _, sec := range sections {
		var cur []string
		flush := func() {
			if len(cur) == 0 {
				return
			}
			content := strings.Join(cur, " ")
			out = append(out, Chunk{
				DocumentID: docID, DocTitle: title, Index: len(out),
				Content: content, Heading: sec.Heading, Page: sec.Page,
				Tokens: EstTokens(content),
			})
			cur = nil
		}
		for _, u := range splitUnits(sec.Text, targetTokens) {
			if len(cur) > 0 && EstTokens(strings.Join(cur, " ")+" "+u) > targetTokens {
				flush()
				if overlapTokens > 0 && len(out) > 0 {
					cur = []string{overlapTail(out[len(out)-1].Content, overlapTokens)}
				}
			}
			cur = append(cur, u)
		}
		flush()
	}
	return out
}

// ChunkText is the single-section entry point with the default 512/64
// token budget. Service wiring moves to ChunkSections in the pipeline task.
func ChunkText(docID, title, text string) []Chunk {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return ChunkSections(docID, title, []Section{{Text: text}}, 512, 64)
}
