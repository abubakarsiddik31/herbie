package websearch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
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
	Provider string // "tavily", "brave", or "free" (default)
	APIKey   string
	BaseURL  string // optional override for testing / proxy
	Timeout  time.Duration
}

func (c Config) Enabled() bool {
	if c.Provider == "none" || c.Provider == "disabled" {
		return false
	}
	return true
}

func New(cfg Config) Searcher {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "brave" && cfg.APIKey != "" {
		base := cfg.BaseURL
		if base == "" {
			base = "https://api.search.brave.com/res/v1/web/search"
		}
		return &braveSearcher{apiKey: cfg.APIKey, baseURL: base, client: client}
	}
	if provider == "tavily" && cfg.APIKey != "" {
		base := cfg.BaseURL
		if base == "" {
			base = "https://api.tavily.com/search"
		}
		return &tavilySearcher{apiKey: cfg.APIKey, baseURL: base, client: client}
	}
	base := cfg.BaseURL
	if base == "" {
		base = "https://www.bing.com/search"
	}
	return &freeSearcher{baseURL: base, client: client}
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

type freeSearcher struct {
	baseURL string
	client  *http.Client
}

func decodeBingTrackerURL(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	if !strings.HasSuffix(u.Hostname(), "bing.com") || u.Path != "/ck/a" {
		return href
	}
	encoded := u.Query().Get("u")
	if len(encoded) < 4 {
		return href
	}
	trimmed := encoded[2:]
	if pad := len(trimmed) % 4; pad > 0 {
		trimmed += strings.Repeat("=", 4-pad)
	}
	decoded, err := base64.URLEncoding.DecodeString(trimmed)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(trimmed)
		if err != nil {
			return href
		}
	}
	dest := string(decoded)
	if _, err := url.ParseRequestURI(dest); err == nil {
		return dest
	}
	return href
}

func (f *freeSearcher) Search(ctx context.Context, query string) ([]Result, error) {
	reqURL := fmt.Sprintf("%s?q=%s&mkt=en-US&setlang=en", f.baseURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	res, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("free web search: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return f.fallbackDuckDuckGo(ctx, query)
	}

	results, err := parseBingHTML(io.LimitReader(res.Body, 2<<20), 5)
	if err != nil || len(results) == 0 {
		return f.fallbackDuckDuckGo(ctx, query)
	}
	return results, nil
}

func parseBingHTML(r io.Reader, maxResults int) ([]Result, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var results []Result
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= maxResults {
			return
		}
		if n.Type == html.ElementNode && n.Data == "li" {
			for _, a := range n.Attr {
				if a.Key == "class" && strings.Contains(a.Val, "b_algo") {
					if res := extractBingAlgo(n); res != nil {
						results = append(results, *res)
					}
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return results, nil
}

func extractBingAlgo(n *html.Node) *Result {
	var linkURL, linkTitle, snippet string
	var walkNode func(*html.Node)
	walkNode = func(curr *html.Node) {
		if curr.Type == html.ElementNode && curr.Data == "h2" {
			for c := curr.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "a" {
					for _, attr := range c.Attr {
						if attr.Key == "href" {
							linkURL = decodeBingTrackerURL(attr.Val)
						}
					}
					linkTitle = getText(c)
				}
			}
		}
		if curr.Type == html.ElementNode && curr.Data == "p" && snippet == "" {
			snippet = getText(curr)
		}
		for c := curr.FirstChild; c != nil; c = c.NextSibling {
			walkNode(c)
		}
	}
	walkNode(n)
	if linkTitle == "" || linkURL == "" {
		return nil
	}
	if len(snippet) > 400 {
		snippet = strings.TrimSpace(snippet[:400]) + "…"
	}
	return &Result{
		Title:   linkTitle,
		URL:     linkURL,
		Snippet: snippet,
	}
}

func getText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(curr *html.Node) {
		if curr.Type == html.TextNode {
			sb.WriteString(curr.Data)
		}
		for c := curr.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(sb.String()), " ")
}

func (f *freeSearcher) fallbackDuckDuckGo(ctx context.Context, query string) ([]Result, error) {
	reqURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1", url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GolemChatbot/1.0")

	res, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var ddg struct {
		AbstractText  string `json:"AbstractText"`
		AbstractURL   string `json:"AbstractURL"`
		Heading       string `json:"Heading"`
		RelatedTopics []struct {
			FirstURL string `json:"FirstURL"`
			Text     string `json:"Text"`
		} `json:"RelatedTopics"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 512<<10)).Decode(&ddg); err != nil {
		return nil, nil
	}

	var results []Result
	if ddg.AbstractText != "" && ddg.AbstractURL != "" {
		title := ddg.Heading
		if title == "" {
			title = query
		}
		results = append(results, Result{
			Title:   title,
			URL:     ddg.AbstractURL,
			Snippet: ddg.AbstractText,
		})
	}
	for _, topic := range ddg.RelatedTopics {
		if len(results) >= 5 {
			break
		}
		if topic.FirstURL != "" && topic.Text != "" {
			results = append(results, Result{
				Title:   topic.Text,
				URL:     topic.FirstURL,
				Snippet: topic.Text,
			})
		}
	}
	return results, nil
}
