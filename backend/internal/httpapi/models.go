package httpapi

import (
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
)

type modelDTO struct {
	ID           string          `json:"id"`
	Label        string          `json:"label"`
	Provider     string          `json:"provider"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
}

// handleListModels advertises the server's selectable models: the catalog
// entries whose provider key is configured, plus the default new
// conversations start with.
func (s *Server) handleListModels(w http.ResponseWriter, _ *http.Request) {
	avail := chat.Available(s.deps.ModelKeys)
	out := make([]modelDTO, 0, len(avail)+1)
	if len(avail) > 0 {
		out = append(out, modelDTO{ID: chat.AutoModelID, Label: "Auto (fallback)", Provider: "auto"})
	}
	for _, m := range avail {
		out = append(out, modelDTO{ID: m.ID, Label: m.Label, Provider: m.Provider, Capabilities: m.Capabilities})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"default": chat.DefaultModel(s.deps.ModelKeys).ID,
		"models":  out,
	})
}
