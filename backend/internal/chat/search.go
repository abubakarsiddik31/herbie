package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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
    "query": {"type": "string", "description": "Primary text query to search documents for."},
    "queries": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional list of up to 3 queries to search concurrently in parallel for independent facets (e.g. comparing two concepts). For dependent multi-hop queries, search sequentially instead."
    },
    "k": {"type": "integer", "description": "How many passages to return (default 5, max 20)."},
    "documentIds": {"type": "array", "items": {"type": "string"}, "description": "Restrict search to these document IDs (from an earlier result). Omit for all documents."}
  },
  "required": ["query"]
}`)

// SearchTool is the built-in retrieval tool. deps.Search nil means the
// RAG stack is disabled: the tool answers text so the model can adapt
// instead of failing the run. Supports parallel multi-query searching via 'queries'.
func SearchTool() tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        SearchToolName,
		Description: "Search the user's uploaded documents for passages relevant to a query. Results are hybrid-retrieved, expanded with surrounding context, and relevance-ranked. For independent facets, provide 'queries' for parallel retrieval; for dependent multi-hop queries, execute sequentially.",
		Schema:      searchSchema,
		Timeout:     30 * time.Second,
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			if deps.Search == nil {
				return tool.Text("No documents are available."), nil
			}
			var a struct {
				Query       string   `json:"query"`
				Queries     []string `json:"queries"`
				K           int      `json:"k"`
				DocumentIDs []string `json:"documentIds"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide a valid query`)}
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

			var allQueries []string
			seen := map[string]bool{}
			for _, q := range a.Queries {
				q = strings.TrimSpace(q)
				if q != "" && !seen[q] {
					seen[q] = true
					allQueries = append(allQueries, q)
					if len(allQueries) >= 3 {
						break
					}
				}
			}
			if primary := strings.TrimSpace(a.Query); primary != "" && !seen[primary] {
				if len(allQueries) == 0 {
					allQueries = append(allQueries, primary)
				} else if len(allQueries) < 3 {
					allQueries = append([]string{primary}, allQueries...)
				}
			}

			if len(allQueries) == 0 {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide a non-empty "query" string`)}
			}

			if len(allQueries) == 1 {
				scored, offset, err := deps.Search(ctx, allQueries[0], k, docIDs)
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
			}

			// Parallel multi-query execution for independent document facets
			type docQueryResult struct {
				query  string
				scored []rag.Scored
				offset int
				err    error
			}
			qResults := make([]docQueryResult, len(allQueries))
			var wg sync.WaitGroup
			for idx, q := range allQueries {
				wg.Add(1)
				go func(i int, queryStr string) {
					defer wg.Done()
					sc, off, err := deps.Search(ctx, queryStr, k, docIDs)
					qResults[i] = docQueryResult{
						query:  queryStr,
						scored: sc,
						offset: off,
						err:    err,
					}
				}(idx, q)
			}
			wg.Wait()

			var sb strings.Builder
			hasAny := false
			for _, qr := range qResults {
				if qr.err != nil || len(qr.scored) == 0 {
					continue
				}
				hasAny = true
				fmt.Fprintf(&sb, "### Document search results for %q:\n\n", qr.query)
				for i, s := range qr.scored {
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
					fmt.Fprintf(&sb, "[%d] (%s) %s\n\n", qr.offset+i+1, label, text)
				}
			}
			if !hasAny {
				return tool.Text("No matching documents found."), nil
			}
			return tool.Text("Sources — cite ONLY these bracket numbers:\n\n" + strings.TrimSpace(sb.String())), nil
		},
	}
}
