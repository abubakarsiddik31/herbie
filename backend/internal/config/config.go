package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/websearch"
)

// Config carries every runtime setting. Values come from the environment;
// a .env file in the working directory or repo root is loaded first, dev
// convenience only.
type Config struct {
	Port             string
	DatabaseURL      string
	GeminiAPIKey     string
	GeminiModel      string
	GeminiBaseURL    string
	OpenAIAPIKey     string
	OpenAIBaseURL    string
	AnthropicAPIKey  string
	AnthropicBaseURL string
	JWTSecret        string
	FrontendOrigin   string
	ChatInputRate    float64 // USD per 1M input tokens (fallback rate)
	ChatOutputRate   float64 // USD per 1M output tokens (fallback rate)

	// User-tool runtime settings.
	ToolHTTPTimeout       time.Duration
	ToolHTTPMaxBytes      int64
	ToolResultMaxBytes    int64
	ToolAllowPrivateHosts bool
	MaxToolsPerUser       int

	// RAGConfig gates the retrieval stack. Enabled is an explicit opt-in
	// so a running stack never requires the rag compose profile.
	RAG RAGConfig

	// OAuth carries social-login provider credentials. A provider is
	// active only when both its client ID and secret are set; the login
	// UI hides providers that are not configured.
	OAuth OAuthConfig

	// WebSearch configures live web search (Tavily or Brave). When an API key
	// is provided, the built-in web_search tool is registered.
	WebSearch                websearch.Config
	WebSearchRequireApproval bool
}

type RAGConfig struct {
	Enabled            bool
	WeaviateURL        string
	MinIOEndpoint      string
	MinIOAccessKey     string
	MinIOSecretKey     string
	MinIOUseSSL        bool
	DocumentsBucket    string
	EmbeddingModel     string
	EmbeddingDims      int
	EmbedBatchSize     int
	EmbeddingInputRate float64
	MaxUploadBytes     int64

	RetrievalAlpha          float64
	RetrieveMult            int
	MaxCandidates           int
	RerankEnabled           bool
	RerankModel             string
	ChunkTargetTokens       int
	ChunkOverlapTokens      int
	ExpandBefore            int
	ExpandAfter             int
	CompactionEnabled       bool
	CompactionModel         string
	CompactionThreshold     int
	CompactionKeepRecent    int
	CompactionSummaryTokens int
}

type OAuthProviderConfig struct {
	ClientID string
	Secret   string
}

func (c OAuthProviderConfig) Enabled() bool {
	return c.ClientID != "" && c.Secret != ""
}

type OAuthConfig struct {
	Google       OAuthProviderConfig
	GitHub       OAuthProviderConfig
	RedirectBase string
}

// dotenvPaths are where a repo-root .env may sit relative to the working
// directory: `make backend` runs the server from backend/, so the repo
// root's .env is one level up.
var dotenvPaths = []string{".env", "../.env", "../../.env"}

func Load() (Config, error) {
	for _, path := range dotenvPaths {
		if _, err := os.Stat(path); err == nil {
			loadDotenv(path)
			break
		}
	}
	inputRate, err := envFloat("CHAT_INPUT_USD_PER_MTOK", 0.30)
	if err != nil {
		return Config{}, err
	}
	outputRate, err := envFloat("CHAT_OUTPUT_USD_PER_MTOK", 2.50)
	if err != nil {
		return Config{}, err
	}
	toolTimeout, err := envInt("TOOL_HTTP_TIMEOUT", 20)
	if err != nil {
		return Config{}, err
	}
	maxTools, err := envInt("MAX_TOOLS_PER_USER", 20)
	if err != nil {
		return Config{}, err
	}
	dims, err := envInt("EMBEDDING_DIMS", 768)
	if err != nil {
		return Config{}, err
	}
	batch, err := envInt("EMBEDDING_BATCH", 96)
	if err != nil {
		return Config{}, err
	}
	embedRate, err := envFloat("EMBEDDING_INPUT_USD_PER_MTOK", 0.15)
	if err != nil {
		return Config{}, err
	}
	alpha, err := envFloat("RETRIEVAL_ALPHA", 0.5)
	if err != nil {
		return Config{}, err
	}
	if alpha < 0 || alpha > 1 {
		return Config{}, fmt.Errorf("invalid RETRIEVAL_ALPHA: %q (want 0..1)", os.Getenv("RETRIEVAL_ALPHA"))
	}
	retrieveMult, err := envInt("RETRIEVE_MULT", 4)
	if err != nil {
		return Config{}, err
	}
	maxCand, err := envInt("RERANK_MAX_CANDIDATES", 40)
	if err != nil {
		return Config{}, err
	}
	chunkTarget, err := envInt("CHUNK_TARGET_TOKENS", 512)
	if err != nil {
		return Config{}, err
	}
	chunkOverlap, err := envInt("CHUNK_OVERLAP_TOKENS", 64)
	if err != nil {
		return Config{}, err
	}
	if chunkTarget <= 0 || chunkOverlap < 0 || chunkOverlap >= chunkTarget {
		return Config{}, fmt.Errorf("invalid chunk budget: target=%d overlap=%d", chunkTarget, chunkOverlap)
	}
	compThreshold, err := envInt("COMPACTION_THRESHOLD_TOKENS", 40000)
	if err != nil {
		return Config{}, err
	}
	if compThreshold <= 0 {
		return Config{}, fmt.Errorf("invalid COMPACTION_THRESHOLD_TOKENS: %d", compThreshold)
	}
	compKeep, err := envInt("COMPACTION_KEEP_RECENT", 10)
	if err != nil {
		return Config{}, err
	}
	compSummary, err := envInt("COMPACTION_SUMMARY_TOKENS", 800)
	if err != nil {
		return Config{}, err
	}

	wsProvider := strings.ToLower(env("WEB_SEARCH_PROVIDER", ""))
	wsKey := os.Getenv("WEB_SEARCH_API_KEY")
	tavilyKey := os.Getenv("TAVILY_API_KEY")
	braveKey := os.Getenv("BRAVE_API_KEY")
	if tavilyKey != "" && wsKey == "" {
		wsKey = tavilyKey
		if wsProvider == "" {
			wsProvider = "tavily"
		}
	} else if braveKey != "" && wsKey == "" {
		wsKey = braveKey
		if wsProvider == "" {
			wsProvider = "brave"
		}
	}
	if wsProvider == "" {
		wsProvider = "tavily"
	}
	wsTimeout, err := envInt("WEB_SEARCH_TIMEOUT", 15)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Port:             env("APP_PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
		GeminiModel:      env("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiBaseURL:    os.Getenv("GEMINI_BASE_URL"),
		OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL:    os.Getenv("OPENAI_BASE_URL"),
		AnthropicAPIKey:  os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicBaseURL: os.Getenv("ANTHROPIC_BASE_URL"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		FrontendOrigin:   env("FRONTEND_ORIGIN", "http://localhost:5173"),
		ChatInputRate:    inputRate,
		ChatOutputRate:   outputRate,

		OAuth: OAuthConfig{
			Google: OAuthProviderConfig{
				ClientID: os.Getenv("OAUTH_GOOGLE_CLIENT_ID"),
				Secret:   os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET"),
			},
			GitHub: OAuthProviderConfig{
				ClientID: os.Getenv("OAUTH_GITHUB_CLIENT_ID"),
				Secret:   os.Getenv("OAUTH_GITHUB_CLIENT_SECRET"),
			},
			RedirectBase: os.Getenv("OAUTH_REDIRECT_BASE"),
		},

		WebSearch: websearch.Config{
			Provider: wsProvider,
			APIKey:   wsKey,
			BaseURL:  os.Getenv("WEB_SEARCH_BASE_URL"),
			Timeout:  time.Duration(wsTimeout) * time.Second,
		},
		WebSearchRequireApproval: envBool("WEB_SEARCH_REQUIRE_APPROVAL", true),

		ToolHTTPTimeout:       time.Duration(toolTimeout) * time.Second,
		ToolHTTPMaxBytes:      envInt64("TOOL_HTTP_MAX_BYTES", 1<<20),
		ToolResultMaxBytes:    envInt64("TOOL_RESULT_MAX_BYTES", 32<<10),
		ToolAllowPrivateHosts: envBool("TOOL_ALLOW_PRIVATE_HOSTS", false),
		MaxToolsPerUser:       maxTools,

		RAG: RAGConfig{
			Enabled:            envBool("RAG_ENABLED", false),
			WeaviateURL:        env("WEAVIATE_URL", "http://localhost:8081"),
			MinIOEndpoint:      env("MINIO_ENDPOINT", "localhost:9000"),
			MinIOAccessKey:     env("MINIO_ACCESS_KEY", "golem"),
			MinIOSecretKey:     env("MINIO_SECRET_KEY", "golem1234"),
			MinIOUseSSL:        envBool("MINIO_USE_SSL", false),
			DocumentsBucket:    env("DOCUMENTS_BUCKET", "golem-chatbot-documents"),
			EmbeddingModel:     env("EMBEDDING_MODEL", "gemini-embedding-001"),
			EmbeddingDims:      dims,
			EmbedBatchSize:     batch,
			EmbeddingInputRate: embedRate,
			MaxUploadBytes:     envInt64("MAX_UPLOAD_BYTES", 20<<20),

			RetrievalAlpha:          alpha,
			RetrieveMult:            retrieveMult,
			MaxCandidates:           maxCand,
			RerankEnabled:           envBool("RERANK_ENABLED", true),
			RerankModel:             env("RERANK_MODEL", "gemini-2.5-flash"),
			ChunkTargetTokens:       chunkTarget,
			ChunkOverlapTokens:      chunkOverlap,
			ExpandBefore:            envIntOr("EXPAND_BEFORE", 1),
			ExpandAfter:             envIntOr("EXPAND_AFTER", 1),
			CompactionEnabled:       envBool("COMPACTION_ENABLED", true),
			CompactionModel:         env("COMPACTION_MODEL", "gemini-2.5-flash"),
			CompactionThreshold:     compThreshold,
			CompactionKeepRecent:    compKeep,
			CompactionSummaryTokens: compSummary,
		},
	}
	var errs []error
	if cfg.OAuth.RedirectBase == "" {
		cfg.OAuth.RedirectBase = "http://localhost:" + cfg.Port
	}
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if cfg.GeminiAPIKey == "" && cfg.OpenAIAPIKey == "" && cfg.AnthropicAPIKey == "" {
		errs = append(errs, errors.New("at least one provider key is required (GEMINI_API_KEY, OPENAI_API_KEY, or ANTHROPIC_API_KEY)"))
	}
	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 bytes"))
	}
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %q", key, v)
	}
	return f, nil
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %q", key, v)
	}
	return n, nil
}

func envInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envIntOr(key string, def int) int {
	if n, err := envInt(key, def); err == nil && n >= 0 {
		return n
	}
	return def
}

// loadDotenv applies KEY=VALUE lines that are not already in the
// environment. Existing environment always wins.
func loadDotenv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
