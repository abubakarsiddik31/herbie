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
// HTTP layer (embedding + Weaviate + usage metering behind it). An empty
// docIDs means all user documents; otherwise search is restricted to those
// document IDs (from an earlier result). It returns retrieved chunks, the
// 0-based offset in the run's cumulative sources list, and any error.
type SearchFunc func(ctx context.Context, query string, k int, docIDs []string) ([]rag.Scored, int, error)

const citationRules = `

When an answer relies on searched documents, cite sources inline with
their bracket numbers, e.g. [1]. The numbers refer to the
search_documents results.`

var searchSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "The text to search the user's documents for."},
    "k": {"type": "integer", "description": "How many passages to return (default 5, max 20)."},
    "documentIds": {"type": "array", "items": {"type": "string"}, "description": "Restrict search to these document IDs (from an earlier result). Omit for all documents."}
  },
  "required": ["query"]
}`)

// SearchTool is the built-in retrieval tool. deps.Search nil means the
// RAG stack is disabled: the tool answers text so the model can adapt
// instead of failing the run.
func SearchTool() tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        SearchToolName,
		Description: "Search the user's uploaded documents for passages relevant to a query. Results are hybrid-retrieved, expanded with surrounding context, and relevance-ranked. Call again with a refined query or narrower documentIds if needed (up to 2-3 queries) — synthesize once evidence is found.",
		Schema:      searchSchema,
		Timeout:     30 * time.Second,
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			if deps.Search == nil {
				return tool.Text("No documents are available."), nil
			}
			var a struct {
				Query       string   `json:"query"`
				K           int      `json:"k"`
				DocumentIDs []string `json:"documentIds"`
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
			var docIDs []string
			for _, id := range a.DocumentIDs {
				if strings.TrimSpace(id) == "" {
					continue
				}
				docIDs = append(docIDs, id)
				if len(docIDs) >= 20 {
					break
				}
			}
			scored, offset, err := deps.Search(ctx, a.Query, k, docIDs)
			if err != nil {
				return tool.Result{}, fmt.Errorf("search documents: %w", err)
			}
			if len(scored) == 0 {
				return tool.Text("No matching documents."), nil
			}
			var sb strings.Builder
			for i, s := range scored {
				label := s.Chunk.DocTitle
				if s.Chunk.Heading != "" {
					label += " § " + s.Chunk.Heading
				}
				if s.Chunk.Page > 0 {
					label += fmt.Sprintf(", p.%d", s.Chunk.Page)
				}
				text := rag.DisplayText(s)
				if len(text) > 2000 {
					text = text[:2000] + "…"
				}
				fmt.Fprintf(&sb, "[%d] (%s) %s\n\n", offset+i+1, label, text)
			}
			return tool.Text("Sources — cite ONLY these bracket numbers:\n\n" + strings.TrimSpace(sb.String())), nil
		},
	}
}
