package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
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
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

func toConversationDTO(c storage.Conversation) conversationDTO {
	return conversationDTO{
		ID: c.ID, Title: c.Title, Model: c.Model, Temperature: c.Temperature, SystemPrompt: c.SystemPrompt,
		RagEnabled: c.RagEnabled,
		CreatedAt:  c.CreatedAt.UTC().Format(timeRFC3339), UpdatedAt: c.UpdatedAt.UTC().Format(timeRFC3339),
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

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	convs, err := s.deps.Convos.List(r.Context(), userID)
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
	}
	out := make([]msgDTO, 0, len(msgs))
	for _, m := range msgs {
		// Tool plumbing stays out of the transcript: tool-result rows and
		// empty assistant tool-call rows are history replay data only.
		if m.Role == string(model.RoleTool) || (m.Content == "" && !userHasImages(m.Data)) {
			continue
		}
		var usage *usageDTO
		if m.InputTokens > 0 || m.OutputTokens > 0 {
			usage = &usageDTO{
				InputTokens:  m.InputTokens,
				OutputTokens: m.OutputTokens,
				CostUsd:      math.Round(float64(m.CostMicros)/10) / 1e5,
				Model:        m.Model,
			}
		}
		out = append(out, msgDTO{
			ID: m.ID, Role: m.Role, Content: m.Content, Truncated: m.Truncated,
			CreatedAt: m.CreatedAt.UTC().Format(timeRFC3339), Usage: usage,
			Images: imagesOf(m.Data), Sources: storedSources(m.Sources),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": toConversationDTO(conv), "messages": out})
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
		if err := s.deps.Convos.SetTitle(r.Context(), r.PathValue("id"), userID, *req.Title); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not rename")
			return
		}
	}
	if err := s.deps.Convos.SetSettings(r.Context(), r.PathValue("id"), userID, patch); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not update settings")
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
