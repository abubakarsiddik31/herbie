package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadToolDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ToolHTTPTimeout != 20*time.Second {
		t.Fatalf("ToolHTTPTimeout = %s", cfg.ToolHTTPTimeout)
	}
	if cfg.ToolHTTPMaxBytes != 1<<20 || cfg.ToolResultMaxBytes != 32<<10 {
		t.Fatalf("byte defaults: %d %d", cfg.ToolHTTPMaxBytes, cfg.ToolResultMaxBytes)
	}
	if cfg.ToolAllowPrivateHosts || cfg.MaxToolsPerUser != 20 {
		t.Fatalf("flags: %v %d", cfg.ToolAllowPrivateHosts, cfg.MaxToolsPerUser)
	}
}

func TestLoadToolOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("TOOL_HTTP_TIMEOUT", "5")
	t.Setenv("TOOL_ALLOW_PRIVATE_HOSTS", "true")
	t.Setenv("MAX_TOOLS_PER_USER", "3")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ToolHTTPTimeout != 5*time.Second || !cfg.ToolAllowPrivateHosts || cfg.MaxToolsPerUser != 3 {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestLoadRequiresSecrets(t *testing.T) {
	t.Setenv("APP_PORT", "8080")
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short JWT secret")
	}
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8080" || cfg.GeminiModel != "gemini-2.5-flash" {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if cfg.ChatInputRate != 0.30 || cfg.ChatOutputRate != 2.50 {
		t.Fatalf("rate defaults: %+v", cfg)
	}
	t.Setenv("CHAT_INPUT_USD_PER_MTOK", "0,5")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for malformed rate override")
	}
}

func TestLoadReadsParentDotenv(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "backend")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("GEMINI_MODEL=from-parent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	t.Chdir(sub)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GeminiModel != "from-parent" {
		t.Fatalf("parent .env not loaded: model = %q", cfg.GeminiModel)
	}
}

func TestLoadAcceptsAnySingleProviderKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("openai-only config must load: %v", err)
	}
	if cfg.OpenAIAPIKey != "sk-test" || cfg.OpenAIBaseURL != "" {
		t.Fatalf("openai key not loaded: %+v", cfg)
	}

	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "ak-test")
	t.Setenv("ANTHROPIC_BASE_URL", "https://proxy.example.com")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("anthropic-only config must load: %v", err)
	}
	if cfg.AnthropicAPIKey != "ak-test" || cfg.AnthropicBaseURL != "https://proxy.example.com" {
		t.Fatalf("anthropic settings not loaded: %+v", cfg)
	}
}

func TestLoadRejectsMissingProviderKeys(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when no provider key is configured")
	}
}

func TestRAGDefaultsDisabled(t *testing.T) {
	os.Unsetenv("RAG_ENABLED")
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RAG.Enabled {
		t.Fatal("RAG must default to disabled")
	}
	if cfg.RAG.EmbeddingModel != "gemini-embedding-001" {
		t.Fatalf("EmbeddingModel = %q", cfg.RAG.EmbeddingModel)
	}
	if cfg.RAG.EmbeddingDims != 768 || cfg.RAG.EmbedBatchSize != 96 {
		t.Fatalf("dims/batch: %d %d", cfg.RAG.EmbeddingDims, cfg.RAG.EmbedBatchSize)
	}
	if cfg.RAG.MaxUploadBytes != 20<<20 {
		t.Fatalf("MaxUploadBytes = %d", cfg.RAG.MaxUploadBytes)
	}
	if cfg.RAG.EmbeddingInputRate != 0.15 {
		t.Fatalf("EmbeddingInputRate = %f", cfg.RAG.EmbeddingInputRate)
	}
	if cfg.RAG.DocumentsBucket != "golem-chatbot-documents" || cfg.RAG.WeaviateURL != "http://localhost:8081" {
		t.Fatalf("infra defaults: %+v", cfg.RAG)
	}
}

func TestRAGEnabledParsesEnv(t *testing.T) {
	t.Setenv("RAG_ENABLED", "true")
	t.Setenv("EMBEDDING_MODEL", "gemini-embedding-001")
	t.Setenv("EMBEDDING_DIMS", "768")
	t.Setenv("MINIO_ENDPOINT", "localhost:9000")
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RAG.Enabled || cfg.RAG.MinIOEndpoint != "localhost:9000" {
		t.Fatalf("env not parsed: %+v", cfg.RAG)
	}
}

func TestRAGBadDimsFails(t *testing.T) {
	t.Setenv("RAG_ENABLED", "true")
	t.Setenv("EMBEDDING_DIMS", "zero")
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	if _, err := Load(); err == nil {
		t.Fatal("want error for bad dims")
	}
}

func TestRAGRetrievalDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RAG.RetrievalAlpha != 0.5 {
		t.Fatalf("RetrievalAlpha = %v", cfg.RAG.RetrievalAlpha)
	}
	if cfg.RAG.RetrieveMult != 4 || cfg.RAG.MaxCandidates != 40 {
		t.Fatalf("retrieve defaults: mult=%d max=%d", cfg.RAG.RetrieveMult, cfg.RAG.MaxCandidates)
	}
	if !cfg.RAG.RerankEnabled || cfg.RAG.RerankModel != "gemini-2.5-flash" {
		t.Fatalf("rerank defaults: %v %q", cfg.RAG.RerankEnabled, cfg.RAG.RerankModel)
	}
	if cfg.RAG.ChunkTargetTokens != 512 || cfg.RAG.ChunkOverlapTokens != 64 {
		t.Fatalf("chunk defaults: %d/%d", cfg.RAG.ChunkTargetTokens, cfg.RAG.ChunkOverlapTokens)
	}
	if cfg.RAG.ExpandBefore != 1 || cfg.RAG.ExpandAfter != 1 {
		t.Fatalf("expand defaults: %d/%d", cfg.RAG.ExpandBefore, cfg.RAG.ExpandAfter)
	}
	if !cfg.RAG.CompactionEnabled || cfg.RAG.CompactionModel != "gemini-2.5-flash" {
		t.Fatalf("compaction defaults: %v %q", cfg.RAG.CompactionEnabled, cfg.RAG.CompactionModel)
	}
	if cfg.RAG.CompactionThreshold != 40000 || cfg.RAG.CompactionKeepRecent != 10 || cfg.RAG.CompactionSummaryTokens != 800 {
		t.Fatalf("compaction budgets: %+v", cfg.RAG)
	}
}

func TestRAGRetrievalOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("RETRIEVAL_ALPHA", "0.7")
	t.Setenv("RETRIEVE_MULT", "2")
	t.Setenv("RERANK_ENABLED", "false")
	t.Setenv("RERANK_MODEL", "gpt-4.1")
	t.Setenv("CHUNK_TARGET_TOKENS", "256")
	t.Setenv("EXPAND_BEFORE", "2")
	t.Setenv("COMPACTION_THRESHOLD_TOKENS", "20000")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RAG.RetrievalAlpha != 0.7 || cfg.RAG.RetrieveMult != 2 {
		t.Fatalf("overrides: %+v", cfg.RAG)
	}
	if cfg.RAG.RerankEnabled || cfg.RAG.RerankModel != "gpt-4.1" {
		t.Fatalf("rerank overrides: %+v", cfg.RAG)
	}
	if cfg.RAG.ChunkTargetTokens != 256 || cfg.RAG.ExpandBefore != 2 || cfg.RAG.CompactionThreshold != 20000 {
		t.Fatalf("budget overrides: %+v", cfg.RAG)
	}
}

func TestRAGBadAlphaFails(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("RETRIEVAL_ALPHA", "1.5")
	if _, err := Load(); err == nil {
		t.Fatal("want error for alpha > 1")
	}
}

func TestRAGBadChunkFails(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("CHUNK_OVERLAP_TOKENS", "512")
	t.Setenv("CHUNK_TARGET_TOKENS", "256")
	if _, err := Load(); err == nil {
		t.Fatal("want error for overlap >= target")
	}
}

func TestRAGBadCompactionFails(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("COMPACTION_THRESHOLD_TOKENS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("want error for non-positive compaction threshold")
	}
}

func TestWigoloConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/gc")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("GEMINI_API_KEY", "k")
	t.Setenv("WIGOLO_URL", "http://wigolo:3333")
	t.Setenv("WIGOLO_API_TOKEN", "test-token")
	t.Setenv("WIGOLO_PATH", "/path/to/dist/index.js")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.WebSearch.Provider != "wigolo" {
		t.Errorf("Provider = %q, want %q", cfg.WebSearch.Provider, "wigolo")
	}
	if cfg.WebSearch.BaseURL != "http://wigolo:3333" {
		t.Errorf("BaseURL = %q, want %q", cfg.WebSearch.BaseURL, "http://wigolo:3333")
	}
	if cfg.WebSearch.APIKey != "test-token" {
		t.Errorf("APIKey = %q, want %q", cfg.WebSearch.APIKey, "test-token")
	}
	if cfg.WebSearch.BinPath != "/path/to/dist/index.js" {
		t.Errorf("BinPath = %q, want %q", cfg.WebSearch.BinPath, "/path/to/dist/index.js")
	}
}
