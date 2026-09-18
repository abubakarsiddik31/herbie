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
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/oauth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/httpapi"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem-chatbot/internal/weaviate"
	"github.com/abubakarsiddik31/golem-chatbot/internal/websearch"
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
	svc, err := auth.NewService(users, refresh, storage.NewOAuthIdentities(pool), cfg.JWTSecret)
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
	var ragRunner httpapi.RagRunner
	var vectors rag.VectorStore
	var objects storage.ObjectStore
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
		objs, err := storage.NewMinIOStore(cfg.RAG.MinIOEndpoint, cfg.RAG.MinIOAccessKey,
			cfg.RAG.MinIOSecretKey, cfg.RAG.DocumentsBucket, cfg.RAG.MinIOUseSSL)
		if err != nil {
			log.Error("rag minio", "err", err)
			os.Exit(1)
		}
		svc := rag.NewService(embedder, vs, objs)
		svc.WithTuning(cfg.RAG.RetrievalAlpha, cfg.RAG.RetrieveMult, cfg.RAG.MaxCandidates, cfg.RAG.ExpandBefore, cfg.RAG.ExpandAfter, cfg.RAG.ChunkTargetTokens, cfg.RAG.ChunkOverlapTokens)
		if cfg.RAG.RerankEnabled {
			if client, err := registry.Resolve(chat.RunSpec{Model: cfg.RAG.RerankModel}); err != nil {
				log.Warn("rerank disabled: model unresolvable", "model", cfg.RAG.RerankModel, "err", err)
			} else {
				svc.WithRanker(rag.NewRanker(client, cfg.RAG.RerankModel))
			}
		}
		ragSearch = func(ctx context.Context, userID, query string, k int, docIDs []string) ([]rag.Scored, rag.UsageReport, error) {
			return svc.Search(ctx, userID, query, rag.SearchOptions{TopK: k, DocIDs: docIDs})
		}
		ragRunner = svc
		vectors = vs
		objects = objs
		log.Info("rag enabled", "model", cfg.RAG.EmbeddingModel, "weaviate", cfg.RAG.WeaviateURL, "minio", cfg.RAG.MinIOEndpoint)
	}

	// Compaction benefits every long thread, not just RAG runs, so the
	// compactor builds whenever its model resolves — independent of the
	// RAG-enabled gate above.
	var compactor *chat.Compactor
	if cfg.RAG.CompactionEnabled {
		if client, err := registry.Resolve(chat.RunSpec{Model: cfg.RAG.CompactionModel}); err != nil {
			log.Warn("compaction disabled: model unresolvable", "model", cfg.RAG.CompactionModel, "err", err)
		} else {
			compactor = chat.NewCompactor(client, cfg.RAG.CompactionModel, cfg.RAG.CompactionThreshold, cfg.RAG.CompactionKeepRecent, cfg.RAG.CompactionSummaryTokens)
		}
	}

	var webSearch websearch.Searcher
	if cfg.WebSearch.Enabled() {
		webSearch = websearch.New(cfg.WebSearch)
		log.Info("web search enabled", "provider", cfg.WebSearch.Provider)
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
		OAuth:     oauthProviders(cfg),
		Convos:    storage.NewConversations(pool),
		Msgs:      storage.NewMessages(pool),
		Shares:    storage.NewShares(pool),
		Profiles:  users,
		Memories:  storage.NewMemories(pool),
		Usage:     storage.NewUsage(pool),
		Tools:     storage.NewTools(pool),
		Pending:   storage.NewPendingCalls(pool),
		Agent:     agent,
		Rates:     rates,
		ModelKeys: modelKeys,
		RagSearch: ragSearch,
		WebSearch: webSearch,
		Compactor: compactor,
		RAG:       ragRunner,
		Docs:      storage.NewDocuments(pool),
		Vectors:   vectors,
		Objects:   objects,
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

// oauthProviders builds the configured social-login providers in display
// order. Unconfigured providers are omitted; with none set the server
// offers password auth only.
func oauthProviders(cfg config.Config) []*oauth.Provider {
	callback := func(id string) string {
		return cfg.OAuth.RedirectBase + "/api/auth/oauth/" + id + "/callback"
	}
	var providers []*oauth.Provider
	if cfg.OAuth.Google.Enabled() {
		providers = append(providers, oauth.Google(
			cfg.OAuth.Google.ClientID, cfg.OAuth.Google.Secret, callback("google")))
	}
	if cfg.OAuth.GitHub.Enabled() {
		providers = append(providers, oauth.GitHub(
			cfg.OAuth.GitHub.ClientID, cfg.OAuth.GitHub.Secret, callback("github")))
	}
	return providers
}
