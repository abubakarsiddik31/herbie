package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// handleEditMessage rewrites a user message and re-runs the conversation
// from that point: everything after the edited row is deleted (truncate &
// resend) and a fresh turn streams with the new content. Images already on
// the row are kept — the edit endpoint only takes text.
func (s *Server) handleEditMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	msgID := r.PathValue("messageId")
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Content == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "content is required")
		return
	}
	if len([]rune(req.Content)) > maxPromptChars {
		writeError(w, http.StatusBadRequest, "bad_request", "message too long")
		return
	}

	ctx := r.Context()
	conv, err := s.deps.Convos.ByID(ctx, convID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
		return
	}
	spec := s.runSpecFor(conv)
	if s.blockedByPendingApproval(w, ctx, convID, userID) {
		return
	}

	target, err := s.deps.Msgs.ByID(ctx, msgID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "message not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load message")
		return
	}
	if target.ConversationID != convID {
		writeError(w, http.StatusNotFound, "not_found", "message not found")
		return
	}
	if target.Role != string(model.RoleUser) {
		writeError(w, http.StatusBadRequest, "bad_request", "only user messages can be edited")
		return
	}

	// Rewrite the text while preserving any image parts in the payload.
	var payload model.Message
	if err := json.Unmarshal(target.Data, &payload); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not decode message")
		return
	}
	payload.Content = req.Content
	if err := s.deps.Msgs.UpdateContent(ctx, msgID, userID, req.Content, mustJSON(payload)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not update message")
		return
	}
	if _, err := s.deps.Msgs.DeleteAfter(ctx, convID, userID, target.CreatedAt, msgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not truncate history")
		return
	}

	msgs, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	idx := -1
	for i, m := range msgs {
		if m.ID == msgID {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeError(w, http.StatusInternalServerError, "internal", "could not reload message")
		return
	}
	_ = s.deps.Convos.Touch(ctx, convID)
	s.runTurn(ctx, userID, convID, spec, historyFrom(msgs, idx), req.Content, payload.Parts, knownRows(msgs), w)
}

// handleRegenerate replaces the last assistant answer: rows after the last
// user message are deleted and that turn re-runs with its original prompt
// and images.
func (s *Server) handleRegenerate(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")

	ctx := r.Context()
	conv, err := s.deps.Convos.ByID(ctx, convID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
		return
	}
	spec := s.runSpecFor(conv)
	if s.blockedByPendingApproval(w, ctx, convID, userID) {
		return
	}

	msgs, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	lastUser := -1
	for i, m := range msgs {
		if m.Role == string(model.RoleUser) {
			lastUser = i
		}
	}
	if lastUser < 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "nothing to regenerate")
		return
	}
	var payload model.Message
	if err := json.Unmarshal(msgs[lastUser].Data, &payload); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not decode message")
		return
	}
	if _, err := s.deps.Msgs.DeleteAfter(ctx, convID, userID, msgs[lastUser].CreatedAt, msgs[lastUser].ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not truncate history")
		return
	}

	// The prompt row stays, so the re-run's `known` set covers the rows that
	// remain in the store (including it) and skips them on persist.
	remaining, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	_ = s.deps.Convos.Touch(ctx, convID)
	s.runTurn(ctx, userID, convID, spec, historyFrom(remaining, lastUser), msgs[lastUser].Content, payload.Parts, knownRows(remaining), w)
}

// handleDeleteMessage removes one message and everything after it —
// deleting a question drops the answer that followed, deleting an answer
// leaves prior context intact.
func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.Msgs.DeleteMessage(r.Context(), r.PathValue("id"), r.PathValue("messageId"), userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "message not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete message")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
