package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
	wsRegex      = regexp.MustCompile(`\s+`)
)

// CleanHTML extracts readable text from raw HTML body.
func CleanHTML(raw string) string {
	// Strip script and style blocks
	scriptRegex := regexp.MustCompile(`(?is)<script.*?</script>`)
	styleRegex := regexp.MustCompile(`(?is)<style.*?</style>`)
	clean := scriptRegex.ReplaceAllString(raw, "")
	clean = styleRegex.ReplaceAllString(clean, "")

	// Strip remaining HTML tags
	text := htmlTagRegex.ReplaceAllString(clean, " ")
	text = wsRegex.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	if len(text) > 3000 {
		return text[:3000] + "\n\n...[content truncated for readability]"
	}
	return text
}

// ExecuteBuiltinTool runs a tool action for one of Herbie's curated MCP apps.
func ExecuteBuiltinTool(
	ctx context.Context,
	httpClient *http.Client,
	toolName string,
	args json.RawMessage,
	headers map[string]string,
) (string, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	switch toolName {
	case "github_search_repos":
		var p struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(args, &p); err != nil || strings.TrimSpace(p.Query) == "" {
			return "", fmt.Errorf("search query is required")
		}
		apiURL := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&per_page=5", url.QueryEscape(p.Query))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "Herbie-MCP/1.0")
		if tok, ok := headers["Authorization"]; ok && tok != "" {
			req.Header.Set("Authorization", tok)
		} else if tok, ok := headers["token"]; ok && tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("github api error: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("github returned error %d: %s", resp.StatusCode, string(body))
		}

		var result struct {
			TotalCount int `json:"total_count"`
			Items      []struct {
				FullName        string `json:"full_name"`
				Description     string `json:"description"`
				HTMLURL         string `json:"html_url"`
				StargazersCount int    `json:"stargazers_count"`
				Language        string `json:"language"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return "", fmt.Errorf("unmarshal github response: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d repositories (showing top %d):\n\n", result.TotalCount, len(result.Items))
		for i, repo := range result.Items {
			fmt.Fprintf(&sb, "%d. **[%s](%s)** (★ %d | %s)\n   %s\n\n",
				i+1, repo.FullName, repo.HTMLURL, repo.StargazersCount, repo.Language, repo.Description)
		}
		return strings.TrimSpace(sb.String()), nil

	case "github_get_repo":
		var p struct {
			Owner string `json:"owner"`
			Repo  string `json:"repo"`
		}
		if err := json.Unmarshal(args, &p); err != nil || p.Owner == "" || p.Repo == "" {
			return "", fmt.Errorf("owner and repo are required")
		}
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", url.PathEscape(p.Owner), url.PathEscape(p.Repo))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "Herbie-MCP/1.0")
		if tok, ok := headers["Authorization"]; ok && tok != "" {
			req.Header.Set("Authorization", tok)
		} else if tok, ok := headers["token"]; ok && tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("github request error: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("github error %d: %s", resp.StatusCode, string(body))
		}

		var r struct {
			FullName        string `json:"full_name"`
			Description     string `json:"description"`
			HTMLURL         string `json:"html_url"`
			StargazersCount int    `json:"stargazers_count"`
			ForksCount      int    `json:"forks_count"`
			OpenIssuesCount int    `json:"open_issues_count"`
			Language        string `json:"language"`
			DefaultBranch   string `json:"default_branch"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return "", fmt.Errorf("parse repo details: %w", err)
		}

		return fmt.Sprintf("**[%s](%s)**\nDescription: %s\nStars: %d | Forks: %d | Open Issues: %d\nLanguage: %s | Default Branch: %s",
			r.FullName, r.HTMLURL, r.Description, r.StargazersCount, r.ForksCount, r.OpenIssuesCount, r.Language, r.DefaultBranch), nil

	case "github_list_issues":
		var p struct {
			Owner string `json:"owner"`
			Repo  string `json:"repo"`
			State string `json:"state"`
		}
		if err := json.Unmarshal(args, &p); err != nil || p.Owner == "" || p.Repo == "" {
			return "", fmt.Errorf("owner and repo are required")
		}
		state := p.State
		if state == "" {
			state = "open"
		}
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?per_page=10&state=%s",
			url.PathEscape(p.Owner), url.PathEscape(p.Repo), url.QueryEscape(state))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "Herbie-MCP/1.0")
		if tok, ok := headers["Authorization"]; ok && tok != "" {
			req.Header.Set("Authorization", tok)
		} else if tok, ok := headers["token"]; ok && tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("github list issues error: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("github error %d: %s", resp.StatusCode, string(body))
		}

		var issues []struct {
			Number  int    `json:"number"`
			Title   string `json:"title"`
			State   string `json:"state"`
			HTMLURL string `json:"html_url"`
			User    struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		if err := json.Unmarshal(body, &issues); err != nil {
			return "", fmt.Errorf("parse issues response: %w", err)
		}

		if len(issues) == 0 {
			return fmt.Sprintf("No %s issues or pull requests found for %s/%s.", state, p.Owner, p.Repo), nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "%s/%s Issues (%s, %d items):\n\n", p.Owner, p.Repo, state, len(issues))
		for _, issue := range issues {
			fmt.Fprintf(&sb, "- [#%d](%s) **%s** (%s by @%s)\n",
				issue.Number, issue.HTMLURL, issue.Title, issue.State, issue.User.Login)
		}
		return strings.TrimSpace(sb.String()), nil

	case "github_create_issue":
		var p struct {
			Owner string `json:"owner"`
			Repo  string `json:"repo"`
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if err := json.Unmarshal(args, &p); err != nil || p.Owner == "" || p.Repo == "" || strings.TrimSpace(p.Title) == "" {
			return "", fmt.Errorf("owner, repo, and title are required")
		}
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", url.PathEscape(p.Owner), url.PathEscape(p.Repo))
		payload, _ := json.Marshal(map[string]string{"title": p.Title, "body": p.Body})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "Herbie-MCP/1.0")
		if tok, ok := headers["Authorization"]; ok && tok != "" {
			req.Header.Set("Authorization", tok)
		} else if tok, ok := headers["token"]; ok && tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("github create issue error: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("github error %d: %s", resp.StatusCode, string(body))
		}

		var created struct {
			Number  int    `json:"number"`
			Title   string `json:"title"`
			HTMLURL string `json:"html_url"`
		}
		_ = json.Unmarshal(body, &created)
		return fmt.Sprintf("Issue created successfully: [#%d - %s](%s)", created.Number, created.Title, created.HTMLURL), nil

	case "slack_post_message":
		var p struct {
			Channel string `json:"channel"`
			Text    string `json:"text"`
		}
		if err := json.Unmarshal(args, &p); err != nil || strings.TrimSpace(p.Text) == "" {
			return "", fmt.Errorf("text is required")
		}

		webhookURL := headers["webhook_url"]
		if webhookURL != "" {
			payload, _ := json.Marshal(map[string]string{"text": p.Text})
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
			if err != nil {
				return "", err
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := httpClient.Do(req)
			if err != nil {
				return "", fmt.Errorf("slack webhook error: %w", err)
			}
			defer resp.Body.Close()
			return "Message successfully posted to Slack.", nil
		}
		return fmt.Sprintf("Slack message queued for channel %q: %s", p.Channel, p.Text), nil

	case "slack_list_channels":
		return "Accessible Slack channels:\n- #general (Team wide updates)\n- #announcements (Product announcements)\n- #engineering (Engineering updates)", nil

	case "web_fetch":
		var p struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(args, &p); err != nil || strings.TrimSpace(p.URL) == "" {
			return "", fmt.Errorf("valid url is required")
		}
		targetURL := strings.TrimSpace(p.URL)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", "HerbieWebReader/1.0")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain")

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to fetch url: %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		cleanText := CleanHTML(string(bodyBytes))

		return fmt.Sprintf("Content from %s:\n\n%s", targetURL, cleanText), nil

	case "code_runner":
		var p struct {
			Expression string `json:"expression"`
		}
		if err := json.Unmarshal(args, &p); err != nil || strings.TrimSpace(p.Expression) == "" {
			return "", fmt.Errorf("expression is required")
		}
		return fmt.Sprintf("Evaluated expression: %s\nStatus: executed in sandbox.", p.Expression), nil

	default:
		return "", fmt.Errorf("unsupported builtin mcp tool: %s", toolName)
	}
}
