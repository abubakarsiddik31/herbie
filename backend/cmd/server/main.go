package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/httpapi"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	golemgemini "github.com/abubakarsiddik31/golem/providers/gemini"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := storage.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := storage.Migrate(ctx, pool); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	users := storage.NewUsers(pool)
	refresh := storage.NewRefreshTokens(pool)
	svc, err := auth.NewService(users, refresh, cfg.JWTSecret)
	if err != nil {
		log.Error("auth", "err", err)
		os.Exit(1)
	}
	tokens, err := auth.NewTokenMaker(cfg.JWTSecret)
	if err != nil {
		log.Error("auth", "err", err)
		os.Exit(1)
	}

	gemini, err := golemgemini.New(golemgemini.Config{
		APIKey: cfg.GeminiAPIKey, Model: cfg.GeminiModel, BaseURL: cfg.GeminiBaseURL,
	})
	if err != nil {
		log.Error("gemini", "err", err)
		os.Exit(1)
	}
	agent, err := chat.New(gemini, golem.UsageLimit{Requests: 12, TotalTokens: 100_000})
	if err != nil {
		log.Error("agent", "err", err)
		os.Exit(1)
	}

	handler := httpapi.NewServer(httpapi.ServerDeps{
		Cfg:    cfg,
		Log:    log,
		Auth:   svc,
		Tokens: tokens,
		Convos: storage.NewConversations(pool),
		Msgs:   storage.NewMessages(pool),
		Usage:  storage.NewUsage(pool),
		Agent:  agent,
		Rates:  cost.Rates{ChatInputPerM: cfg.ChatInputRate, ChatOutputPerM: cfg.ChatOutputRate},
	})
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Info("listening", "port", cfg.Port)

	select {
	case err := <-errCh:
		log.Error("server", "err", err)
		os.Exit(1)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("shutdown", "err", err)
	}
	log.Info("stopped")
}
