package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/websearch"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// WebSearchToolName is the tool identifier for live web search.
const WebSearchToolName = "web_search"

var webSearchSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "The search query to look up on the web."},
    "queries": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional list of up to 4 search queries to execute concurrently in parallel. Use this to research multiple sub-topics simultaneously in a single turn for maximum speed."
    }
  },
  "required": ["query"]
}`)

// WebSearchTool creates the built-in web search tool. When requireApproval is true
// and the call is not approved yet, it defers for user confirmation. Supports concurrent
// multi-query execution via the "queries" parameter.
func WebSearchTool(searcher websearch.Searcher, requireApproval bool) tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        WebSearchToolName,
		Description: "Search the public web for current events, live information, recent developments, or facts not present in your training data. Supports parallel multi-query searching via 'queries' (up to 4 topics concurrently).",
		Schema:      webSearchSchema,
		Timeout:     30 * time.Second,
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			var a struct {
				Query   string   `json:"query"`
				Queries []string `json:"queries"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide a valid query`)}
			}

			clean := func(s string) string {
				s = strings.TrimSpace(s)
				s = strings.TrimPrefix(s, "@web")
				s = strings.TrimPrefix(s, ":")
				return strings.TrimSpace(s)
			}

			var allQueries []string
			seen := map[string]bool{}
			for _, q := range a.Queries {
				cq := clean(q)
				if cq != "" && !seen[cq] {
					seen[cq] = true
					allQueries = append(allQueries, cq)
					if len(allQueries) >= 4 {
						break
					}
				}
			}
			if primary := clean(a.Query); primary != "" && !seen[primary] {
				if len(allQueries) == 0 {
					allQueries = append(allQueries, primary)
				} else if len(allQueries) < 4 {
					allQueries = append([]string{primary}, allQueries...)
				}
			}

			if len(allQueries) == 0 {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide a non-empty "query" string`)}
			}

			if requireApproval && !tool.CallApproved(ctx) {
				reason := fmt.Sprintf("Web search for %q", allQueries[0])
				if len(allQueries) > 1 {
					reason = fmt.Sprintf("Parallel web search for: %s", strings.Join(allQueries, ", "))
				}
				return tool.Result{}, &tool.Deferred{
					Kind:   tool.DeferApproval,
					Reason: reason,
				}
			}
			if searcher == nil {
				return tool.Text("Web search is currently unavailable."), nil
			}

			if len(allQueries) == 1 {
				query := allQueries[0]
				start := time.Now()
				results, err := searcher.Search(ctx, query)
				dur := time.Since(start).Milliseconds()
				if err != nil {
					return tool.Result{}, fmt.Errorf("web search: %w", err)
				}
				if deps.RecordSearch != nil {
					deps.RecordSearch(ctx, query, WebSearchToolName, "", len(results), dur)
				}
				if len(results) == 0 {
					return tool.Text(fmt.Sprintf("No web results found for %q.", query)), nil
				}
				if deps.AddWebSources != nil {
					deps.AddWebSources(results)
				}
				var sb strings.Builder
				for i, r := range results {
					fmt.Fprintf(&sb, "[%d] %s\nURL: %s\n%s\n\n", i+1, r.Title, r.URL, r.Snippet)
				}
				return tool.Text("Web search results:\n\n" + strings.TrimSpace(sb.String())), nil
			}

			// Parallel multi-query execution
			type queryRes struct {
				query      string
				results    []websearch.Result
				durationMs int64
				err        error
			}
			itemResults := make([]queryRes, len(allQueries))
			var wg sync.WaitGroup
			for idx, q := range allQueries {
				wg.Add(1)
				go func(i int, queryStr string) {
					defer wg.Done()
					qStart := time.Now()
					res, err := searcher.Search(ctx, queryStr)
					qDur := time.Since(qStart).Milliseconds()
					itemResults[i] = queryRes{
						query:      queryStr,
						results:    res,
						durationMs: qDur,
						err:        err,
					}
				}(idx, q)
			}
			wg.Wait()

			if deps.RecordSearch != nil {
				for _, ir := range itemResults {
					deps.RecordSearch(ctx, ir.query, WebSearchToolName, "", len(ir.results), ir.durationMs)
				}
			}

			var sb strings.Builder
			var allResults []websearch.Result
			sourceNum := 1
			hasAny := false
			for _, ir := range itemResults {
				if ir.err != nil || len(ir.results) == 0 {
					continue
				}
				hasAny = true
				fmt.Fprintf(&sb, "### Topic: %q\n", ir.query)
				for _, r := range ir.results {
					fmt.Fprintf(&sb, "[%d] %s\nURL: %s\n%s\n\n", sourceNum, r.Title, r.URL, r.Snippet)
					sourceNum++
					allResults = append(allResults, r)
				}
			}
			if !hasAny {
				return tool.Text(fmt.Sprintf("No web results found for %s.", strings.Join(allQueries, ", "))), nil
			}
			if deps.AddWebSources != nil && len(allResults) > 0 {
				deps.AddWebSources(allResults)
			}
			return tool.Text("Web search results:\n\n" + strings.TrimSpace(sb.String())), nil
		},
	}
}
