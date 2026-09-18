package httpapi

import (
	"errors"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// handleShareConversation mints the conversation's public link. The raw
// token is shown once (201); resharing an already-shared conversation
// answers 200 without it — unrecoverable by design, revoke and re-share
// to rotate.
func (s *Server) handleShareConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	token, hash, err := auth.NewRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not share conversation")
		return
	}
	_, created, err := s.deps.Shares.Share(r.Context(), r.PathValue("id"), userID, hash)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not share conversation")
		return
	}
	if created {
		writeJSON(w, http.StatusCreated, map[string]string{"token": token})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"shared": true})
}

func (s *Server) handleUnshareConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.Shares.Unshare(r.Context(), r.PathValue("id"), userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation is not shared")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not revoke share")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleShareState(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	hash, err := s.deps.Shares.SharedHash(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load share state")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"shared": hash != ""})
}

// handleGetShared serves a revoked-or-live public thread. No auth, no
// usage/cost, no settings — title and transcript only.
func (s *Server) handleGetShared(w http.ResponseWriter, r *http.Request) {
	convID, userID, err := s.deps.Shares.Resolve(r.Context(), auth.HashRefreshToken(r.PathValue("token")))
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "shared conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load shared conversation")
		return
	}
	conv, err := s.deps.Convos.ByID(r.Context(), convID, userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "shared conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load shared conversation")
		return
	}
	msgs, err := s.deps.Msgs.ForConversation(r.Context(), conv.ID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"title":    conv.Title,
		"messages": transcriptMessages(msgs, false),
	})
}
