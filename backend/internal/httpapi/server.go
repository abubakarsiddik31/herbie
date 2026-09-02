package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
)

// ServerDeps carries the wired collaborators. Stores are narrow interfaces
// (chat.go) so handler tests run offline; *storage.* types satisfy them.
type ServerDeps struct {
	Cfg       config.Config
	Log       *slog.Logger
	Auth      *auth.Service
	Tokens    *auth.TokenMaker
	Convos    ConvoStore
	Msgs      MsgStore
	Usage     UsageStore
	Tools     ToolStore
	Pending   PendingStore
	Agent     *chat.Agent
	Rates     cost.Table
	ModelKeys chat.ProviderKeys
}

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

	// Authenticated API: more specific patterns above win over this
	// catch-all, so /api/auth/* stays public while the rest of /api/
	// requires a valid bearer token.
	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/me", s.handleMe)
	authed.HandleFunc("GET /api/conversations", s.handleListConversations)
	authed.HandleFunc("POST /api/conversations", s.handleCreateConversation)
	authed.HandleFunc("GET /api/conversations/{id}", s.handleGetConversation)
	authed.HandleFunc("PATCH /api/conversations/{id}", s.handlePatchConversation)
	authed.HandleFunc("DELETE /api/conversations/{id}", s.handleDeleteConversation)
	authed.HandleFunc("POST /api/conversations/{id}/messages", s.handleSendMessage)
	authed.HandleFunc("POST /api/conversations/{id}/approvals", s.handleApprovals)
	authed.HandleFunc("GET /api/tools", s.handleListTools)
	authed.HandleFunc("POST /api/tools", s.handleCreateTool)
	authed.HandleFunc("PATCH /api/tools/{id}", s.handlePatchTool)
	authed.HandleFunc("DELETE /api/tools/{id}", s.handleDeleteTool)
	authed.HandleFunc("GET /api/usage/summary", s.handleUsageSummary)
	s.mux.Handle("/api/", requireAuth(deps.Tokens, authed))

	return withCORS(deps.Cfg.FrontendOrigin, logRequests(deps.Log, s.mux))
}
