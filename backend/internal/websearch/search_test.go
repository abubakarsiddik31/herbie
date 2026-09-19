package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestFreeSearchSuccess(t *testing.T) {
	sampleHTML := `<!DOCTYPE html><html><body>
		<ol id="b_results">
			<li class="b_algo">
				<h2><a href="https://example.com/golem"><strong>Golem</strong> - Legends</a></h2>
				<p class="b_lineclamp2">A magical entity in folklore made of clay.</p>
			</li>
		</ol>
	</body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer ts.Close()

	s := New(Config{
		Provider: "free",
		BaseURL:  ts.URL,
	})
	results, err := s.Search(context.Background(), "golem")
	if err != nil {
		t.Fatalf("free search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Golem - Legends" {
		t.Errorf("title = %q, want %q", results[0].Title, "Golem - Legends")
	}
	if results[0].URL != "https://example.com/golem" {
		t.Errorf("url = %q, want %q", results[0].URL, "https://example.com/golem")
	}
	if !strings.Contains(results[0].Snippet, "magical entity") {
		t.Errorf("snippet = %q, want to contain 'magical entity'", results[0].Snippet)
	}
}

func TestDecodeBingTrackerURL(t *testing.T) {
	tracker := "https://www.bing.com/ck/a?!&&p=abc&u=a1aHR0cHM6Ly9lbi53aWtpcGVkaWEub3JnL3dpa2kvR29sZW0&ntb=1"
	want := "https://en.wikipedia.org/wiki/Golem"
	got := decodeBingTrackerURL(tracker)
	if got != want {
		t.Errorf("decodeBingTrackerURL = %q, want %q", got, want)
	}

	direct := "https://example.com/page"
	if decodeBingTrackerURL(direct) != direct {
		t.Errorf("plain url changed: %q", decodeBingTrackerURL(direct))
	}
}

func TestWigoloSearchSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/search" {
			t.Errorf("expected /v1/search, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		var req struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Query != "ai agents news" {
			t.Errorf("unexpected query: %s", req.Query)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]string{
				{
					"title":   "AI Agent Breakthroughs",
					"url":     "https://example.com/ai-agents",
					"snippet": "New developments in multi-agent orchestration.",
				},
			},
		})
	}))
	defer ts.Close()

	s := New(Config{
		Provider: "wigolo",
		BaseURL:  ts.URL,
		APIKey:   "secret-token",
	})
	results, err := s.Search(context.Background(), "ai agents news")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "AI Agent Breakthroughs" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[0].URL != "https://example.com/ai-agents" {
		t.Errorf("unexpected url: %s", results[0].URL)
	}
}

func TestWigoloSearchEvidenceFallback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []any{},
			"evidence": []map[string]string{
				{
					"title":   "Evidence Item",
					"url":     "https://example.com/evidence",
					"excerpt": "Evidence excerpt here.",
				},
			},
		})
	}))
	defer ts.Close()

	s := New(Config{
		Provider: "wigolo",
		BaseURL:  ts.URL,
	})
	results, err := s.Search(context.Background(), "fallback query")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Evidence Item" {
		t.Fatalf("unexpected fallback results: %+v", results)
	}
	if results[0].Snippet != "Evidence excerpt here." {
		t.Errorf("unexpected snippet: %s", results[0].Snippet)
	}
}

func TestWigoloLive(t *testing.T) {
	if os.Getenv("WIGOLO_LIVE") != "1" {
		t.Skip("skipping live wigolo test; set WIGOLO_LIVE=1 to run")
	}
	s := New(Config{
		Provider: "wigolo",
		BaseURL:  "http://localhost:3333",
	})
	results, err := s.Search(context.Background(), "AI agents news")
	if err != nil {
		t.Fatalf("live search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	t.Logf("Got %d live results, first title: %s", len(results), results[0].Title)
}
