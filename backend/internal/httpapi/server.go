package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
)

// ServerDeps carries the wired collaborators. Fields join as later tasks
// land (auth service, stores, agent).
type ServerDeps struct {
	Cfg config.Config
	Log *slog.Logger
}

func NewServer(deps ServerDeps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return logRequests(deps.Log, mux)
}
