package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRequiresSecrets(t *testing.T) {
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
