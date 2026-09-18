package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type Searcher interface {
	Search(ctx context.Context, query string) ([]Result, error)
}

type Config struct {
	Provider string // "tavily" (default) or "brave"
	APIKey   string
	BaseURL  string // optional override for testing / proxy
	Timeout  time.Duration
}

func (c Config) Enabled() bool {
	return c.APIKey != ""
}

func New(cfg Config) Searcher {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "brave" {
		base := cfg.BaseURL
		if base == "" {
			base = "https://api.search.brave.com/res/v1/web/search"
		}
		return &braveSearcher{apiKey: cfg.APIKey, baseURL: base, client: client}
	}
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.tavily.com/search"
	}
	return &tavilySearcher{apiKey: cfg.APIKey, baseURL: base, client: client}
}

type tavilySearcher struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func (t *tavilySearcher) Search(ctx context.Context, query string) ([]Result, error) {
	payload, err := json.Marshal(map[string]any{
		"api_key":      t.apiKey,
		"query":        query,
		"max_results":  5,
		"search_depth": "basic",
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily search: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return nil, fmt.Errorf("tavily search failed (status %d): %s", res.StatusCode, string(body))
	}

	var resp struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode tavily response: %w", err)
	}
	results := make([]Result, len(resp.Results))
	for i, r := range resp.Results {
		snippet := r.Content
		if len(snippet) > 400 {
			snippet = strings.TrimSpace(snippet[:400]) + "…"
		}
		results[i] = Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
		}
	}
	return results, nil
}

type braveSearcher struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func (b *braveSearcher) Search(ctx context.Context, query string) ([]Result, error) {
	reqURL := fmt.Sprintf("%s?q=%s&count=5", b.baseURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Subscription-Token", b.apiKey)
	req.Header.Set("Accept", "application/json")

	res, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return nil, fmt.Errorf("brave search failed (status %d): %s", res.StatusCode, string(body))
	}

	var resp struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode brave response: %w", err)
	}
	results := make([]Result, len(resp.Web.Results))
	for i, r := range resp.Web.Results {
		snippet := r.Description
		if len(snippet) > 400 {
			snippet = strings.TrimSpace(snippet[:400]) + "…"
		}
		results[i] = Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snippet,
		}
	}
	return results, nil
}
