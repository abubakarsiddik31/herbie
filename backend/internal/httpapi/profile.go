package httpapi

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

const maxInstructionsChars = 4000

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	text, err := s.deps.Profiles.Instructions(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"defaultInstructions": text})
}

func (s *Server) handlePatchProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		DefaultInstructions *string `json:"defaultInstructions"`
	}
	if err := decodeJSON(r, &req); err != nil || req.DefaultInstructions == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "defaultInstructions is required")
		return
	}
	if utf8.RuneCountInString(*req.DefaultInstructions) > maxInstructionsChars {
		writeError(w, http.StatusBadRequest, "validation_error", "instructions too long")
		return
	}
	if err := s.deps.Profiles.SetInstructions(r.Context(), userID, *req.DefaultInstructions); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not save profile")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
