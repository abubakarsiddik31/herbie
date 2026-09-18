package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/oauth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// ServerDeps carries the wired collaborators. Stores are narrow interfaces
// (chat.go) so handler tests run offline; *storage.* types satisfy them.
type ServerDeps struct {
	Cfg    config.Config
	Log    *slog.Logger
	Auth   *auth.Service
	Tokens *auth.TokenMaker
	// OAuth lists the configured social-login providers in display
	// order. Empty = password auth only; the login UI hides the
	// provider buttons.
	OAuth     []*oauth.Provider
	Shares    ShareStore
	Profiles  ProfileStore
	Convos    ConvoStore
	Msgs      MsgStore
	Usage     UsageStore
	Tools     ToolStore
	Pending   PendingStore
	Agent     *chat.Agent
	Rates     cost.Table
	ModelKeys chat.ProviderKeys
	// RagSearch retrieves document chunks and returns the query
	// embedding's exact usage for metering. Nil = RAG disabled: no
	// search tool is registered and the documents API answers 503.
	RagSearch RagSearchFunc
	// Compactor summarizes hot histories into the run instructions.
	// Nil = compaction disabled: history passes through verbatim.
	Compactor *chat.Compactor
	// The retrieval stack's collaborators for the documents API. All nil
	// when RAG is disabled.
	RAG     RagRunner
	Docs    DocStore
	Vectors rag.VectorStore
	Objects storage.ObjectStore
}

// RagSearchFunc is rag.Service.Search narrowed to what the chat path
// needs (the score rows plus the query-embedding and rerank usage).
// docIDs empty = all user documents; otherwise restricted to those IDs.
type RagSearchFunc func(ctx context.Context, userID, query string, k int, docIDs []string) ([]rag.Scored, rag.UsageReport, error)

type Server struct {
	deps ServerDeps
	mux  *http.ServeMux
}

// NewServer builds the root handler, CORS outermost. Auth endpoints are
// public on the main mux; everything mounted on the authed submux sits
// behind the bearer-token middleware. Later tasks add routes to authed.
func NewServer(deps ServerDeps) http.Handler {
	s := &Server{deps: deps, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Public (unauthenticated) auth endpoints.
	s.mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/auth/providers", s.handleOAuthProviders)
	s.mux.HandleFunc("GET /api/auth/oauth/{provider}", s.handleOAuthStart)
	s.mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", s.handleOAuthCallback)
	s.mux.HandleFunc("POST /api/auth/oauth/consume", s.handleOAuthConsume)
	s.mux.HandleFunc("GET /api/shared/{token}", s.handleGetShared)

	// Authenticated API: more specific patterns above win over this
	// catch-all, so /api/auth/* stays public while the rest of /api/
	// requires a valid bearer token.
	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/me", s.handleMe)
	authed.HandleFunc("GET /api/profile", s.handleGetProfile)
	authed.HandleFunc("PATCH /api/profile", s.handlePatchProfile)
	authed.HandleFunc("GET /api/models", s.handleListModels)
	authed.HandleFunc("GET /api/conversations", s.handleListConversations)
	authed.HandleFunc("POST /api/conversations", s.handleCreateConversation)
	authed.HandleFunc("GET /api/conversations/{id}", s.handleGetConversation)
	authed.HandleFunc("PATCH /api/conversations/{id}", s.handlePatchConversation)
	authed.HandleFunc("DELETE /api/conversations/{id}", s.handleDeleteConversation)
	authed.HandleFunc("GET /api/conversations/{id}/shares", s.handleShareState)
	authed.HandleFunc("POST /api/conversations/{id}/shares", s.handleShareConversation)
	authed.HandleFunc("DELETE /api/conversations/{id}/shares", s.handleUnshareConversation)
	authed.HandleFunc("POST /api/conversations/{id}/messages", s.handleSendMessage)
	authed.HandleFunc("POST /api/conversations/{id}/messages/{messageId}/edit", s.handleEditMessage)
	authed.HandleFunc("DELETE /api/conversations/{id}/messages/{messageId}", s.handleDeleteMessage)
	authed.HandleFunc("POST /api/conversations/{id}/regenerate", s.handleRegenerate)
	authed.HandleFunc("POST /api/conversations/{id}/approvals", s.handleApprovals)
	authed.HandleFunc("GET /api/tools", s.handleListTools)
	authed.HandleFunc("POST /api/tools", s.handleCreateTool)
	authed.HandleFunc("PATCH /api/tools/{id}", s.handlePatchTool)
	authed.HandleFunc("DELETE /api/tools/{id}", s.handleDeleteTool)
	authed.HandleFunc("GET /api/usage/summary", s.handleUsageSummary)
	authed.HandleFunc("POST /api/documents", s.handleUploadDocument)
	authed.HandleFunc("GET /api/documents", s.handleListDocuments)
	authed.HandleFunc("DELETE /api/documents/{id}", s.handleDeleteDocument)
	s.mux.Handle("/api/", requireAuth(deps.Tokens, authed))

	return withCORS(deps.Cfg.FrontendOrigin, logRequests(deps.Log, s.mux))
}
