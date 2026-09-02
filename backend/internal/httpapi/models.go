package httpapi

import (
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
)

type modelDTO struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Provider string `json:"provider"`
}

// handleListModels advertises the server's selectable models: the catalog
// entries whose provider key is configured, plus the default new
// conversations start with.
func (s *Server) handleListModels(w http.ResponseWriter, _ *http.Request) {
	avail := chat.Available(s.deps.ModelKeys)
	out := make([]modelDTO, 0, len(avail))
	for _, m := range avail {
		out = append(out, modelDTO{ID: m.ID, Label: m.Label, Provider: m.Provider})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"default": chat.DefaultModel(s.deps.ModelKeys).ID,
		"models":  out,
	})
}
