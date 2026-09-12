package rag

import "strings"

const (
	chunkTarget  = 1000
	chunkOverlap = 150
)

// Chunk is one retrieval unit: a slice of a document's text with the
// metadata the index and the citations need.
type Chunk struct {
	DocumentID string
	DocTitle   string
	Index      int
	Content    string
}

// ChunkText splits text into ~1000-char chunks with ~150-char overlap,
// preferring paragraph boundaries (blocks separated by blank lines,
// markdown headings always starting a block). A block longer than the
// target is hard-split at word boundaries, and consecutive windows of
// such a block share the overlap tail. Chunks come back in reading
// order with sequential Index values.
func ChunkText(docID, title, text string) []Chunk {
	var out []Chunk
	emit := func(content string) {
		if s := strings.TrimSpace(content); s != "" {
			out = append(out, Chunk{DocumentID: docID, DocTitle: title, Index: len(out), Content: s})
		}
	}
	var cur strings.Builder
	var carry string // overlap tail carried into the next chunk after a hard split
	flush := func() {
		emit(cur.String())
		cur.Reset()
	}
	for _, block := range blocks(text) {
		for len(block) > chunkTarget {
			flush()
			window, rest := wordWindow(block, chunkTarget-chunkOverlap)
			if carry != "" {
				emit(carry + " " + window)
			} else {
				emit(window)
			}
			carry = lastWords(window, chunkOverlap)
			block = rest
		}
		if carry != "" {
			cur.WriteString(carry + "\n\n")
			carry = ""
		}
		if cur.Len() > 0 && cur.Len()+len(block)+2 > chunkTarget {
			flush()
		}
		cur.WriteString(block)
		cur.WriteString("\n\n")
	}
	flush()
	return out
}

// blocks splits on blank lines; a markdown heading line always starts a
// new block so headings stay attached to the body that follows them.
func blocks(text string) []string {
	var out []string
	for _, para := range strings.Split(text, "\n\n") {
		var cur string
		for _, ln := range strings.Split(strings.TrimSpace(para), "\n") {
			trimmed := strings.TrimSpace(ln)
			if strings.HasPrefix(trimmed, "#") {
				if cur != "" {
					out = append(out, cur)
				}
				cur = trimmed
				continue
			}
			if cur == "" {
				cur = trimmed
				continue
			}
			cur += "\n" + trimmed
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

// wordWindow cuts s at a word boundary at or before n chars, returning
// the window and the rest.
func wordWindow(s string, n int) (string, string) {
	if n >= len(s) {
		return s, ""
	}
	cut := n
	for cut > 0 && s[cut] != ' ' && s[cut] != '\n' {
		cut--
	}
	if cut == 0 {
		cut = n // no whitespace to respect: hard cut
	}
	return strings.TrimSpace(s[:cut]), strings.TrimSpace(s[cut:])
}

// lastWords returns the last ~n chars of s, extended forward to the end
// of the word it lands in so the next chunk resumes at a word start.
func lastWords(s string, n int) string {
	if n >= len(s) {
		return s
	}
	start := len(s) - n
	for start < len(s) && s[start] != ' ' && s[start] != '\n' {
		start++
	}
	return strings.TrimSpace(s[start:])
}
