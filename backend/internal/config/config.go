package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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
		},
	}
	var errs []error
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
