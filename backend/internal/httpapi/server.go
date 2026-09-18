package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/oauth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/cost"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem-chatbot/internal/websearch"
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
	Memories  MemoryStore
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
	// WebSearch provides public web retrieval. Nil = web search disabled:
	// the web_search tool is not registered.
	WebSearch websearch.Searcher
	// Compactor summarizes hot histories into the run instructions.
	// Nil = compaction disabled: history passes through verbatim.
	Compactor *chat.Compactor
	// The retrieval stack's collaborators for the documents API. All nil
	// when RAG is disabled.
	RAG       RagRunner
	Docs      DocStore
	Projects  ProjectStore
	Workflows WorkflowStore
	Vectors   rag.VectorStore
	Objects   storage.ObjectStore
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
	s.mux.HandleFunc("POST /api/webhooks/{slug}", s.handlePublicWebhook)
	s.mux.HandleFunc("GET /api/webhooks/{slug}", s.handlePublicWebhook)

	// Authenticated API: more specific patterns above win over this
	// catch-all, so /api/auth/* stays public while the rest of /api/
	// requires a valid bearer token.
	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/me", s.handleMe)
	authed.HandleFunc("GET /api/profile", s.handleGetProfile)
	authed.HandleFunc("PATCH /api/profile", s.handlePatchProfile)
	authed.HandleFunc("GET /api/memories", s.handleListMemories)
	authed.HandleFunc("POST /api/memories", s.handleCreateMemory)
	authed.HandleFunc("DELETE /api/memories/{id}", s.handleDeleteMemory)
	authed.HandleFunc("DELETE /api/memories", s.handleClearMemories)
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
	authed.HandleFunc("POST /api/extract-text", s.handleExtractText)
	authed.HandleFunc("GET /api/projects", s.handleListProjects)
	authed.HandleFunc("POST /api/projects", s.handleCreateProject)
	authed.HandleFunc("GET /api/projects/{id}", s.handleGetProject)
	authed.HandleFunc("PATCH /api/projects/{id}", s.handlePatchProject)
	authed.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)
	authed.HandleFunc("POST /api/projects/{id}/files", s.handleUploadProjectFile)
	authed.HandleFunc("GET /api/projects/{id}/files", s.handleListProjectFiles)
	authed.HandleFunc("POST /api/projects/{id}/conversations", s.handleCreateProjectConversation)
	authed.HandleFunc("GET /api/projects/{id}/conversations", s.handleListProjectConversations)
	authed.HandleFunc("GET /api/workflows", s.handleListWorkflows)
	authed.HandleFunc("POST /api/workflows", s.handleCreateWorkflow)
	authed.HandleFunc("GET /api/workflows/{id}", s.handleGetWorkflow)
	authed.HandleFunc("PATCH /api/workflows/{id}", s.handlePatchWorkflow)
	authed.HandleFunc("DELETE /api/workflows/{id}", s.handleDeleteWorkflow)
	authed.HandleFunc("POST /api/workflows/{id}/run", s.handleRunWorkflow)
	authed.HandleFunc("GET /api/workflows/{id}/runs", s.handleListWorkflowRuns)
	authed.HandleFunc("GET /api/workflows/{id}/runs/{runId}", s.handleGetWorkflowRun)
	authed.HandleFunc("GET /api/workflow-credentials", s.handleListWorkflowCredentials)
	authed.HandleFunc("POST /api/workflow-credentials", s.handleCreateWorkflowCredential)
	authed.HandleFunc("DELETE /api/workflow-credentials/{id}", s.handleDeleteWorkflowCredential)
	s.mux.Handle("/api/", requireAuth(deps.Tokens, authed))

	return withCORS(deps.Cfg.FrontendOrigin, logRequests(deps.Log, s.mux))
}

func (s *Server) isSecure(r *http.Request) bool {
	if r != nil {
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			return true
		}
	}
	return strings.HasPrefix(s.deps.Cfg.FrontendOrigin, "https://") ||
		strings.HasPrefix(s.deps.Cfg.OAuth.RedirectBase, "https://")
}
