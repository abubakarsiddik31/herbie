package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
)

// ServerDeps carries the wired collaborators. Fields join as later tasks
// land (stores, agent).
type ServerDeps struct {
	Cfg    config.Config
	Log    *slog.Logger
	Auth   *auth.Service
	Tokens *auth.TokenMaker
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
	// later tasks mount more authed routes here
	s.mux.Handle("/api/", requireAuth(deps.Tokens, authed))

	return withCORS(deps.Cfg.FrontendOrigin, logRequests(deps.Log, s.mux))
}
