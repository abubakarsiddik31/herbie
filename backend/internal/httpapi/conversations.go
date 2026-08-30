package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type conversationDTO struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toConversationDTO(c storage.Conversation) conversationDTO {
	return conversationDTO{ID: c.ID, Title: c.Title, CreatedAt: c.CreatedAt.UTC().Format(timeRFC3339), UpdatedAt: c.UpdatedAt.UTC().Format(timeRFC3339)}
}

const timeRFC3339 = "2006-01-02T15:04:05.000Z07:00"

// convoCRUD is the management capability the CRUD routes need beyond the
// chat-facing ConvoStore, which stays minimal so handler tests run offline.
// Production wiring always supplies *storage.Conversations, which satisfies
// both; fakes that only implement ConvoStore degrade to a 500 here.
type convoCRUD interface {
	Create(ctx context.Context, userID, title string) (storage.Conversation, error)
	List(ctx context.Context, userID string) ([]storage.Conversation, error)
	Delete(ctx context.Context, id, userID string) error
}

func (s *Server) crud() (convoCRUD, bool) {
	c, ok := s.deps.Convos.(convoCRUD)
	return c, ok
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	crud, ok := s.crud()
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "conversation management unavailable")
		return
	}
	convs, err := crud.List(r.Context(), userID)
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
		Title string `json:"title"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req) // optional body
	}
	crud, ok := s.crud()
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "conversation management unavailable")
		return
	}
	conv, err := crud.Create(r.Context(), userID, req.Title)
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
	type msgDTO struct {
		ID        string `json:"id"`
		Role      string `json:"role"`
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
		CreatedAt string `json:"createdAt"`
	}
	out := make([]msgDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, msgDTO{ID: m.ID, Role: m.Role, Content: m.Content, Truncated: m.Truncated, CreatedAt: m.CreatedAt.UTC().Format(timeRFC3339)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation": toConversationDTO(conv), "messages": out})
}

func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Title string `json:"title"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Title == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "title is required")
		return
	}
	if err := s.deps.Convos.SetTitle(r.Context(), r.PathValue("id"), userID, req.Title); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not rename")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	crud, ok := s.crud()
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "conversation management unavailable")
		return
	}
	if err := crud.Delete(r.Context(), r.PathValue("id"), userID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
