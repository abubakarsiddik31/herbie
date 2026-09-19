package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
    "query": {"type": "string", "description": "The search query to look up on the web for up-to-date facts, current events, or external information."}
  },
  "required": ["query"]
}`)

// WebSearchTool creates the built-in web search tool. When requireApproval is true
// and the call is not approved yet, it defers for user confirmation.
func WebSearchTool(searcher websearch.Searcher, requireApproval bool) tool.Tool[Deps] {
	return tool.Tool[Deps]{
		Name:        WebSearchToolName,
		Description: "Search the public web for current events, live information, recent developments, or facts not present in your training data (1-3 targeted queries recommended).",
		Schema:      webSearchSchema,
		Timeout:     20 * time.Second,
		MaxRetries:  tool.RetryLimit(1),
		Exec: func(ctx context.Context, deps Deps, args json.RawMessage) (tool.Result, error) {
			var a struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(args, &a); err != nil || strings.TrimSpace(a.Query) == "" {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide a non-empty "query" string`)}
			}
			query := strings.TrimSpace(a.Query)
			query = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(query, "@web"), ":"))
			if query == "" {
				return tool.Result{}, &model.ModelRetry{Err: fmt.Errorf(`provide a non-empty "query" string`)}
			}
			if requireApproval && !tool.CallApproved(ctx) {
				return tool.Result{}, &tool.Deferred{
					Kind:   tool.DeferApproval,
					Reason: fmt.Sprintf("Web search for %q", query),
				}
			}
			if searcher == nil {
				return tool.Text("Web search is currently unavailable."), nil
			}
			results, err := searcher.Search(ctx, query)
			if err != nil {
				return tool.Result{}, fmt.Errorf("web search: %w", err)
			}
			if len(results) == 0 {
				return tool.Text(fmt.Sprintf("No web results found for %q.", query)), nil
			}
			var sb strings.Builder
			for i, r := range results {
				fmt.Fprintf(&sb, "[%d] %s\nURL: %s\n%s\n\n", i+1, r.Title, r.URL, r.Snippet)
			}
			return tool.Text("Web search results:\n\n" + strings.TrimSpace(sb.String())), nil
		},
	}
}
