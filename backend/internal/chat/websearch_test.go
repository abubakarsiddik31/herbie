package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/websearch"
	"github.com/abubakarsiddik31/golem/tool"
)

type mockSearcher struct {
	results []websearch.Result
	err     error
	called  string
}

func (m *mockSearcher) Search(_ context.Context, query string) ([]websearch.Result, error) {
	m.called = query
	return m.results, m.err
}

func TestWebSearchToolSchema(t *testing.T) {
	tl := WebSearchTool(nil, false)
	if tl.Name != WebSearchToolName {
		t.Fatalf("name: %s", tl.Name)
	}
	var schema struct {
		Type       string   `json:"type"`
		Required   []string `json:"required"`
		Properties map[string]any
	}
	if err := json.Unmarshal(tl.Schema, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Type != "object" || len(schema.Required) != 1 || schema.Required[0] != "query" {
		t.Fatalf("schema: %+v", schema)
	}
}

func TestWebSearchToolApprovalDeferral(t *testing.T) {
	mock := &mockSearcher{results: []websearch.Result{{Title: "T", URL: "U", Snippet: "S"}}}
	tl := WebSearchTool(mock, true)

	// Unapproved call defers for approval
	_, err := tl.Exec(context.Background(), Deps{}, json.RawMessage(`{"query":"weather today"}`))
	var deferred *tool.Deferred
	if !errors.As(err, &deferred) {
		t.Fatalf("expected Deferred error, got %v", err)
	}
	if deferred.Kind != tool.DeferApproval {
		t.Fatalf("expected DeferApproval, got %v", deferred.Kind)
	}
	if mock.called != "" {
		t.Fatal("searcher should not have been called when unapproved")
	}

	// Approved call runs search
	ctx := tool.WithApprovedCall(context.Background())
	res, err := tl.Exec(ctx, Deps{}, json.RawMessage(`{"query":"weather today"}`))
	if err != nil {
		t.Fatalf("approved call failed: %v", err)
	}
	if mock.called != "weather today" {
		t.Fatalf("expected query 'weather today', got %q", mock.called)
	}
	if !strings.Contains(res.Text, "Web search results:") || !strings.Contains(res.Text, "URL: U") {
		t.Fatalf("unexpected content: %s", res.Text)
	}
}

func TestWebSearchToolNoResults(t *testing.T) {
	mock := &mockSearcher{results: nil}
	tl := WebSearchTool(mock, false)
	res, err := tl.Exec(context.Background(), Deps{}, json.RawMessage(`{"query":"nothing"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "No web results found") {
		t.Fatalf("unexpected response: %s", res.Text)
	}
}

func TestWebSearchToolEmptyQuery(t *testing.T) {
	mock := &mockSearcher{}
	tl := WebSearchTool(mock, false)
	_, err := tl.Exec(context.Background(), Deps{}, json.RawMessage(`{"query":"  "}`))
	if err == nil {
		t.Fatal("expected error on empty query")
	}
}

func TestWebSearchToolRecordsSearch(t *testing.T) {
	mock := &mockSearcher{results: []websearch.Result{{Title: "Test", URL: "http://example.com", Snippet: "Test snippet"}}}
	tl := WebSearchTool(mock, false)

	var recordedQuery, recordedKind string
	var recordedCount int
	deps := Deps{
		RecordSearch: func(_ context.Context, query, kind, _ string, resultsCount int, _ int64) {
			recordedQuery = query
			recordedKind = kind
			recordedCount = resultsCount
		},
	}
	_, err := tl.Exec(context.Background(), deps, json.RawMessage(`{"query":"golang 1.25"}`))
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if recordedQuery != "golang 1.25" || recordedKind != WebSearchToolName || recordedCount != 1 {
		t.Fatalf("unexpected search recorded: query=%q kind=%q count=%d", recordedQuery, recordedKind, recordedCount)
	}
}

func TestWebSearchToolParallelQueries(t *testing.T) {
	mock := &mockSearcher{results: []websearch.Result{{Title: "Test", URL: "http://example.com", Snippet: "Test snippet"}}}
	tl := WebSearchTool(mock, false)

	recorded := []string{}
	deps := Deps{
		RecordSearch: func(_ context.Context, query, kind, _ string, _ int, _ int64) {
			recorded = append(recorded, query)
		},
	}
	res, err := tl.Exec(context.Background(), deps, json.RawMessage(`{"queries":["topic one","topic two"]}`))
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if len(recorded) != 2 {
		t.Fatalf("expected 2 recorded queries, got %d", len(recorded))
	}
	if !strings.Contains(res.Text, "Topic: \"topic one\"") || !strings.Contains(res.Text, "Topic: \"topic two\"") {
		t.Fatalf("missing topic headers: %s", res.Text)
	}
}

func TestWebSearchGuidanceAppended(t *testing.T) {
	spec := RunSpec{SystemPrompt: "base"}
	got := promptFor(spec, []tool.Tool[Deps]{WebSearchTool(nil, false)})
	if !strings.Contains(got, "web_search tool to search the live web") {
		t.Fatalf("expected web search guidance: %s", got)
	}
}
