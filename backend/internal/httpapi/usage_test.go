package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type fakeSummaryStore struct {
	summary storage.Summary
	gotDays int
}

func (f *fakeSummaryStore) Add(_ context.Context, _ storage.UsageEvent) error { return nil }

func (f *fakeSummaryStore) RecordSearch(_ context.Context, _ storage.SearchQuery) error { return nil }

func (f *fakeSummaryStore) Summary(_ context.Context, _ string, days int) (storage.Summary, error) {
	f.gotDays = days
	return f.summary, nil
}

func TestUsageSummaryShapeAndDays(t *testing.T) {
	store := &fakeSummaryStore{summary: storage.Summary{
		Totals: []storage.KindModelRow{
			{Kind: "chat", Model: "gemini-2.5-flash", InputTokens: 120, OutputTokens: 70, Requests: 2, CostMicros: 211},
		},
		Daily: []storage.DailyRow{
			{Day: time.Date(2026, 8, 30, 15, 4, 0, 0, time.UTC), InputTokens: 120, OutputTokens: 70, CostMicros: 211},
		},
	}}
	h, token := newHandlerServer(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), store)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/usage/summary?days=7", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	if store.gotDays != 7 {
		t.Fatalf("days not forwarded: %d", store.gotDays)
	}
	var got struct {
		Totals []struct {
			Kind         string  `json:"kind"`
			Model        string  `json:"model"`
			InputTokens  int     `json:"inputTokens"`
			OutputTokens int     `json:"outputTokens"`
			Requests     int     `json:"requests"`
			CostUsd      float64 `json:"costUsd"`
		} `json:"totals"`
		Daily []struct {
			Day          string  `json:"day"`
			InputTokens  int     `json:"inputTokens"`
			OutputTokens int     `json:"outputTokens"`
			CostUsd      float64 `json:"costUsd"`
		} `json:"daily"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if len(got.Totals) != 1 || len(got.Daily) != 1 {
		t.Fatalf("row counts: %+v", got)
	}
	tot := got.Totals[0]
	if tot.Kind != "chat" || tot.Model != "gemini-2.5-flash" || tot.InputTokens != 120 ||
		tot.OutputTokens != 70 || tot.Requests != 2 {
		t.Fatalf("totals row: %+v", tot)
	}
	// 211 micros -> round(21.1)=21 -> 21/1e5 = 0.00021 USD.
	if tot.CostUsd != 0.00021 {
		t.Fatalf("totals costUsd: %v", tot.CostUsd)
	}
	d := got.Daily[0]
	if d.Day != "2026-08-30" || d.InputTokens != 120 || d.OutputTokens != 70 || d.CostUsd != 0.00021 {
		t.Fatalf("daily row: %+v", d)
	}
}

func TestUsageSummaryDaysDefaultsAndRejectsOutOfRange(t *testing.T) {
	store := &fakeSummaryStore{}
	h, token := newHandlerServer(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), store)

	for _, tc := range []struct {
		query, why string
		want       int
	}{
		{"", "no param defaults to 30", 30},
		{"?days=7", "valid value forwarded", 7},
		{"?days=365", "upper bound accepted", 365},
		{"?days=366", "above range falls back to default", 30},
		{"?days=0", "zero falls back to default", 30},
		{"?days=-3", "negative falls back to default", 30},
		{"?days=abc", "non-numeric falls back to default", 30},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/usage/summary"+tc.query, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", tc.why, rec.Code)
		}
		if store.gotDays != tc.want {
			t.Fatalf("%s: got days %d, want %d", tc.why, store.gotDays, tc.want)
		}
	}
}

func TestUsageSummaryEmptyIsJSONNotNull(t *testing.T) {
	store := &fakeSummaryStore{}
	h, token := newHandlerServer(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), store)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/usage/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(rec, req)

	// Object key order is not meaningful; assert both keys hold empty arrays
	// (never null) so clients can iterate without nil checks.
	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	for _, key := range []string{"totals", "daily", "documents"} {
		raw, ok := got[key]
		if !ok || string(raw) != "[]" {
			t.Fatalf("key %q must be an empty JSON array, got %q (present=%v)", key, raw, ok)
		}
	}
	if _, ok := got["searches"]; !ok {
		t.Fatalf("key 'searches' must be present in usage summary")
	}
	if _, ok := got["tools"]; !ok {
		t.Fatalf("key 'tools' must be present in usage summary")
	}
	if len(got) != 5 {
		t.Fatalf("unexpected keys: %v", got)
	}
}

func TestUsageSummaryIncludesDocumentSpend(t *testing.T) {
	store := &fakeSummaryStore{summary: storage.Summary{
		Documents: []storage.DocumentSpend{
			{DocumentID: "d1", Filename: "notes.md", InputTokens: 700, CostMicros: 105},
		},
	}}
	h, token := newHandlerServer(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), store)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/usage/summary?days=7", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Documents []struct {
			DocumentID  string  `json:"documentId"`
			Filename    string  `json:"filename"`
			InputTokens int     `json:"inputTokens"`
			CostUsd     float64 `json:"costUsd"`
		} `json:"documents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Documents) != 1 || body.Documents[0].Filename != "notes.md" || body.Documents[0].InputTokens != 700 {
		t.Fatalf("documents: %+v", body.Documents)
	}
	if body.Documents[0].CostUsd != microsToUSD(105) {
		t.Fatalf("costUsd: %v", body.Documents[0].CostUsd)
	}
}

func TestUsageSummaryIncludesSearchAnalysis(t *testing.T) {
	now := time.Now()
	store := &fakeSummaryStore{summary: storage.Summary{
		Searches: storage.SearchAnalysis{
			TotalQueries: 3,
			WebQueries:   2,
			DocQueries:   1,
			ByProvider: []storage.SearchProviderCount{
				{Provider: "tavily", Count: 2},
				{Provider: "rag", Count: 1},
			},
			Daily: []storage.SearchDailyCount{
				{Day: "2026-09-19", Count: 3},
			},
			Recent: []storage.SearchQueryItem{
				{
					ID:           "sq-1",
					Query:        "latest news",
					Kind:         "web_search",
					Provider:     "tavily",
					ResultsCount: 5,
					DurationMs:   120,
					CreatedAt:    now,
				},
			},
			TopQueries: []storage.SearchQueryCount{
				{Query: "latest news", Count: 2},
			},
		},
	}}
	h, token := newHandlerServer(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), store)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/usage/summary?days=7", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Searches struct {
			TotalQueries int `json:"totalQueries"`
			WebQueries   int `json:"webQueries"`
			DocQueries   int `json:"docQueries"`
			ByProvider   []struct {
				Provider string `json:"provider"`
				Count    int    `json:"count"`
			} `json:"byProvider"`
			Daily []struct {
				Day   string `json:"day"`
				Count int    `json:"count"`
			} `json:"daily"`
			Recent []struct {
				ID           string `json:"id"`
				Query        string `json:"query"`
				Kind         string `json:"kind"`
				Provider     string `json:"provider"`
				ResultsCount int    `json:"resultsCount"`
			} `json:"recent"`
			TopQueries []struct {
				Query string `json:"query"`
				Count int    `json:"count"`
			} `json:"topQueries"`
		} `json:"searches"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Searches.TotalQueries != 3 || body.Searches.WebQueries != 2 || body.Searches.DocQueries != 1 {
		t.Fatalf("unexpected query counts: %+v", body.Searches)
	}
	if len(body.Searches.ByProvider) != 2 || body.Searches.ByProvider[0].Provider != "tavily" {
		t.Fatalf("unexpected byProvider: %+v", body.Searches.ByProvider)
	}
	if len(body.Searches.Recent) != 1 || body.Searches.Recent[0].Query != "latest news" {
		t.Fatalf("unexpected recent: %+v", body.Searches.Recent)
	}
}

func TestUsageSummaryIncludesToolsAnalysis(t *testing.T) {
	store := &fakeSummaryStore{summary: storage.Summary{
		Tools: storage.ToolAnalysis{
			TotalExecutions:   5,
			SandboxExecutions: 3,
			SuccessCount:      4,
			FailedCount:       1,
			AvgDurationMs:     125,
			ByTool: []storage.ToolCount{
				{ToolName: "code_runner", Count: 3},
				{ToolName: "google_calendar", Count: 2},
			},
		},
	}}
	h, token := newHandlerServer(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), store)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/usage/summary?days=7", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Tools struct {
			TotalExecutions   int `json:"totalExecutions"`
			SandboxExecutions int `json:"sandboxExecutions"`
			SuccessCount      int `json:"successCount"`
			FailedCount       int `json:"failedCount"`
			AvgDurationMs     int `json:"avgDurationMs"`
			ByTool            []struct {
				ToolName string `json:"toolName"`
				Count    int    `json:"count"`
			} `json:"byTool"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Tools.TotalExecutions != 5 || body.Tools.SandboxExecutions != 3 || body.Tools.SuccessCount != 4 {
		t.Fatalf("unexpected tools counts: %+v", body.Tools)
	}
	if len(body.Tools.ByTool) != 2 || body.Tools.ByTool[0].ToolName != "code_sandbox" {
		t.Fatalf("expected code_runner mapped to code_sandbox, got: %+v", body.Tools.ByTool)
	}
}
