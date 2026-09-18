package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTavilySearchSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req struct {
			APIKey string `json:"api_key"`
			Query  string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.APIKey != "test-key" || req.Query != "golang news" {
			t.Errorf("unexpected request: %+v", req)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]string{
				{"title": "Go 1.25 Released", "url": "https://golang.org/doc/1.25", "content": "Details on the release."},
			},
		})
	}))
	defer ts.Close()

	s := New(Config{
		Provider: "tavily",
		APIKey:   "test-key",
		BaseURL:  ts.URL,
	})
	results, err := s.Search(context.Background(), "golang news")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Go 1.25 Released" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestBraveSearchSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("X-Subscription-Token") != "brave-key" {
			t.Errorf("missing token header")
		}
		if r.URL.Query().Get("q") != "rust news" {
			t.Errorf("unexpected query: %s", r.URL.Query().Get("q"))
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{
				"results": []map[string]string{
					{"title": "Rust Updates", "url": "https://rust-lang.org", "description": "Rust blog."},
				},
			},
		})
	}))
	defer ts.Close()

	s := New(Config{
		Provider: "brave",
		APIKey:   "brave-key",
		BaseURL:  ts.URL,
	})
	results, err := s.Search(context.Background(), "rust news")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Rust Updates" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestSearchHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer ts.Close()

	s := New(Config{
		Provider: "tavily",
		APIKey:   "key",
		BaseURL:  ts.URL,
	})
	_, err := s.Search(context.Background(), "query")
	if err == nil {
		t.Fatal("expected error on 429")
	}
}
