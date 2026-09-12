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
	for _, key := range []string{"totals", "daily"} {
		raw, ok := got[key]
		if !ok || string(raw) != "[]" {
			t.Fatalf("key %q must be an empty JSON array, got %q (present=%v)", key, raw, ok)
		}
	}
	if len(got) != 2 {
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
