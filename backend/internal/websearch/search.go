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
	"os/exec"
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
	Provider string // "wigolo", "tavily", "brave", or "free" (default)
	APIKey   string
	BaseURL  string // optional override for testing / proxy
	BinPath  string // optional path to local wigolo CLI/dist
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
	if provider == "wigolo" {
		base := cfg.BaseURL
		if base == "" && cfg.BinPath == "" {
			base = "http://localhost:3333"
		}
		return &wigoloSearcher{
			baseURL: base,
			binPath: cfg.BinPath,
			apiKey:  cfg.APIKey,
			client:  client,
		}
	}
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

type wigoloSearcher struct {
	baseURL string
	binPath string
	apiKey  string
	client  *http.Client
}

func (w *wigoloSearcher) Search(ctx context.Context, query string) ([]Result, error) {
	if w.baseURL != "" {
		results, err := w.searchHTTP(ctx, query)
		if err == nil {
			return results, nil
		}
		if w.binPath == "" {
			return nil, err
		}
	}
	if w.binPath != "" {
		return w.searchCLI(ctx, query)
	}
	return nil, fmt.Errorf("wigolo search: neither baseURL nor binPath configured")
}

func (w *wigoloSearcher) searchHTTP(ctx context.Context, query string) ([]Result, error) {
	reqURL := strings.TrimRight(w.baseURL, "/")
	if !strings.HasSuffix(reqURL, "/v1/search") {
		reqURL += "/v1/search"
	}
	payload, err := json.Marshal(map[string]any{
		"query":                 query,
		"max_results":           5,
		"include_content":       false,
		"max_tokens_out":        4000,
		"content_max_chars":     4000,
		"max_total_chars":       20000,
		"include_full_markdown": false,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if w.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+w.apiKey)
	}

	res, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wigolo search: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return nil, fmt.Errorf("wigolo search failed (status %d): %s", res.StatusCode, string(body))
	}

	var resp struct {
		Results []struct {
			Title    string `json:"title"`
			URL      string `json:"url"`
			Snippet  string `json:"snippet"`
			Content  string `json:"content"`
			Markdown string `json:"markdown"`
		} `json:"results"`
		Evidence []struct {
			Title          string `json:"title"`
			URL            string `json:"url"`
			SectionHeading string `json:"section_heading"`
			Excerpt        string `json:"excerpt"`
		} `json:"evidence"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode wigolo response: %w", err)
	}

	results := make([]Result, 0, len(resp.Results))
	for _, r := range resp.Results {
		snip := strings.TrimSpace(r.Snippet)
		if snip == "" {
			if r.Markdown != "" {
				snip = strings.TrimSpace(r.Markdown)
			} else if r.Content != "" {
				snip = strings.TrimSpace(r.Content)
			}
		}
		if len(snip) > 2500 {
			snip = strings.TrimSpace(snip[:2500]) + "…"
		}
		results = append(results, Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snip,
		})
	}
	if len(results) == 0 && len(resp.Evidence) > 0 {
		for _, ev := range resp.Evidence {
			snip := ev.Excerpt
			if len(snip) > 80000 {
				snip = strings.TrimSpace(snip[:80000]) + "…"
			}
			results = append(results, Result{
				Title:   ev.Title,
				URL:     ev.URL,
				Snippet: snip,
			})
		}
	}
	return results, nil
}

func (w *wigoloSearcher) searchCLI(ctx context.Context, query string) ([]Result, error) {
	args := []string{"search", query, "--json", "--max-results=5"}
	var cmd *exec.Cmd
	if strings.HasSuffix(w.binPath, ".js") {
		cmd = exec.CommandContext(ctx, "node", append([]string{w.binPath}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, w.binPath, args...)
	}

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("wigolo cli error: %w", err)
	}

	var resp struct {
		Results []struct {
			Title    string `json:"title"`
			URL      string `json:"url"`
			Snippet  string `json:"snippet"`
			Content  string `json:"content"`
			Markdown string `json:"markdown"`
		} `json:"results"`
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var raw map[string]json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			break
		}
		if rawResults, ok := raw["results"]; ok {
			_ = json.Unmarshal(rawResults, &resp.Results)
			if len(resp.Results) > 0 {
				break
			}
		}
	}

	results := make([]Result, 0, len(resp.Results))
	for _, r := range resp.Results {
		snip := r.Snippet
		if r.Markdown != "" && len(r.Markdown) > len(snip) {
			snip = r.Markdown
		} else if r.Content != "" && len(r.Content) > len(snip) {
			snip = r.Content
		}
		if len(snip) > 1500 {
			snip = strings.TrimSpace(snip[:1500]) + "…"
		}
		results = append(results, Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: snip,
		})
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
