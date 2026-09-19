package mcp

import "encoding/json"

// CatalogApp represents a curated, zero-configuration MCP application.
type CatalogApp struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Mention      string `json:"mention"`
	Category     string `json:"category"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	Tools        []Tool `json:"tools"`
	RequiresAuth bool   `json:"requiresAuth"`
	AuthType     string `json:"authType,omitempty"`
	AuthPrompt   string `json:"authPrompt,omitempty"`
	Connected    bool   `json:"connected"`
	ConnectedAt  string `json:"connectedAt,omitempty"`
	ServerID     string `json:"serverId,omitempty"`
}

// DefaultCatalog holds the curated list of 1-click MCP apps ready to link.
var DefaultCatalog = []CatalogApp{
	{
		ID:           "github",
		Name:         "GitHub",
		Mention:      "github",
		Category:     "Developer",
		Description:  "Inspect repositories, search code, list pull requests, and manage issues.",
		Icon:         "github",
		RequiresAuth: false,
		AuthType:     "token",
		AuthPrompt:   "Optional Personal Access Token (for private repos)",
		Tools: []Tool{
			{
				Name:        "github_search_repos",
				Description: "Search public and private repositories across GitHub.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query keywords"}},"required":["query"]}`),
			},
			{
				Name:        "github_get_repo",
				Description: "Get details and statistics for a GitHub repository.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"},"repo":{"type":"string"}},"required":["owner","repo"]}`),
			},
			{
				Name:        "github_list_issues",
				Description: "List issues and pull requests for a repository.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"},"repo":{"type":"string"},"state":{"type":"string","enum":["open","closed","all"]}},"required":["owner","repo"]}`),
			},
			{
				Name:        "github_create_issue",
				Description: "Create an issue on a GitHub repository (requires confirmation).",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"},"repo":{"type":"string"},"title":{"type":"string"},"body":{"type":"string"}},"required":["owner","repo","title"]}`),
			},
		},
	},
	{
		ID:           "slack",
		Name:         "Slack",
		Mention:      "slack",
		Category:     "Communication",
		Description:  "Send notifications, broadcast updates, and post announcements to Slack.",
		Icon:         "slack",
		RequiresAuth: false,
		AuthType:     "webhook",
		AuthPrompt:   "Optional Slack Webhook URL or Bot Token",
		Tools: []Tool{
			{
				Name:        "slack_post_message",
				Description: "Post a notification or message to a Slack channel or webhook (requires confirmation).",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"channel":{"type":"string","description":"Target channel or webhook"},"text":{"type":"string","description":"Message content"}},"required":["text"]}`),
			},
			{
				Name:        "slack_list_channels",
				Description: "List accessible Slack channels.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
	},
	{
		ID:           "web_fetch",
		Name:         "Web Reader & Fetch",
		Mention:      "web_fetch",
		Category:     "Web & Data",
		Description:  "Read and parse web articles, extract clean text, and inspect page structure.",
		Icon:         "globe",
		RequiresAuth: false,
		Tools: []Tool{
			{
				Name:        "web_fetch",
				Description: "Fetch webpage content and extract clean markdown text and links.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"Valid web URL to fetch"}},"required":["url"]}`),
			},
		},
	},
	{
		ID:           "code_runner",
		Name:         "Code Sandbox",
		Mention:      "code_runner",
		Category:     "Developer",
		Description:  "Perform computations, data transformations, and math calculations safely.",
		Icon:         "terminal",
		RequiresAuth: false,
		Tools: []Tool{
			{
				Name:        "code_runner",
				Description: "Evaluate calculations, math expressions, and format structured data.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string","description":"Expression or calculation to evaluate"}},"required":["expression"]}`),
			},
		},
	},
}

// GetCatalogApp retrieves a catalog item by ID.
func GetCatalogApp(id string) (CatalogApp, bool) {
	for _, app := range DefaultCatalog {
		if app.ID == id {
			return app, true
		}
	}
	return CatalogApp{}, false
}
