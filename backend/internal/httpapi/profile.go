package httpapi

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

const maxInstructionsChars = 4000

// generationInput is the shared wire shape for the provider-agnostic
// generation controls on profiles, projects, and bots.
type generationInput struct {
	TopP            *float64 `json:"topP"`
	ClearTopP       bool     `json:"clearTopP"`
	MaxOutputTokens *int     `json:"maxOutputTokens"`
	Reasoning       *string  `json:"reasoning"`
}

func validateGeneration(in generationInput) error {
	if in.TopP != nil && (*in.TopP < 0 || *in.TopP > 1) {
		return errors.New("topP must be between 0 and 1")
	}
	if in.MaxOutputTokens != nil && (*in.MaxOutputTokens < 0 || *in.MaxOutputTokens > 200000) {
		return errors.New("maxOutputTokens must be between 0 and 200000")
	}
	if in.Reasoning != nil {
		switch *in.Reasoning {
		case "", "low", "medium", "high":
		default:
			return errors.New("reasoning must be low, medium, or high")
		}
	}
	return nil
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	text, err := s.deps.Profiles.Instructions(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load profile")
		return
	}
	defaults, err := s.deps.Profiles.GenerationDefaults(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"defaultInstructions":    text,
		"defaultTopP":            defaults.TopP,
		"defaultMaxOutputTokens": defaults.MaxOutputTokens,
		"defaultReasoning":       defaults.Reasoning,
	})
}

func (s *Server) handlePatchProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		DefaultInstructions *string `json:"defaultInstructions"`
		generationInput
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	hasGeneration := req.TopP != nil || req.ClearTopP || req.MaxOutputTokens != nil || req.Reasoning != nil
	if req.DefaultInstructions == nil && !hasGeneration {
		writeError(w, http.StatusBadRequest, "bad_request", "nothing to update")
		return
	}
	if req.DefaultInstructions != nil {
		if utf8.RuneCountInString(*req.DefaultInstructions) > maxInstructionsChars {
			writeError(w, http.StatusBadRequest, "validation_error", "instructions too long")
			return
		}
	}
	if err := validateGeneration(req.generationInput); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if req.DefaultInstructions != nil {
		if err := s.deps.Profiles.SetInstructions(r.Context(), userID, *req.DefaultInstructions); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal", "could not save profile")
			return
		}
	}
	if hasGeneration {
		current, err := s.deps.Profiles.GenerationDefaults(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not load profile")
			return
		}
		if req.ClearTopP {
			current.TopP = nil
		} else if req.TopP != nil {
			current.TopP = req.TopP
		}
		if req.MaxOutputTokens != nil {
			current.MaxOutputTokens = *req.MaxOutputTokens
		}
		if req.Reasoning != nil {
			current.Reasoning = *req.Reasoning
		}
		if err := s.deps.Profiles.SetGenerationDefaults(r.Context(), userID, current); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal", "could not save profile")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
