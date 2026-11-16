package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// handleEditMessage forks a user message instead of rewriting history: the
// edited text becomes a sibling user row under the target's parent, the new
// branch becomes the active leaf, and a fresh turn streams from it. The old
// question and answer stay intact off the active path. Images already on the
// target are copied to the sibling — the edit endpoint only takes text.
func (s *Server) handleEditMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	msgID := r.PathValue("messageId")
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}
	req.Content = storage.SanitizeText(req.Content)
	if strings.TrimSpace(req.Content) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "content is required")
		return
	}
	if runes := []rune(req.Content); len(runes) > maxPromptChars {
		req.Content = string(runes[:maxPromptChars]) + "\n\n[input auto-compacted: truncated at 1,000,000 characters]"
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
	spec := s.runSpecFor(ctx, conv)
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
	s.unarchiveForTurn(ctx, conv)

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
	idx := rowOnPath(path, target.ID)
	if idx < 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "only messages on the active branch can be edited")
		return
	}

	// Copy the stored payload (images included) but replace the text.
	var payload model.Message
	if err := json.Unmarshal(target.Data, &payload); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not decode message")
		return
	}
	payload.Content = req.Content
	siblingID, err := s.deps.Msgs.Add(ctx, storage.Message{
		ConversationID: convID, UserID: userID, Role: string(model.RoleUser),
		ParentID: target.ParentID,
		Content:  req.Content,
		Data:     mustJSON(payload),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store message")
		return
	}
	if err := s.deps.Convos.SetActiveLeaf(ctx, convID, userID, siblingID); err != nil {
		s.deps.Log.Error("could not set active leaf", "err", err, "conv_id", convID)
	}
	_ = s.deps.Convos.Touch(ctx, convID)
	s.runTurn(ctx, userID, convID, spec, conv.BotID, historyFrom(path, idx), req.Content, payload.Parts, knownRows(path), siblingID, w)
}

// handleRegenerate creates a sibling assistant chain for the last user row on
// the active path. The previous answer stays off-path; the new chain becomes
// the active leaf.
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
	spec := s.runSpecFor(ctx, conv)
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
	s.unarchiveForTurn(ctx, conv)

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
	if lastUser < 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "nothing to regenerate")
		return
	}
	var payload model.Message
	if err := json.Unmarshal(path[lastUser].Data, &payload); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not decode message")
		return
	}
	_ = s.deps.Convos.Touch(ctx, convID)
	s.runTurn(ctx, userID, convID, spec, conv.BotID, historyFrom(path, lastUser), path[lastUser].Content, payload.Parts, knownRows(path), path[lastUser].ID, w)
}

// handleDeleteMessage deletes one message's whole branch (subtree). Deleting
// an active subtree moves the leaf to the deleted root's parent; deleting an
// inactive branch leaves the active path untouched. Bot conversations may
// prune branches; the runner appends to the active leaf.
func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	ctx := r.Context()
	if s.blockedByPendingApproval(w, ctx, convID, userID) {
		return
	}
	release, ok := s.beginRun(convID)
	if !ok {
		s.blockedByActiveRun(w, convID)
		return
	}
	defer release()
	if err := s.deps.Msgs.DeleteBranch(ctx, convID, r.PathValue("messageId"), userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "message not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete message")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleActivateMessage switches the conversation to a variant branch: it
// resolves the clicked row to its variant root (a user row, or the direct
// child of the nearest ancestor user row for assistant/tool rows) and selects
// the deepest leaf of that root's subtree, deterministically breaking ties by
// (created_at, id) descending.
func (s *Server) handleActivateMessage(w http.ResponseWriter, r *http.Request) {
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
	target, err := s.deps.Msgs.ByID(ctx, r.PathValue("messageId"), userID)
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
	rows, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	if _, err := s.resolveActiveLeaf(ctx, conv, rows); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	rootID := variantRootID(rows, target)
	leaf := deepestLeafID(rows, rootID)
	if leaf == "" {
		leaf = rootID
	}
	if err := s.deps.Convos.SetActiveLeaf(ctx, convID, userID, leaf); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not activate branch")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// variantRootID resolves the branch switcher's unit: user rows are their own
// root; assistant/tool rows walk up to the direct child of the nearest
// ancestor user row (often the hidden assistant tool-call row).
func variantRootID(rows []storage.Message, target storage.Message) string {
	if target.Role == string(model.RoleUser) {
		return target.ID
	}
	byID := make(map[string]storage.Message, len(rows))
	for _, m := range rows {
		byID[m.ID] = m
	}
	cur := target
	for cur.ParentID != "" {
		parent, ok := byID[cur.ParentID]
		if !ok || parent.Role == string(model.RoleUser) {
			break
		}
		cur = parent
	}
	return cur.ID
}

// deepestLeafID returns the deepest descendant of rootID, breaking equal
// depths by (created_at, id) descending. The root itself wins when childless.
func deepestLeafID(rows []storage.Message, rootID string) string {
	children := make(map[string][]storage.Message, len(rows))
	for _, m := range rows {
		if m.ParentID != "" {
			children[m.ParentID] = append(children[m.ParentID], m)
		}
	}
	type node struct {
		row   storage.Message
		depth int
	}
	stack := []node{{row: storage.Message{ID: rootID}, depth: 0}}
	for _, m := range rows {
		if m.ID == rootID {
			stack[0].row = m
			break
		}
	}
	var best node
	found := false
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		kids := children[n.row.ID]
		if len(kids) == 0 {
			if !found || n.depth > best.depth ||
				(n.depth == best.depth && (n.row.CreatedAt.After(best.row.CreatedAt) ||
					(n.row.CreatedAt.Equal(best.row.CreatedAt) && n.row.ID > best.row.ID))) {
				best, found = n, true
			}
			continue
		}
		for _, kid := range kids {
			stack = append(stack, node{row: kid, depth: n.depth + 1})
		}
	}
	if !found {
		return ""
	}
	return best.row.ID
}

// handleForkConversation copies the visible path (or a prefix up to
// messageId) into a brand-new conversation. Settings are copied; bot scoping
// never is. The copy is display-only: no ledger events, no usage writes.
func (s *Server) handleForkConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convID := r.PathValue("id")
	var req struct {
		MessageID string `json:"messageId"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req) // optional body
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
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	leaf, err := s.resolveActiveLeaf(ctx, conv, rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	targetID := req.MessageID
	if targetID == "" {
		targetID = leaf
	}
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "conversation is empty")
		return
	}
	known := false
	for _, m := range rows {
		if m.ID == targetID {
			known = true
			break
		}
	}
	if !known {
		writeError(w, http.StatusNotFound, "not_found", "message not found")
		return
	}
	path, err := s.pathToLeaf(ctx, convID, userID, targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	manualTitle := "manual"
	convPatch := storage.ConversationPatch{
		Model:        &conv.Model,
		Temperature:  conv.Temperature,
		SystemPrompt: &conv.SystemPrompt,
		RagEnabled:   &conv.RagEnabled,
		ProjectID:    conv.ProjectID,
		TitleSource:  &manualTitle,
	}
	forked, err := s.deps.Convos.Create(ctx, userID, "Fork of "+conv.Title, convPatch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create fork")
		return
	}
	tail, err := s.deps.Msgs.CopyPath(ctx, convID, userID, forked.ID, path)
	if err != nil {
		_ = s.deps.Convos.Delete(ctx, forked.ID, userID)
		writeError(w, http.StatusInternalServerError, "internal", "could not copy conversation")
		return
	}
	if tail != "" {
		if err := s.deps.Convos.SetActiveLeaf(ctx, forked.ID, userID, tail); err != nil {
			s.deps.Log.Error("could not set fork leaf", "err", err, "conv_id", forked.ID)
		}
		if conv, err = s.deps.Convos.ByID(ctx, forked.ID, userID); err == nil {
			forked = conv
		}
	}
	writeJSON(w, http.StatusCreated, toConversationDTO(forked))
}
