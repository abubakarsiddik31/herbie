package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// handleContinue resumes an interrupted answer: the partial assistant text is
// folded into the run instructions and a new assistant branch is created
// under the same user row, leaving the partial off-path.
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
	var partial strings.Builder
	for _, m := range path[lastUser+1:] {
		if m.Role == string(model.RoleAssistant) && strings.TrimSpace(m.Content) != "" {
			partial.WriteString(m.Content)
		}
	}
	if partial.Len() == 0 {
		writeError(w, http.StatusBadRequest, "nothing_to_continue", "there is no interrupted answer to continue")
		return
	}
	spec := s.runSpecFor(ctx, conv)
	spec.SystemPrompt += "\n\n[Interrupted answer to continue]\n" + partial.String() +
		"\n\nContinue from the exact stopping point. Output only the continuation; do not repeat the text above or add a preamble."
	s.unarchiveForTurn(ctx, conv)
	s.runTurn(ctx, userID, convID, projectScope(conv), spec, conv.BotID,
		historyFrom(path, lastUser+1), "Continue the interrupted answer from where it stopped.",
		nil, knownRows(path), path[lastUser].ID, w)
}
