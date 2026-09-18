package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type MemoryStore interface {
	List(ctx context.Context, userID string) ([]storage.Memory, error)
	Create(ctx context.Context, userID, content string) (storage.Memory, error)
	Delete(ctx context.Context, userID, memoryID string) error
	DeleteAll(ctx context.Context, userID string) error
}

var _ MemoryStore = (*storage.Memories)(nil)

const maxMemoryChars = 1000

func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if s.deps.Memories == nil {
		writeJSON(w, http.StatusOK, []storage.Memory{})
		return
	}
	memories, err := s.deps.Memories.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load memories")
		return
	}
	if memories == nil {
		memories = []storage.Memory{}
	}
	writeJSON(w, http.StatusOK, memories)
}

func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if s.deps.Memories == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "memory storage disabled")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Content) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "content is required")
		return
	}
	content := strings.TrimSpace(req.Content)
	if utf8.RuneCountInString(content) > maxMemoryChars {
		writeError(w, http.StatusBadRequest, "validation_error", "memory too long")
		return
	}
	mem, err := s.deps.Memories.Create(r.Context(), userID, content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create memory")
		return
	}
	writeJSON(w, http.StatusCreated, mem)
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if s.deps.Memories == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "memory storage disabled")
		return
	}
	memoryID := r.PathValue("id")
	if memoryID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "memory id required")
		return
	}
	if err := s.deps.Memories.Delete(r.Context(), userID, memoryID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "memory not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete memory")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleClearMemories(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if s.deps.Memories == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "memory storage disabled")
		return
	}
	if err := s.deps.Memories.DeleteAll(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not clear memories")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
