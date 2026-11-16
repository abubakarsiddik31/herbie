package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

const (
	maxPromptTitleRunes = 200
	maxPromptBodyRunes  = 8000
	maxSavedPrompts     = 100
)

type savedPromptDTO struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toSavedPromptDTO(p storage.SavedPrompt) savedPromptDTO {
	return savedPromptDTO{
		ID: p.ID, Title: p.Title, Body: p.Body,
		CreatedAt: p.CreatedAt.UTC().Format(timeRFC3339),
		UpdatedAt: p.UpdatedAt.UTC().Format(timeRFC3339),
	}
}

// validatePromptText checks a present title/body against the library limits.
func validatePromptText(w http.ResponseWriter, title, body *string) bool {
	if title != nil {
		n := utf8.RuneCountInString(*title)
		if strings.TrimSpace(*title) == "" || n > maxPromptTitleRunes {
			writeError(w, http.StatusBadRequest, "validation_error", "title must be 1-200 characters")
			return false
		}
	}
	if body != nil {
		n := utf8.RuneCountInString(*body)
		if strings.TrimSpace(*body) == "" || n > maxPromptBodyRunes {
			writeError(w, http.StatusBadRequest, "validation_error", "body must be 1-8000 characters")
			return false
		}
	}
	return true
}

func (s *Server) handleListSavedPrompts(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	prompts, err := s.deps.Prompts.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list prompts")
		return
	}
	if prompts == nil {
		prompts = []storage.SavedPrompt{}
	}
	out := make([]savedPromptDTO, len(prompts))
	for i, p := range prompts {
		out[i] = toSavedPromptDTO(p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompts": out})
}

func (s *Server) handleCreateSavedPrompt(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	if !validatePromptText(w, &req.Title, &req.Body) {
		return
	}
	n, err := s.deps.Prompts.Count(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create prompt")
		return
	}
	if n >= maxSavedPrompts {
		writeError(w, http.StatusConflict, "prompt_limit", "prompt limit reached")
		return
	}
	p, err := s.deps.Prompts.Create(r.Context(), userID, req.Title, req.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create prompt")
		return
	}
	writeJSON(w, http.StatusCreated, toSavedPromptDTO(p))
}

func (s *Server) handlePatchSavedPrompt(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Title *string `json:"title"`
		Body  *string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	if !validatePromptText(w, req.Title, req.Body) {
		return
	}
	p, err := s.deps.Prompts.Update(r.Context(), r.PathValue("id"), userID, req.Title, req.Body)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "prompt not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not update prompt")
		return
	}
	writeJSON(w, http.StatusOK, toSavedPromptDTO(p))
}

func (s *Server) handleDeleteSavedPrompt(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.Prompts.Delete(r.Context(), r.PathValue("id"), userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "prompt not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete prompt")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
