package rag

import (
	"fmt"
	"strings"
	"testing"
)

func TestChunkSectionsBudgets(t *testing.T) {
	text := strings.Repeat("This is a sentence about retrieval. ", 200) // ~7400 chars ≈ 1850 tokens
	got := ChunkSections("d1", "t", []Section{{Heading: "H", Text: text}}, 512, 64)
	if len(got) < 3 {
		t.Fatalf("want >=3 chunks, got %d", len(got))
	}
	for i, c := range got {
		if c.Index != i || c.DocumentID != "d1" || c.DocTitle != "t" {
			t.Fatalf("meta %+v", c)
		}
		if c.Heading != "H" {
			t.Fatalf("heading lost: %+v", c)
		}
		if c.Tokens > 512+32 { // small slack for one oversize sentence
			t.Fatalf("chunk %d over budget: %d tokens", i, c.Tokens)
		}
		if c.Tokens != EstTokens(c.Content) {
			t.Fatalf("chunk %d tokens not estimated: %+v", i, c)
		}
	}
}

func TestChunkSectionsOverlap(t *testing.T) {
	text := strings.Repeat("alpha beta gamma delta epsilon. ", 120)
	got := ChunkSections("d", "t", []Section{{Text: text}}, 128, 32)
	if len(got) < 2 {
		t.Fatalf("want multiple chunks, got %d", len(got))
	}
	words0 := strings.Fields(got[0].Content)
	tail := strings.Join(words0[len(words0)-8:], " ")
	if !strings.Contains(got[1].Content, tail) {
		t.Fatalf("no overlap: %q not in %q", tail, got[1].Content)
	}
}

func TestChunkSectionsSentenceBoundary(t *testing.T) {
	sents := []string{}
	for i := 0; i < 40; i++ {
		sents = append(sents, fmt.Sprintf("Sentence number %d ends here.", i))
	}
	got := ChunkSections("d", "t", []Section{{Text: strings.Join(sents, " ")}}, 64, 16)
	for _, c := range got[:len(got)-1] {
		if !strings.HasSuffix(c.Content, ".") {
			t.Fatalf("mid-sentence split: %q", c.Content)
		}
	}
}

func TestChunkSectionsPageAndHeading(t *testing.T) {
	secs := []Section{{Heading: "A", Page: 1, Text: "first"}, {Heading: "B", Page: 2, Text: "second"}}
	got := ChunkSections("d", "t", secs, 512, 64)
	if len(got) != 2 || got[0].Page != 1 || got[1].Page != 2 || got[0].Heading != "A" || got[1].Heading != "B" {
		t.Fatalf("sections: %+v", got)
	}
}

func TestChunkTextCompat(t *testing.T) {
	got := ChunkText("d", "t", "hello world")
	if len(got) != 1 || got[0].Content != "hello world" || got[0].Tokens != EstTokens("hello world") {
		t.Fatalf("compat: %+v", got)
	}
	if len(ChunkText("d", "t", "")) != 0 {
		t.Fatal("empty text must yield no chunks")
	}
}
