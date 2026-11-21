package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

type conversationDTO struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Model        string   `json:"model"`
	Temperature  *float64 `json:"temperature"`
	SystemPrompt string   `json:"systemPrompt"`
	RagEnabled   bool     `json:"ragEnabled"`
	// BotID is set only for bot-scoped conversations (always-on teammates).
	BotID *string `json:"botId,omitempty"`
	// ActiveLeafID is the visible message-tree leaf; null for empty chats.
	ActiveLeafID *string `json:"activeLeafId"`
	// PinnedAt/ArchivedAt are nullable organization timestamps.
	PinnedAt   *string `json:"pinnedAt"`
	ArchivedAt *string `json:"archivedAt"`
	// TitleSource records the title's provenance ("heuristic" or "manual").
	TitleSource string `json:"titleSource"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func rfc3339Ptr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	v := t.UTC().Format(timeRFC3339)
	return &v
}

func toConversationDTO(c storage.Conversation) conversationDTO {
	return conversationDTO{
		ID: c.ID, Title: c.Title, Model: c.Model, Temperature: c.Temperature, SystemPrompt: c.SystemPrompt,
		RagEnabled: c.RagEnabled, BotID: c.BotID, ActiveLeafID: c.ActiveLeafID,
		PinnedAt: rfc3339Ptr(c.PinnedAt), ArchivedAt: rfc3339Ptr(c.ArchivedAt), TitleSource: c.TitleSource,
		CreatedAt: c.CreatedAt.UTC().Format(timeRFC3339), UpdatedAt: c.UpdatedAt.UTC().Format(timeRFC3339),
	}
}

const timeRFC3339 = "2006-01-02T15:04:05.000Z07:00"

const maxSystemPromptChars = 4000

// settingsPatch converts an incoming partial settings body into a storage
// patch, validating the values against the server's model catalog. A nil
// modelID leaves the stored model unchanged; an empty (but present) one
// resets to the server default.
func (s *Server) settingsPatch(modelID *string, temperature *float64, clearTemperature bool, systemPrompt *string, ragEnabled *bool) (storage.ConversationPatch, error) {
	patch := storage.ConversationPatch{Temperature: temperature, ClearTemperature: clearTemperature, SystemPrompt: systemPrompt, RagEnabled: ragEnabled}
	if modelID != nil {
		if *modelID != "" {
			spec, ok := chat.FindModel(*modelID)
			if !ok {
				return patch, errors.New("unknown model " + *modelID)
			}
			if !chat.HasProvider(s.deps.ModelKeys, spec.Provider) {
				return patch, errors.New("model " + *modelID + " is not configured on this server")
			}
		}
		patch.Model = modelID
	}
	if temperature != nil && (*temperature < 0 || *temperature > 2) {
		return patch, errors.New("temperature must be between 0 and 2")
	}
	if systemPrompt != nil && utf8.RuneCountInString(*systemPrompt) > maxSystemPromptChars {
		return patch, errors.New("system prompt too long")
	}
	return patch, nil
}

// queryBool parses an optional strict boolean query parameter: "true"/"1"
// and "false"/"0" only, an empty value meaning false.
func queryBool(r *http.Request, key string) (bool, error) {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	switch v {
	case "":
		return false, nil
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", key)
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	pinned, err := queryBool(r, "pinned")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	archived, err := queryBool(r, "archived")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	convs, err := s.deps.Convos.List(r.Context(), userID, strings.TrimSpace(r.URL.Query().Get("q")), pinned, archived)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list conversations")
		return
	}
	if convs == nil {
		convs = []storage.Conversation{}
	}
	out := make([]conversationDTO, len(convs))
	for i, c := range convs {
		out[i] = toConversationDTO(c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Title            string   `json:"title"`
		Model            string   `json:"model"`
		Temperature      *float64 `json:"temperature"`
		SystemPrompt     string   `json:"systemPrompt"`
		RagEnabled       *bool    `json:"ragEnabled"`
		ClearTemperature bool     `json:"-"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req) // optional body
	}
	patch, err := s.settingsPatch(&req.Model, req.Temperature, req.ClearTemperature, &req.SystemPrompt, req.RagEnabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	conv, err := s.deps.Convos.Create(r.Context(), userID, req.Title, patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create conversation")
		return
	}
	writeJSON(w, http.StatusCreated, toConversationDTO(conv))
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	conv, err := s.deps.Convos.ByID(r.Context(), r.PathValue("id"), userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "conversation not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
		return
	}
	msgs, err := s.deps.Msgs.ForConversation(r.Context(), conv.ID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	leaf, err := s.resolveActiveLeaf(r.Context(), conv, msgs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	path, err := s.pathToLeaf(r.Context(), conv.ID, userID, leaf)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load messages")
		return
	}
	compThreshold := 40000
	compKeepRecent := 10
	if s.deps.Compactor != nil {
		compThreshold = s.deps.Compactor.Threshold()
		compKeepRecent = s.deps.Compactor.KeepRecent()
	} else if s.deps.Cfg.RAG.CompactionThreshold > 0 {
		compThreshold = s.deps.Cfg.RAG.CompactionThreshold
		compKeepRecent = s.deps.Cfg.RAG.CompactionKeepRecent
	}
	estimatedTokens := chat.EstimateStorageTokens(path)
	hasCompacted := false
	for _, m := range path {
		if strings.Contains(m.Content, "[Compacted earlier context") || strings.Contains(m.Content, "[input auto-compacted") {
			hasCompacted = true
			break
		}
	}
	if leaf != "" {
		conv.ActiveLeafID = &leaf
	}
	var feedback map[string]string
	if s.deps.Feedback != nil {
		feedback, _ = s.deps.Feedback.ForConversation(r.Context(), conv.ID, userID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation": toConversationDTO(conv),
		"messages":     transcriptMessages(msgs, leaf, true, feedback),
		"context": map[string]any{
			"estimatedTokens": estimatedTokens,
			"thresholdTokens": compThreshold,
			"keepRecent":      compKeepRecent,
			"compacted":       hasCompacted,
		},
	})
}

type usageDTO struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	CostUsd      float64 `json:"costUsd"`
	Model        string  `json:"model"`
}

type msgDTO struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Truncated bool             `json:"truncated"`
	CreatedAt string           `json:"createdAt"`
	Usage     *usageDTO        `json:"usage,omitempty"`
	Images    []imageDTO       `json:"images,omitempty"`
	Sources   *json.RawMessage `json:"sources,omitempty"`
	Branch    *branchDTO       `json:"branch,omitempty"`
	// ReplyToID/QuoteText carry the user's reply context; the snapshot is
	// server-stored. Feedback is private (the owner's thumbs rating) and is
	// never rendered in shared transcripts.
	ReplyToID string `json:"replyToId,omitempty"`
	QuoteText string `json:"quoteText,omitempty"`
	Feedback  string `json:"feedback,omitempty"`
}

// branchDTO describes a variant switcher for a user row or a whole assistant
// chain. index is the row's 0-based position in ids (ordered by created_at, id).
type branchDTO struct {
	Count int      `json:"count"`
	Index int      `json:"index"`
	IDs   []string `json:"ids"`
}

var webSearchResultRe = regexp.MustCompile(`\[(\d+)\]\s*([^\n]+)\nURL:\s*([^\n]+)`)

func extractWebSourcesFromToolMsgs(toolMsgs []string) []map[string]any {
	var rows []map[string]any
	for _, content := range toolMsgs {
		if !strings.Contains(content, "Web search results:") {
			continue
		}
		matches := webSearchResultRe.FindAllStringSubmatchIndex(content, -1)
		for i, match := range matches {
			title := strings.TrimSpace(content[match[4]:match[5]])
			rawURL := strings.TrimSpace(content[match[6]:match[7]])
			snippetStart := match[1]
			snippetEnd := len(content)
			if i+1 < len(matches) {
				snippetEnd = matches[i+1][0]
			}
			snippet := strings.TrimSpace(content[snippetStart:snippetEnd])
			if len(snippet) > 160 {
				snippet = strings.TrimSpace(snippet[:160]) + "…"
			}
			rows = append(rows, map[string]any{
				"documentId": rawURL,
				"title":      title,
				"heading":    rawURL,
				"page":       0,
				"snippet":    snippet,
				"score":      1.0,
				"url":        rawURL,
			})
		}
	}
	return rows
}

// transcriptMessages renders the active path through stored rows for
// display, skipping tool plumbing (tool-result rows and empty assistant
// tool-call rows are history replay data only). Branch metadata rides user
// variants (sibling user rows) and assistant variants per chain: the roots
// are the direct assistant children of the nearest ancestor user row (often
// hidden tool-call rows), and the switcher attaches to the LAST visible
// assistant row of each chain segment on the path. privateView enables the
// owner-only fields — usage/cost and thumbs feedback — so shared threads
// never leak spend or ratings. Quotes stay visible: they are part of the
// conversation.
func transcriptMessages(msgs []storage.Message, leafID string, privateView bool, feedback map[string]string) []msgDTO {
	if leafID == "" {
		return []msgDTO{}
	}
	byID := make(map[string]storage.Message, len(msgs))
	for _, m := range msgs {
		byID[m.ID] = m
	}
	// Walk parent links to get the visible path in root→leaf order.
	var reversed []storage.Message
	seen := map[string]bool{}
	for id := leafID; id != "" && !seen[id]; {
		m, ok := byID[id]
		if !ok {
			break
		}
		seen[id] = true
		reversed = append(reversed, m)
		id = m.ParentID
	}
	path := make([]storage.Message, len(reversed))
	for i := range reversed {
		path[len(reversed)-1-i] = reversed[i]
	}

	assistantChildren := make(map[string][]storage.Message)
	for _, m := range msgs {
		if m.Role == string(model.RoleAssistant) {
			assistantChildren[m.ParentID] = append(assistantChildren[m.ParentID], m)
		}
	}
	sortRows := func(rows []storage.Message) {
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
				return rows[i].ID < rows[j].ID
			}
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		})
	}
	for parent := range assistantChildren {
		sortRows(assistantChildren[parent])
	}

	branches := make(map[int]*branchDTO)
	// User variants: sibling user rows sharing the same parent.
	for i, m := range path {
		if m.Role != string(model.RoleUser) {
			continue
		}
		var siblings []storage.Message
		for _, other := range msgs {
			if other.Role == string(model.RoleUser) && other.ParentID == m.ParentID {
				siblings = append(siblings, other)
			}
		}
		if len(siblings) > 1 {
			sortRows(siblings)
			branches[i] = branchMeta(siblings, m.ID)
		}
	}
	// Assistant variants: per chain, attached to the last visible assistant
	// row of that chain on the path.
	lastVisible := make(map[string]int) // chain root ID → path index
	for i, m := range path {
		if m.Role != string(model.RoleAssistant) || m.Content == "" {
			continue
		}
		root := m
		for root.ParentID != "" {
			parent, ok := byID[root.ParentID]
			if !ok || parent.Role == string(model.RoleUser) {
				break
			}
			root = parent
		}
		lastVisible[root.ID] = i
	}
	for rootID, i := range lastVisible {
		root, ok := byID[rootID]
		if !ok || root.ParentID == "" {
			continue
		}
		parent, ok := byID[root.ParentID]
		if !ok || parent.Role != string(model.RoleUser) {
			continue
		}
		roots := assistantChildren[parent.ID]
		if len(roots) > 1 {
			branches[i] = branchMeta(roots, rootID)
		}
	}

	out := make([]msgDTO, 0, len(path))
	var pendingToolContents []string
	for i, m := range path {
		if m.Role == string(model.RoleTool) {
			pendingToolContents = append(pendingToolContents, m.Content)
			continue
		}
		if m.Content == "" && !userHasImages(m.Data) {
			continue
		}
		var usage *usageDTO
		if privateView && (m.InputTokens > 0 || m.OutputTokens > 0) {
			usage = &usageDTO{
				InputTokens:  m.InputTokens,
				OutputTokens: m.OutputTokens,
				CostUsd:      math.Round(float64(m.CostMicros)/10) / 1e5,
				Model:        m.Model,
			}
		}
		sources := storedSources(m.Sources)
		if sources == nil && m.Role == string(model.RoleAssistant) && len(pendingToolContents) > 0 {
			if extracted := extractWebSourcesFromToolMsgs(pendingToolContents); len(extracted) > 0 {
				if b, err := json.Marshal(extracted); err == nil {
					raw := json.RawMessage(b)
					sources = &raw
				}
			}
		}
		if m.Role == string(model.RoleAssistant) || m.Role == string(model.RoleUser) {
			pendingToolContents = nil
		}
		row := msgDTO{
			ID: m.ID, Role: m.Role, Content: m.Content, Truncated: m.Truncated,
			CreatedAt: m.CreatedAt.UTC().Format(timeRFC3339), Usage: usage,
			Images: imagesOf(m.Data), Sources: sources, Branch: branches[i],
			ReplyToID: m.ReplyToID, QuoteText: m.QuoteText,
		}
		if privateView {
			row.Feedback = feedback[m.ID]
		}
		out = append(out, row)
	}
	return out
}

// branchMeta builds the switcher payload for rows ordered by (created_at, id).
func branchMeta(rows []storage.Message, currentID string) *branchDTO {
	ids := make([]string, len(rows))
	index := -1
	for i, m := range rows {
		ids[i] = m.ID
		if m.ID == currentID {
			index = i
		}
	}
	if index < 0 {
		return nil
	}
	return &branchDTO{Count: len(ids), Index: index, IDs: ids}
}

// storedSources returns the persisted citation rows for a history message,
// nil when the run never searched (empty arrays stay out of the payload).
func storedSources(raw []byte) *json.RawMessage {
	if len(raw) == 0 || string(raw) == "[]" || string(raw) == "null" {
		return nil
	}
	var v []map[string]any
	if json.Unmarshal(raw, &v) != nil || len(v) == 0 {
		return nil
	}
	out := json.RawMessage(raw)
	return &out
}

// imageDTO is one stored image attachment, served as a data URL.
type imageDTO struct {
	MediaType string `json:"mediaType"`
	DataURL   string `json:"dataUrl"`
}

// imagesOf extracts an image listing from a stored message payload; empty
// for text-only rows.
func imagesOf(data []byte) []imageDTO {
	var payload model.Message
	if json.Unmarshal(data, &payload) != nil || len(payload.Parts) == 0 {
		return nil
	}
	images := make([]imageDTO, 0, len(payload.Parts))
	for _, p := range payload.Parts {
		images = append(images, imageDTO{
			MediaType: p.MediaType,
			DataURL:   "data:" + p.MediaType + ";base64," + base64.StdEncoding.EncodeToString(p.Data),
		})
	}
	return images
}

func userHasImages(data []byte) bool {
	var payload model.Message
	return json.Unmarshal(data, &payload) == nil && len(payload.Parts) > 0
}

func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Title            *string  `json:"title"`
		Model            *string  `json:"model"`
		Temperature      *float64 `json:"temperature"`
		SystemPrompt     *string  `json:"systemPrompt"`
		RagEnabled       *bool    `json:"ragEnabled"`
		ClearTemperature bool     `json:"clearTemperature"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	if req.Title != nil && *req.Title == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "title cannot be empty")
		return
	}
	patch, err := s.settingsPatch(req.Model, req.Temperature, req.ClearTemperature, req.SystemPrompt, req.RagEnabled)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if req.Title != nil {
		if err := s.deps.Convos.RenameTitle(r.Context(), r.PathValue("id"), userID, *req.Title); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "conversation not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal", "could not rename")
			return
		}
	}
	if err := s.deps.Convos.SetSettings(r.Context(), r.PathValue("id"), userID, patch); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not update settings")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePinConversation flips pinned_at; pinning is allowed during a run
// because it never touches the message tree.
func (s *Server) handlePinConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Pinned *bool `json:"pinned"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Pinned == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "pinned is required")
		return
	}
	if err := s.deps.Convos.SetPinned(r.Context(), r.PathValue("id"), userID, *req.Pinned); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not update pin")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleArchiveConversation flips archived_at. Archiving is refused while a
// run (including a parked approval) owns the conversation; unarchiving is
// always allowed.
func (s *Server) handleArchiveConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Archived *bool `json:"archived"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Archived == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "archived is required")
		return
	}
	id := r.PathValue("id")
	// Ownership first: a foreign/unknown conversation is 404 even when a run
	// happens to be active.
	if _, err := s.deps.Convos.ByID(r.Context(), id, userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not load conversation")
		return
	}
	if *req.Archived {
		if s.runActive(id) {
			writeError(w, http.StatusConflict, "run_active", "a run is active on this conversation")
			return
		}
		// A paused approval holds no run slot; archiving would hide the
		// conversation and resume writes would land in an archived chat.
		if s.deps.Pending != nil {
			pending, err := s.deps.Pending.ForConversation(r.Context(), id, userID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "could not check approvals")
				return
			}
			if len(pending) > 0 {
				writeError(w, http.StatusConflict, "approval_pending", "resolve the pending approval first")
				return
			}
		}
	}
	if err := s.deps.Convos.SetArchived(r.Context(), id, userID, *req.Archived); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not update archive")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.Convos.Delete(r.Context(), r.PathValue("id"), userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
