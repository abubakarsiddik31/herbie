package rag

import (
	"strings"
	"testing"
)

func TestChunkCountsAndMeta(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		wantMin     int
		wantMax     int
		wantMaxSize int
	}{
		{"empty", "", 0, 0, 0},
		{"short", "hello world", 1, 1, chunkTarget + 1},
		{"paragraphs pack", strings.Repeat("para body\n\n", 30), 1, 2, chunkTarget + 1},
		{"long single paragraph", strings.Repeat("word ", 500), 2, 5, chunkTarget + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ChunkText("d1", "title", tc.text)
			if len(got) < tc.wantMin || len(got) > tc.wantMax {
				t.Fatalf("chunk count = %d, want [%d,%d]", len(got), tc.wantMin, tc.wantMax)
			}
			for i, c := range got {
				if c.Index != i || c.DocumentID != "d1" || c.DocTitle != "title" {
					t.Fatalf("meta wrong at %d: %+v", i, c)
				}
				if strings.TrimSpace(c.Content) == "" {
					t.Fatalf("empty content at %d", i)
				}
				if len(c.Content) > tc.wantMaxSize {
					t.Fatalf("chunk %d too large: %d", i, len(c.Content))
				}
			}
		})
	}
}

func TestChunkOverlapBetweenSplits(t *testing.T) {
	text := strings.Repeat("alpha ", 500) // 3000 chars, single block → hard splits
	got := ChunkText("d", "t", text)
	if len(got) < 3 {
		t.Fatalf("want ≥3 chunks, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		prev := strings.Fields(got[i-1].Content)
		cur := strings.Join(strings.Fields(got[i].Content), " ")
		lastTwo := strings.Join(prev[len(prev)-2:], " ")
		if !strings.HasPrefix(cur, lastTwo) {
			t.Fatalf("chunk %d does not resume from the tail of chunk %d", i, i-1)
		}
	}
}

func TestChunkHeadingKeptWithBody(t *testing.T) {
	got := ChunkText("d", "t", "# Title\n\n"+strings.Repeat("x", 400))
	if len(got) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(got))
	}
	if !strings.HasPrefix(got[0].Content, "# Title") {
		t.Fatalf("heading lost: %q", got[0].Content[:20])
	}
}

func TestChunkHeadingStartsNewChunk(t *testing.T) {
	// A full paragraph followed by a heading block: packing must flush the
	// first chunk and start the next one AT the heading, never mid-heading.
	text := strings.Repeat("filler ", 141) + "\n\n# Section\nbody text"
	got := ChunkText("d", "t", text)
	if len(got) != 2 {
		t.Fatalf("want 2 chunks, got %d: %+v", len(got), got)
	}
	if !strings.HasPrefix(got[1].Content, "# Section") {
		t.Fatalf("second chunk should start with heading: %q", got[1].Content)
	}
}
