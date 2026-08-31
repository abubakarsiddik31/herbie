package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config carries every runtime setting. Values come from the environment;
// a .env file in the working directory or repo root is loaded first, dev
// convenience only.
type Config struct {
	Port           string
	DatabaseURL    string
	GeminiAPIKey   string
	GeminiModel    string
	GeminiBaseURL  string
	JWTSecret      string
	FrontendOrigin string
	ChatInputRate  float64 // USD per 1M input tokens
	ChatOutputRate float64 // USD per 1M output tokens
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
	cfg := Config{
		Port:           env("APP_PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		GeminiAPIKey:   os.Getenv("GEMINI_API_KEY"),
		GeminiModel:    env("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiBaseURL:  os.Getenv("GEMINI_BASE_URL"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		FrontendOrigin: env("FRONTEND_ORIGIN", "http://localhost:5173"),
		ChatInputRate:  inputRate,
		ChatOutputRate: outputRate,
	}
	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if cfg.GeminiAPIKey == "" {
		errs = append(errs, errors.New("GEMINI_API_KEY is required"))
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
