package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// ReadDocumentToolName is the built-in tool for reading sequential chunks of a specific document.
const ReadDocumentToolName = "read_document"

// ReadDocResult carries the read chunks along with document pagination metadata and
// the 0-based offset in the run's cumulative sources list.
type ReadDocResult struct {
	DocumentID   string
	DocTitle     string
	TotalChunks  int
	Offset       int
	Limit        int
	Chunks       []rag.Chunk
	SourceOffset int
}

// ReadDocFunc retrieves sequential chunks from a specific document.
type ReadDocFunc func(ctx context.Context, documentID string, offset, limit int) (ReadDocResult, error)

var readDocumentSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "documentId": {"type": "string", "description": "The ID of the document to read (from list_documents or search_documents results)."},
    "offset": {"type": "integer", "description": "The 0-based chunk index to start reading from (default 0). For document summary or overview, start at offset 0."},
    "limit": {"type": "integer", "description": "The number of sequential chunks to read (default 5, maximum 10)."}
  },
  "required": ["documentId"]
}`)

// ReadDocumentTool provides the agent with the ability to read sequential, bounded sections
// of a specific uploaded document. This is especially useful for document summarization
// (reading offset 0 for abstract/introduction) and multi-hop queries where an outline or
// contiguous text needs to be inspected.
func ReadDocumentTool() tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        ReadDocumentToolName,
		Description: "Read sequential chunks of a specific document by its document ID with pagination (offset and limit). Use this tool to read the beginning of a document (offset 0) to summarize it, inspect chapters in order, or follow references across sections. Bounded to at most 10 chunks per call to protect context. Always cite read passages using their bracket numbers.",
		Schema:      readDocumentSchema,
		Timeout:     30 * time.Second,
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			if deps.ReadDoc == nil {
				return tool.Text("Document reading is not available."), nil
			}
			var a struct {
				DocumentID string `json:"documentId"`
				Offset     int    `json:"offset"`
				Limit      int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.DocumentID) == "" {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide a valid "documentId" string`)}
			}
			limit := a.Limit
			if limit <= 0 {
				limit = 5
			}
			if limit > 10 {
				limit = 10
			}
			offset := a.Offset
			if offset < 0 {
				offset = 0
			}

			res, err := deps.ReadDoc(ctx, a.DocumentID, offset, limit)
			if err != nil {
				return tool.Text(fmt.Sprintf("Failed to read document %q: %v", a.DocumentID, err)), nil
			}
			if res.TotalChunks == 0 {
				return tool.Text(fmt.Sprintf("Document %q has no processed text chunks.", res.DocTitle)), nil
			}
			if offset >= res.TotalChunks {
				return tool.Text(fmt.Sprintf("Offset %d is beyond the total chunk count (%d chunks) for document %q.", offset, res.TotalChunks, res.DocTitle)), nil
			}
			if len(res.Chunks) == 0 {
				return tool.Text(fmt.Sprintf("No chunks found in range [%d, %d] for document %q.", offset, offset+limit-1, res.DocTitle)), nil
			}

			var sb strings.Builder
			endIndex := offset + len(res.Chunks) - 1
			fmt.Fprintf(&sb, "Document: %s (ID: %s)\n", res.DocTitle, res.DocumentID)
			fmt.Fprintf(&sb, "Showing chunks %d to %d of %d total chunks:\n\n", offset, endIndex, res.TotalChunks)
			fmt.Fprintf(&sb, "Sources — cite these bracket numbers in your answer:\n\n")

			for i, ch := range res.Chunks {
				label := ch.DocTitle
				if ch.Heading != "" {
					label += " § " + ch.Heading
				}
				if ch.Page > 0 {
					label += fmt.Sprintf(", p.%d", ch.Page)
				}
				label += fmt.Sprintf(" (chunk %d/%d)", ch.Index, res.TotalChunks)

				text := ch.Content
				if len(text) > 2000 {
					text = text[:2000] + "…"
				}
				fmt.Fprintf(&sb, "[%d] (%s)\n%s\n\n", res.SourceOffset+i+1, label, text)
			}

			nextOffset := offset + len(res.Chunks)
			if nextOffset < res.TotalChunks {
				remaining := res.TotalChunks - nextOffset
				fmt.Fprintf(&sb, "--- %d more chunks remaining in this document (chunks %d to %d). Call read_document with offset=%d to continue reading. ---",
					remaining, nextOffset, res.TotalChunks-1, nextOffset)
			} else {
				fmt.Fprintf(&sb, "--- Reached the end of document %q (total chunks: %d). ---", res.DocTitle, res.TotalChunks)
			}

			return tool.Text(strings.TrimSpace(sb.String())), nil
		},
	}
}
