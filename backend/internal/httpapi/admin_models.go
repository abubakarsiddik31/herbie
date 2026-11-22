package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// ModelCatalogStore is the admin-managed model catalog surface.
type ModelCatalogStore interface {
	List(ctx context.Context) ([]storage.ModelCatalogRow, error)
	Get(ctx context.Context, id string) (storage.ModelCatalogRow, error)
	Upsert(ctx context.Context, m storage.ModelCatalogRow) (storage.ModelCatalogRow, error)
	Delete(ctx context.Context, id string) error
}

var _ ModelCatalogStore = (*storage.ModelCatalog)(nil)

type modelCatalogDTO struct {
	ID           string          `json:"id"`
	Provider     string          `json:"provider"`
	Label        string          `json:"label"`
	InputPerM    float64         `json:"inputPerM"`
	OutputPerM   float64         `json:"outputPerM"`
	Enabled      bool            `json:"enabled"`
	Capabilities json.RawMessage `json:"capabilities"`
	SortOrder    int             `json:"sortOrder"`
	UpdatedAt    string          `json:"updatedAt"`
}

func toModelCatalogDTO(m storage.ModelCatalogRow) modelCatalogDTO {
	caps := json.RawMessage(m.Capabilities)
	if len(caps) == 0 {
		caps = json.RawMessage("{}")
	}
	return modelCatalogDTO{
		ID: m.ID, Provider: m.Provider, Label: m.Label,
		InputPerM: m.InputPerM, OutputPerM: m.OutputPerM, Enabled: m.Enabled,
		Capabilities: caps, SortOrder: m.SortOrder, UpdatedAt: m.UpdatedAt.UTC().Format(timeRFC3339),
	}
}

func (s *Server) handleAdminListModels(w http.ResponseWriter, r *http.Request) {
	if s.deps.ModelCatalog == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "model catalog not configured")
		return
	}
	rows, err := s.deps.ModelCatalog.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list models")
		return
	}
	out := make([]modelCatalogDTO, len(rows))
	for i, m := range rows {
		out[i] = toModelCatalogDTO(m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": out})
}

func (s *Server) handleAdminUpsertModel(w http.ResponseWriter, r *http.Request) {
	if s.deps.ModelCatalog == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "model catalog not configured")
		return
	}
	var req struct {
		ID           string          `json:"id"`
		Provider     *string         `json:"provider"`
		Label        *string         `json:"label"`
		InputPerM    *float64        `json:"inputPerM"`
		OutputPerM   *float64        `json:"outputPerM"`
		Enabled      *bool           `json:"enabled"`
		Capabilities json.RawMessage `json:"capabilities"`
		SortOrder    *int            `json:"sortOrder"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	pathID := strings.TrimSpace(r.PathValue("id"))
	id := pathID
	if id == "" {
		id = strings.TrimSpace(req.ID)
	}
	if id == "" || id == chat.AutoModelID {
		writeError(w, http.StatusBadRequest, "validation_error", "id is required and cannot be auto")
		return
	}
	row := storage.ModelCatalogRow{ID: id, Enabled: true}
	if pathID != "" {
		existing, err := s.deps.ModelCatalog.Get(r.Context(), id)
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "model not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not load model")
			return
		}
		row = existing
	}
	if req.Provider != nil {
		row.Provider = *req.Provider
	}
	if req.Label != nil {
		row.Label = strings.TrimSpace(*req.Label)
	}
	if row.Provider == "" || !validProvider(row.Provider) {
		writeError(w, http.StatusBadRequest, "validation_error", "unknown provider")
		return
	}
	if row.Label == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "label is required")
		return
	}
	if req.InputPerM != nil {
		if *req.InputPerM < 0 {
			writeError(w, http.StatusBadRequest, "validation_error", "inputPerM must be non-negative")
			return
		}
		row.InputPerM = *req.InputPerM
	}
	if req.OutputPerM != nil {
		if *req.OutputPerM < 0 {
			writeError(w, http.StatusBadRequest, "validation_error", "outputPerM must be non-negative")
			return
		}
		row.OutputPerM = *req.OutputPerM
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	if len(req.Capabilities) > 0 {
		row.Capabilities = req.Capabilities
	}
	if req.SortOrder != nil {
		row.SortOrder = *req.SortOrder
	}
	saved, err := s.deps.ModelCatalog.Upsert(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not save model")
		return
	}
	s.reloadModelCatalog(r.Context())
	writeJSON(w, http.StatusOK, toModelCatalogDTO(saved))
}

func (s *Server) handleAdminDeleteModel(w http.ResponseWriter, r *http.Request) {
	if s.deps.ModelCatalog == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "model catalog not configured")
		return
	}
	if err := s.deps.ModelCatalog.Delete(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "model not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete model")
		return
	}
	s.reloadModelCatalog(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// reloadModelCatalog refreshes the in-process list from the DB. Cost rates
// snapshotted at boot keep their values until the next restart.
func (s *Server) reloadModelCatalog(ctx context.Context) {
	if s.deps.ModelCatalog == nil {
		return
	}
	rows, err := s.deps.ModelCatalog.List(ctx)
	if err != nil {
		s.deps.Log.Warn("could not reload model catalog", "err", err)
		return
	}
	specs := make([]chat.ModelSpec, 0, len(rows))
	for _, m := range rows {
		if !m.Enabled {
			continue
		}
		var caps map[string]bool
		if len(m.Capabilities) > 0 {
			_ = json.Unmarshal(m.Capabilities, &caps)
		}
		specs = append(specs, chat.ModelSpec{
			ID: m.ID, Provider: m.Provider, Label: m.Label,
			InputPerM: m.InputPerM, OutputPerM: m.OutputPerM,
			Capabilities: caps,
		})
	}
	chat.SetCatalog(specs)
}
