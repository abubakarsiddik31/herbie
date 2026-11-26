package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// handleContinue resumes an interrupted answer: the partial assistant row is
// kept as history and the continuation lands beneath it as a new assistant
// row. The partial is never promoted to instructions.
func (s *Server) handleContinue(w http.ResponseWriter, r *http.Request) {
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
	if s.blockedByPendingApproval(w, ctx, convID, userID) {
		return
	}
	if blockedByBotConversation(w, conv) {
		return
	}
	release, ok := s.beginRun(convID)
	if !ok {
		s.blockedByActiveRun(w, convID)
		return
	}
	defer release()

	rows, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	leaf, err := s.resolveActiveLeaf(ctx, conv, rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	path, err := s.pathToLeaf(ctx, convID, userID, leaf)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load history")
		return
	}
	lastUser := -1
	for i, m := range path {
		if m.Role == string(model.RoleUser) {
			lastUser = i
		}
	}
	if lastUser < 0 || lastUser == len(path)-1 {
		writeError(w, http.StatusBadRequest, "nothing_to_continue", "there is no interrupted answer to continue")
		return
	}
	partial := path[len(path)-1]
	if partial.Role != string(model.RoleAssistant) || !partial.Truncated || strings.TrimSpace(partial.Content) == "" {
		writeError(w, http.StatusBadRequest, "nothing_to_continue", "the last answer is not an interrupted response")
		return
	}
	spec := s.runSpecFor(ctx, conv)
	s.unarchiveForTurn(ctx, conv)
	s.runTurn(ctx, userID, convID, projectScope(conv), spec, conv.BotID,
		historyFrom(path, len(path)), "Continue the interrupted answer from where it stopped. Output only the continuation.",
		nil, knownRows(path), partial.ID, w)
}
