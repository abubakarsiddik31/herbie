package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

func TestReadDocumentToolSchema(t *testing.T) {
	tl := ReadDocumentTool()
	if tl.Name != ReadDocumentToolName {
		t.Fatalf("name %q, want %q", tl.Name, ReadDocumentToolName)
	}
	var schema map[string]any
	if err := json.Unmarshal(tl.Schema, &schema); err != nil {
		t.Fatal(err)
	}
	if _, ok := schema["additionalProperties"]; ok {
		t.Fatal("Gemini rejects additionalProperties in tool schemas")
	}
	props := schema["properties"].(map[string]any)
	if props["documentId"] == nil || props["offset"] == nil || props["limit"] == nil {
		t.Fatal("missing properties")
	}
	req, ok := schema["required"].([]any)
	if !ok || len(req) != 1 || req[0] != "documentId" {
		t.Fatalf("required property should be documentId, got %+v", req)
	}
}

func TestReadDocumentToolExec(t *testing.T) {
	deps := Deps{
		ReadDoc: func(_ context.Context, docID string, offset, limit int) (ReadDocResult, error) {
			if docID != "doc-123" {
				return ReadDocResult{}, errors.New("not found")
			}
			return ReadDocResult{
				DocumentID:   "doc-123",
				DocTitle:     "nature-paper.pdf",
				TotalChunks:  20,
				Offset:       offset,
				Limit:        limit,
				SourceOffset: 0,
				Chunks: []rag.Chunk{
					{
						DocumentID: "doc-123",
						DocTitle:   "nature-paper.pdf",
						Heading:    "Abstract",
						Page:       1,
						Index:      0,
						Content:    "This paper investigates dialect prejudice in AI.",
					},
					{
						DocumentID: "doc-123",
						DocTitle:   "nature-paper.pdf",
						Heading:    "Introduction",
						Page:       1,
						Index:      1,
						Content:    "Recent language models exhibit covert bias.",
					},
				},
			}, nil
		},
	}

	out, err := ReadDocumentTool().Exec(context.Background(), deps, json.RawMessage(`{"documentId":"doc-123","offset":0,"limit":2}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.Text, "Document: nature-paper.pdf (ID: doc-123)") {
		t.Fatalf("header missing: %q", out.Text)
	}
	if !strings.Contains(out.Text, "[1] (nature-paper.pdf § Abstract, p.1 (chunk 0/20))") {
		t.Fatalf("chunk 0 missing: %q", out.Text)
	}
	if !strings.Contains(out.Text, "[2] (nature-paper.pdf § Introduction, p.1 (chunk 1/20))") {
		t.Fatalf("chunk 1 missing: %q", out.Text)
	}
	if !strings.Contains(out.Text, "18 more chunks remaining in this document (chunks 2 to 19)") {
		t.Fatalf("pagination notice missing: %q", out.Text)
	}
}

func TestReadDocumentToolEndOfDocument(t *testing.T) {
	deps := Deps{
		ReadDoc: func(_ context.Context, docID string, offset, limit int) (ReadDocResult, error) {
			return ReadDocResult{
				DocumentID:   "doc-123",
				DocTitle:     "nature-paper.pdf",
				TotalChunks:  5,
				Offset:       3,
				Limit:        2,
				SourceOffset: 2,
				Chunks: []rag.Chunk{
					{
						DocumentID: "doc-123",
						DocTitle:   "nature-paper.pdf",
						Index:      3,
						Content:    "Conclusion section.",
					},
					{
						DocumentID: "doc-123",
						DocTitle:   "nature-paper.pdf",
						Index:      4,
						Content:    "References.",
					},
				},
			}, nil
		},
	}

	out, err := ReadDocumentTool().Exec(context.Background(), deps, json.RawMessage(`{"documentId":"doc-123","offset":3,"limit":2}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.Text, "Reached the end of document \"nature-paper.pdf\" (total chunks: 5).") {
		t.Fatalf("end notice missing: %q", out.Text)
	}
}

func TestReadDocumentToolBeyondBounds(t *testing.T) {
	deps := Deps{
		ReadDoc: func(_ context.Context, docID string, offset, limit int) (ReadDocResult, error) {
			return ReadDocResult{
				DocumentID:  "doc-123",
				DocTitle:    "nature-paper.pdf",
				TotalChunks: 10,
				Offset:      15,
			}, nil
		},
	}

	out, err := ReadDocumentTool().Exec(context.Background(), deps, json.RawMessage(`{"documentId":"doc-123","offset":15}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.Text, "Offset 15 is beyond the total chunk count (10 chunks)") {
		t.Fatalf("unexpected message: %q", out.Text)
	}
}

func TestReadDocumentToolValidation(t *testing.T) {
	tl := ReadDocumentTool()
	deps := Deps{
		ReadDoc: func(_ context.Context, _ string, _, _ int) (ReadDocResult, error) {
			return ReadDocResult{}, nil
		},
	}

	_, err := tl.Exec(context.Background(), deps, json.RawMessage(`{"offset":0}`))
	if err == nil {
		t.Fatal("expected error for missing documentId")
	}
	var retry *model.ModelRetry
	if !errors.As(err, &retry) {
		t.Fatalf("expected ModelRetry, got %v", err)
	}
}

func TestPromptForReadDocument(t *testing.T) {
	got := promptFor(RunSpec{SystemPrompt: "base"}, []tool.Tool[Deps]{ReadDocumentTool()})
	if !strings.Contains(got, "read_document") || !strings.Contains(got, "Summarization workflow") {
		t.Fatalf("guidance lacks read_document summarization instructions: %s", got)
	}
}
