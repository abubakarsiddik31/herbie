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
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem-chatbot/internal/weaviate"
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

	modelKeys := chat.ProviderKeys{
		Gemini: cfg.GeminiAPIKey, GeminiBaseURL: cfg.GeminiBaseURL,
		OpenAI: cfg.OpenAIAPIKey, OpenAIBaseURL: cfg.OpenAIBaseURL,
		Anthropic: cfg.AnthropicAPIKey, AnthropicBaseURL: cfg.AnthropicBaseURL,
	}
	registry := chat.NewModelRegistry(modelKeys)
	agent, err := chat.New(registry, golem.UsageLimit{Requests: 12, TotalTokens: 100_000}, chat.ToolEnv{
		HTTPTimeout:       cfg.ToolHTTPTimeout,
		HTTPMaxBytes:      cfg.ToolHTTPMaxBytes,
		ResultMaxBytes:    cfg.ToolResultMaxBytes,
		AllowPrivateHosts: cfg.ToolAllowPrivateHosts,
	})
	if err != nil {
		log.Error("agent", "err", err)
		os.Exit(1)
	}

	// The RAG stack boots only when explicitly enabled: embedder, vector
	// store, and object store all need the rag compose profile running.
	var ragSearch httpapi.RagSearchFunc
	if cfg.RAG.Enabled {
		if cfg.GeminiAPIKey == "" {
			log.Error("rag", "err", "RAG_ENABLED requires GEMINI_API_KEY for embeddings")
			os.Exit(1)
		}
		embedder, err := rag.NewEmbedder(cfg.GeminiAPIKey, cfg.RAG.EmbeddingModel, cfg.RAG.EmbeddingDims, cfg.RAG.EmbedBatchSize)
		if err != nil {
			log.Error("rag embedder", "err", err)
			os.Exit(1)
		}
		vs := weaviate.New(cfg.RAG.WeaviateURL, cfg.RAG.EmbeddingDims, nil)
		if err := vs.EnsureCollection(ctx); err != nil {
			log.Error("rag weaviate", "err", err)
			os.Exit(1)
		}
		objects, err := storage.NewMinIOStore(cfg.RAG.MinIOEndpoint, cfg.RAG.MinIOAccessKey,
			cfg.RAG.MinIOSecretKey, cfg.RAG.DocumentsBucket, cfg.RAG.MinIOUseSSL)
		if err != nil {
			log.Error("rag minio", "err", err)
			os.Exit(1)
		}
		svc := rag.NewService(embedder, vs, objects)
		ragSearch = svc.Search
		log.Info("rag enabled", "model", cfg.RAG.EmbeddingModel, "weaviate", cfg.RAG.WeaviateURL, "minio", cfg.RAG.MinIOEndpoint)
	}

	// Per-model ledger rates from the catalog; the env rates stay as the
	// fallback for models without a catalog entry.
	byModel := make(map[string]cost.Rates, len(chat.Catalog())+1)
	for _, m := range chat.Catalog() {
		byModel[m.ID] = cost.Rates{ChatInputPerM: m.InputPerM, ChatOutputPerM: m.OutputPerM}
	}
	if cfg.RAG.Enabled {
		byModel[cfg.RAG.EmbeddingModel] = cost.Rates{ChatInputPerM: cfg.RAG.EmbeddingInputRate}
	}
	rates := cost.Table{
		Default: cost.Rates{ChatInputPerM: cfg.ChatInputRate, ChatOutputPerM: cfg.ChatOutputRate},
		ByModel: byModel,
	}

	handler := httpapi.NewServer(httpapi.ServerDeps{
		Cfg:       cfg,
		Log:       log,
		Auth:      svc,
		Tokens:    tokens,
		Convos:    storage.NewConversations(pool),
		Msgs:      storage.NewMessages(pool),
		Usage:     storage.NewUsage(pool),
		Tools:     storage.NewTools(pool),
		Pending:   storage.NewPendingCalls(pool),
		Agent:     agent,
		Rates:     rates,
		ModelKeys: modelKeys,
		RagSearch: ragSearch,
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
