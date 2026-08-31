package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// ToolStore is the persistence the tools CRUD routes need. *storage.Tools
// satisfies it; tests use fakes.
type ToolStore interface {
	Create(ctx context.Context, t storage.UserTool) (storage.UserTool, error)
	List(ctx context.Context, userID string) ([]storage.UserTool, error)
	ByID(ctx context.Context, id, userID string) (storage.UserTool, error)
	Update(ctx context.Context, t storage.UserTool) (storage.UserTool, error)
	Delete(ctx context.Context, id, userID string) error
	Count(ctx context.Context, userID string) (int, error)
}

var _ ToolStore = (*storage.Tools)(nil)

// headerMask replaces every header value in API responses. A masked value
// sent back on PATCH preserves the stored secret.
const headerMask = "••••"

type toolDTO struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Method          string            `json:"method"`
	URLTemplate     string            `json:"urlTemplate"`
	Params          json.RawMessage   `json:"params"`
	BodyTemplate    string            `json:"bodyTemplate"`
	Headers         map[string]string `json:"headers"`
	RequireApproval bool              `json:"requireApproval"`
	Enabled         bool              `json:"enabled"`
	CreatedAt       string            `json:"createdAt"`
	UpdatedAt       string            `json:"updatedAt"`
}

func toToolDTO(t storage.UserTool) toolDTO {
	headers := map[string]string{}
	_ = json.Unmarshal(t.Headers, &headers)
	masked := make(map[string]string, len(headers))
	for k := range headers {
		masked[k] = headerMask
	}
	return toolDTO{
		ID: t.ID, Name: t.Name, Description: t.Description, Method: t.Method,
		URLTemplate: t.URLTemplate, Params: t.Params, BodyTemplate: t.BodyTemplate,
		Headers:         masked,
		RequireApproval: t.RequireApproval, Enabled: t.Enabled,
		CreatedAt: t.CreatedAt.UTC().Format(timeRFC3339),
		UpdatedAt: t.UpdatedAt.UTC().Format(timeRFC3339),
	}
}

type createToolRequest struct {
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Method          string            `json:"method"`
	URLTemplate     string            `json:"urlTemplate"`
	Params          []chat.ParamDef   `json:"params"`
	BodyTemplate    string            `json:"bodyTemplate"`
	Headers         map[string]string `json:"headers"`
	RequireApproval bool              `json:"requireApproval"`
	Enabled         *bool             `json:"enabled"`
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	rows, err := s.deps.Tools.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list tools")
		return
	}
	out := make([]toolDTO, 0, len(rows))
	for _, t := range rows {
		out = append(out, toToolDTO(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateTool(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req createToolRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	cfg := req.toConfig()
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if _, err := cfg.Schema(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "parameters could not form a schema")
		return
	}
	limit := s.deps.Cfg.MaxToolsPerUser
	if limit <= 0 {
		limit = 20 // config default; guards zero-valued test configs
	}
	if n, err := s.deps.Tools.Count(r.Context(), userID); err == nil && n >= limit {
		writeError(w, http.StatusBadRequest, "limit_reached", "tool limit reached")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	row, err := s.deps.Tools.Create(r.Context(), userToolFromConfig(cfg, userID, enabled))
	if err != nil {
		if errors.Is(err, storage.ErrDuplicate) {
			writeError(w, http.StatusConflict, "duplicate_name", "a tool with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not create tool")
		return
	}
	writeJSON(w, http.StatusCreated, toToolDTO(row))
}

type patchToolRequest struct {
	Name            *string             `json:"name"`
	Description     *string             `json:"description"`
	Method          *string             `json:"method"`
	URLTemplate     *string             `json:"urlTemplate"`
	Params          *[]chat.ParamDef    `json:"params"`
	BodyTemplate    *string             `json:"bodyTemplate"`
	Headers         *map[string]*string `json:"headers"`
	RequireApproval *bool               `json:"requireApproval"`
	Enabled         *bool               `json:"enabled"`
}

func (s *Server) handlePatchTool(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	row, err := s.deps.Tools.ByID(r.Context(), id, userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "tool not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load tool")
		return
	}
	cfg, err := toolConfigFromUserTool(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "tool config unreadable")
		return
	}
	var req patchToolRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	if req.Name != nil {
		cfg.Name = *req.Name
	}
	if req.Description != nil {
		cfg.Description = *req.Description
	}
	if req.Method != nil {
		cfg.Method = *req.Method
	}
	if req.URLTemplate != nil {
		cfg.URLTemplate = *req.URLTemplate
	}
	if req.BodyTemplate != nil {
		cfg.BodyTemplate = *req.BodyTemplate
	}
	if req.Params != nil {
		cfg.Params = *req.Params
	}
	if req.RequireApproval != nil {
		cfg.RequireApproval = *req.RequireApproval
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	headers := cfg.Headers
	if req.Headers != nil {
		if headers == nil {
			headers = map[string]string{}
		}
		for k, v := range *req.Headers {
			switch {
			case v == nil:
				delete(headers, k)
			case *v == headerMask:
				// keep the stored secret for this key
			default:
				headers[k] = *v
			}
		}
	}
	row.Name, row.Description, row.Method, row.URLTemplate = cfg.Name, cfg.Description, cfg.Method, cfg.URLTemplate
	row.Params, row.BodyTemplate = mustMarshal(cfg.Params), cfg.BodyTemplate
	row.Headers = mustMarshal(headers)
	row.RequireApproval = cfg.RequireApproval
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	updated, err := s.deps.Tools.Update(r.Context(), row)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "tool not found")
			return
		}
		if errors.Is(err, storage.ErrDuplicate) {
			writeError(w, http.StatusConflict, "duplicate_name", "a tool with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not update tool")
		return
	}
	writeJSON(w, http.StatusOK, toToolDTO(updated))
}

func (s *Server) handleDeleteTool(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	err := s.deps.Tools.Delete(r.Context(), r.PathValue("id"), userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "tool not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not delete tool")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c createToolRequest) toConfig() chat.ToolConfig {
	headers := c.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	return chat.ToolConfig{
		Name: c.Name, Description: c.Description, Method: c.Method,
		URLTemplate: c.URLTemplate, Params: c.Params, BodyTemplate: c.BodyTemplate,
		Headers: headers, RequireApproval: c.RequireApproval,
	}
}

func toolConfigFromUserTool(t storage.UserTool) (chat.ToolConfig, error) {
	cfg := chat.ToolConfig{
		Name: t.Name, Description: t.Description, Method: t.Method,
		URLTemplate: t.URLTemplate, BodyTemplate: t.BodyTemplate,
		RequireApproval: t.RequireApproval,
	}
	if err := json.Unmarshal(t.Params, &cfg.Params); err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(t.Headers, &cfg.Headers); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func userToolFromConfig(cfg chat.ToolConfig, userID string, enabled bool) storage.UserTool {
	return storage.UserTool{
		UserID: userID, Name: cfg.Name, Description: cfg.Description,
		Method: cfg.Method, URLTemplate: cfg.URLTemplate,
		Params: mustMarshal(cfg.Params), BodyTemplate: cfg.BodyTemplate,
		Headers: mustMarshal(cfg.Headers), RequireApproval: cfg.RequireApproval,
		Enabled: enabled,
	}
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}
