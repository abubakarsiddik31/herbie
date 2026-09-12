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

// SearchToolName is the built-in retrieval tool; citation rules join the
// system prompt exactly when a tool with this name is registered.
const SearchToolName = "search_documents"

// SearchFunc retrieves the user's document chunks; wired per run by the
// HTTP layer (embedding + Weaviate + usage metering behind it).
type SearchFunc func(ctx context.Context, query string, k int) ([]rag.Scored, error)

const citationRules = `

When an answer relies on searched documents, cite sources inline with
their bracket numbers, e.g. [1]. The numbers refer to the
search_documents results.`

var searchSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "The text to search the user's documents for."},
    "k": {"type": "integer", "description": "How many passages to return (default 5, max 20)."}
  },
  "required": ["query"]
}`)

// SearchTool is the built-in retrieval tool. deps.Search nil means the
// RAG stack is disabled: the tool answers text so the model can adapt
// instead of failing the run.
func SearchTool() tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        SearchToolName,
		Description: "Search the user's uploaded documents for passages relevant to a query. Use it when the answer may depend on the user's documents.",
		Schema:      searchSchema,
		Timeout:     30 * time.Second,
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			if deps.Search == nil {
				return tool.Text("No documents are available."), nil
			}
			var a struct {
				Query string `json:"query"`
				K     int    `json:"k"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Query) == "" {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide a non-empty "query" string`)}
			}
			k := a.K
			if k <= 0 {
				k = 5
			}
			if k > 20 {
				k = 20
			}
			scored, err := deps.Search(ctx, a.Query, k)
			if err != nil {
				return tool.Result{}, fmt.Errorf("search documents: %w", err)
			}
			if len(scored) == 0 {
				return tool.Text("No matching documents."), nil
			}
			var sb strings.Builder
			for i, s := range scored {
				content := s.Chunk.Content
				if len(content) > 2000 {
					content = content[:2000] + "…"
				}
				fmt.Fprintf(&sb, "[%d] (%s) %s\n\n", i+1, s.Chunk.DocTitle, content)
			}
			return tool.Text(strings.TrimSpace(sb.String())), nil
		},
	}
}
