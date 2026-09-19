package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem/tool"
)

// ListDocumentsToolName is the built-in tool for listing uploaded documents.
const ListDocumentsToolName = "list_documents"

// DocumentInfo carries metadata of one uploaded document for the agent.
type DocumentInfo struct {
	ID         string `json:"id"`
	Filename   string `json:"filename"`
	Mime       string `json:"mime"`
	SizeBytes  int64  `json:"sizeBytes"`
	Status     string `json:"status"`
	ChunkCount int    `json:"chunkCount"`
}

// ListDocsFunc returns available documents for the active context.
type ListDocsFunc func(ctx context.Context) ([]DocumentInfo, error)

var listDocumentsSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "limit": {"type": "integer", "description": "Maximum number of documents to return (default 50)."}
  }
}`)

// ListDocumentsTool provides the agent with visibility into uploaded files,
// their document IDs, chunk counts, sizes, and ingestion statuses.
func ListDocumentsTool() tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        ListDocumentsToolName,
		Description: "List the user's uploaded documents available for retrieval, including their filename, document ID, chunk count, file size, and status. Call this tool first to discover available documents and IDs. To summarize a document, use read_document(documentId=..., offset=0). For targeted queries, use search_documents.",
		Schema:      listDocumentsSchema,
		Timeout:     15 * time.Second,
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			if deps.ListDocs == nil {
				return tool.Text("No documents are available."), nil
			}
			docs, err := deps.ListDocs(ctx)
			if err != nil {
				return tool.Result{}, fmt.Errorf("list documents: %w", err)
			}
			if len(docs) == 0 {
				return tool.Text("No uploaded documents found."), nil
			}
			var a struct {
				Limit int `json:"limit"`
			}
			_ = json.Unmarshal(args, &a)
			limit := a.Limit
			if limit <= 0 || limit > len(docs) {
				limit = len(docs)
			}
			if limit > 50 {
				limit = 50
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "Uploaded documents (%d total):\n", len(docs))
			for i := 0; i < limit; i++ {
				d := docs[i]
				sizeStr := formatFileSize(d.SizeBytes)
				fmt.Fprintf(&sb, "- [id: %s] %s\n  Chunks: %d | Size: %s | MIME: %s | Status: %s\n",
					d.ID, d.Filename, d.ChunkCount, sizeStr, d.Mime, d.Status)
			}
			if limit < len(docs) {
				fmt.Fprintf(&sb, "...and %d more documents.\n", len(docs)-limit)
			}
			fmt.Fprintf(&sb, "\nUsage Guide:\n")
			fmt.Fprintf(&sb, "• To summarize a document: call read_document(documentId=\"<id>\", offset=0, limit=5) to read its title, abstract, and introduction.\n")
			fmt.Fprintf(&sb, "• To read subsequent sections: call read_document with the next offset.\n")
			fmt.Fprintf(&sb, "• For specific keywords or multi-hop facts: call search_documents(query=\"...\", documentIds=[\"<id>\"]).\n")
			fmt.Fprintf(&sb, "• Never use web_search for uploaded documents.\n")
			return tool.Text(strings.TrimSpace(sb.String())), nil
		},
	}
}

func formatFileSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
