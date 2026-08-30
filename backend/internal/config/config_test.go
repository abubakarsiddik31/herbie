package config

import "testing"

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
