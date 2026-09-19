package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestListDocumentsToolSchema(t *testing.T) {
	tl := ListDocumentsTool()
	if tl.Name != ListDocumentsToolName {
		t.Fatalf("name %q, want %q", tl.Name, ListDocumentsToolName)
	}
	var schema map[string]any
	if err := json.Unmarshal(tl.Schema, &schema); err != nil {
		t.Fatal(err)
	}
	if _, ok := schema["additionalProperties"]; ok {
		t.Fatal("Gemini rejects additionalProperties in tool schemas")
	}
	props := schema["properties"].(map[string]any)
	if props["limit"] == nil {
		t.Fatal("missing limit property")
	}
}

func TestListDocumentsToolExec(t *testing.T) {
	deps := Deps{
		ListDocs: func(_ context.Context) ([]DocumentInfo, error) {
			return []DocumentInfo{
				{
					ID:         "doc-1",
					Filename:   "architecture.md",
					Mime:       "text/markdown",
					SizeBytes:  2048,
					Status:     "ready",
					ChunkCount: 4,
				},
				{
					ID:         "doc-2",
					Filename:   "budget.pdf",
					Mime:       "application/pdf",
					SizeBytes:  1048576,
					Status:     "ready",
					ChunkCount: 15,
				},
			}, nil
		},
	}

	out, err := ListDocumentsTool().Exec(context.Background(), deps, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "Uploaded documents (2 total):") {
		t.Fatalf("header missing: %q", out.Text)
	}
	if !strings.Contains(out.Text, "[id: doc-1] architecture.md") || !strings.Contains(out.Text, "Chunks: 4 | Size: 2.0 KB") {
		t.Fatalf("doc-1 missing or formatted incorrectly: %q", out.Text)
	}
	if !strings.Contains(out.Text, "[id: doc-2] budget.pdf") || !strings.Contains(out.Text, "Chunks: 15 | Size: 1.0 MB") {
		t.Fatalf("doc-2 missing or formatted incorrectly: %q", out.Text)
	}
	if !strings.Contains(out.Text, "Usage Guide:") || !strings.Contains(out.Text, "read_document") {
		t.Fatalf("usage guide missing in list_documents: %q", out.Text)
	}
}

func TestListDocumentsToolEmpty(t *testing.T) {
	deps := Deps{
		ListDocs: func(_ context.Context) ([]DocumentInfo, error) {
			return []DocumentInfo{}, nil
		},
	}
	out, err := ListDocumentsTool().Exec(context.Background(), deps, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "No uploaded documents found." {
		t.Fatalf("unexpected empty text: %q", out.Text)
	}

	nilDeps := Deps{ListDocs: nil}
	outNil, err := ListDocumentsTool().Exec(context.Background(), nilDeps, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if outNil.Text != "No documents are available." {
		t.Fatalf("unexpected nil text: %q", outNil.Text)
	}
}

func TestListDocumentsToolLimit(t *testing.T) {
	deps := Deps{
		ListDocs: func(_ context.Context) ([]DocumentInfo, error) {
			return []DocumentInfo{
				{ID: "d1", Filename: "f1.txt", SizeBytes: 100, Status: "ready", ChunkCount: 1},
				{ID: "d2", Filename: "f2.txt", SizeBytes: 100, Status: "ready", ChunkCount: 1},
				{ID: "d3", Filename: "f3.txt", SizeBytes: 100, Status: "ready", ChunkCount: 1},
			}, nil
		},
	}
	out, err := ListDocumentsTool().Exec(context.Background(), deps, json.RawMessage(`{"limit": 2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "[id: d1]") || !strings.Contains(out.Text, "[id: d2]") {
		t.Fatalf("d1 or d2 missing: %q", out.Text)
	}
	if strings.Contains(out.Text, "[id: d3]") {
		t.Fatalf("d3 should have been limited: %q", out.Text)
	}
	if !strings.Contains(out.Text, "...and 1 more documents.") {
		t.Fatalf("missing more documents line: %q", out.Text)
	}
}
