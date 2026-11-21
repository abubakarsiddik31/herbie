package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// FeedbackStore bridges thumbs ratings to storage.
type FeedbackStore interface {
	Set(ctx context.Context, userID, messageID, rating string) error
	Clear(ctx context.Context, userID, messageID string) error
	ForConversation(ctx context.Context, convID, userID string) (map[string]string, error)
}

var _ FeedbackStore = (*storage.Feedback)(nil)

// feedbackMessage loads the target message and enforces the write rules:
// it must belong to the caller and the path's conversation, and only
// assistant answers are ratable. Missing/foreign rows are 404 so feedback
// cannot probe another user's messages; user rows are 400.
func (s *Server) feedbackMessage(w http.ResponseWriter, r *http.Request) (storage.Message, bool) {
	userID, _ := userIDFrom(r.Context())
	msg, err := s.deps.Msgs.ByID(r.Context(), r.PathValue("messageId"), userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "message not found")
			return storage.Message{}, false
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load message")
		return storage.Message{}, false
	}
	if msg.ConversationID != r.PathValue("id") {
		writeError(w, http.StatusNotFound, "not_found", "message not found")
		return storage.Message{}, false
	}
	if msg.Role != string(model.RoleAssistant) {
		writeError(w, http.StatusBadRequest, "bad_request", "only assistant messages can be rated")
		return storage.Message{}, false
	}
	return msg, true
}

// handleSetFeedback upserts a thumbs rating ("up" or "down").
func (s *Server) handleSetFeedback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rating string `json:"rating"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	if req.Rating != "up" && req.Rating != "down" {
		writeError(w, http.StatusBadRequest, "validation_error", `rating must be "up" or "down"`)
		return
	}
	msg, ok := s.feedbackMessage(w, r)
	if !ok {
		return
	}
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.Feedback.Set(r.Context(), userID, msg.ID, req.Rating); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not save feedback")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleClearFeedback removes the caller's rating; ids still go through the
// same ownership and role checks so a foreign id never looks clearable.
func (s *Server) handleClearFeedback(w http.ResponseWriter, r *http.Request) {
	msg, ok := s.feedbackMessage(w, r)
	if !ok {
		return
	}
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.Feedback.Clear(r.Context(), userID, msg.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not clear feedback")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
