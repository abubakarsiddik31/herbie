package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/google/uuid"
)

type ProjectStore interface {
	Create(ctx context.Context, p storage.Project) (storage.Project, error)
	Get(ctx context.Context, id, userID string) (storage.Project, error)
	List(ctx context.Context, userID string) ([]storage.Project, error)
	Update(ctx context.Context, id, userID string, patch storage.ProjectPatch) (storage.Project, error)
	Delete(ctx context.Context, id, userID string) error
}

type projectDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	FilesCount   int    `json:"filesCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func toProjectDTO(p storage.Project) projectDTO {
	return projectDTO{
		ID:           p.ID,
		Name:         p.Name,
		Description:  p.Description,
		Instructions: p.Instructions,
		FilesCount:   p.FilesCount,
		CreatedAt:    p.CreatedAt.UTC().Format(timeRFC3339),
		UpdatedAt:    p.UpdatedAt.UTC().Format(timeRFC3339),
	}
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if s.deps.Projects == nil {
		writeJSON(w, http.StatusOK, map[string]any{"projects": []projectDTO{}})
		return
	}
	userID, _ := userIDFrom(r.Context())
	list, err := s.deps.Projects.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list projects")
		return
	}
	dtos := make([]projectDTO, len(list))
	for i, p := range list {
		dtos[i] = toProjectDTO(p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": dtos})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if s.deps.Projects == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "projects not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	var req struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	p, err := s.deps.Projects.Create(r.Context(), storage.Project{
		UserID:       userID,
		Name:         strings.TrimSpace(req.Name),
		Description:  strings.TrimSpace(req.Description),
		Instructions: strings.TrimSpace(req.Instructions),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create project")
		return
	}
	writeJSON(w, http.StatusCreated, toProjectDTO(p))
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	if s.deps.Projects == nil {
		writeError(w, http.StatusNotFound, "not_found", "project not found")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	p, err := s.deps.Projects.Get(r.Context(), id, userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not get project")
		return
	}

	var files []documentJSON
	if s.deps.Docs != nil {
		docs, err := s.deps.Docs.ListByProject(r.Context(), id, userID)
		if err == nil {
			files = make([]documentJSON, len(docs))
			for i, d := range docs {
				files[i] = documentToJSON(d)
			}
		}
	}

	var convs []conversationDTO
	if s.deps.Convos != nil {
		clist, err := s.deps.Convos.ListByProject(r.Context(), id, userID)
		if err == nil {
			convs = make([]conversationDTO, len(clist))
			for i, c := range clist {
				convs[i] = toConversationDTO(c)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"project":       toProjectDTO(p),
		"files":         files,
		"conversations": convs,
	})
}

func (s *Server) handlePatchProject(w http.ResponseWriter, r *http.Request) {
	if s.deps.Projects == nil {
		writeError(w, http.StatusNotFound, "not_found", "project not found")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	var req struct {
		Name         *string `json:"name"`
		Description  *string `json:"description"`
		Instructions *string `json:"instructions"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid body")
		return
	}
	p, err := s.deps.Projects.Update(r.Context(), id, userID, storage.ProjectPatch{
		Name:         req.Name,
		Description:  req.Description,
		Instructions: req.Instructions,
	})
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not update project")
		return
	}
	writeJSON(w, http.StatusOK, toProjectDTO(p))
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if s.deps.Projects == nil {
		writeError(w, http.StatusNotFound, "not_found", "project not found")
		return
	}
	userID, _ := userIDFrom(r.Context())
	id := r.PathValue("id")
	err := s.deps.Projects.Delete(r.Context(), id, userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not delete project")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUploadProjectFile(w http.ResponseWriter, r *http.Request) {
	if s.deps.Docs == nil || s.deps.RAG == nil {
		writeError(w, http.StatusServiceUnavailable, "rag_disabled", "RAG is not enabled")
		return
	}
	userID, _ := userIDFrom(r.Context())
	projectID := r.PathValue("id")

	if s.deps.Projects != nil {
		_, err := s.deps.Projects.Get(r.Context(), projectID, userID)
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "project not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not load project")
			return
		}
	}

	maxBytes := s.deps.Cfg.RAG.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = 20 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "file exceeds upload size limit")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "file field missing")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	mime, ok := allowedUploadTypes[ext]
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "allowed types: pdf, docx, xlsx, pptx, txt, md, csv, tsv, json, code files")
		return
	}
	content, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read file")
		return
	}

	docID := uuid.NewString()
	doc, err := s.deps.Docs.Create(r.Context(), storage.Document{
		ID:         docID,
		UserID:     userID,
		ProjectID:  &projectID,
		ObjectKey:  rag.ObjectKey(userID, docID, header.Filename),
		Filename:   header.Filename,
		Mime:       mime,
		SizeBytes:  int64(len(content)),
		Status:     "processing",
		ChunkCount: 0,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not record document")
		return
	}

	go func() {
		ctx := context.Background()
		chunks, rep, ingestErr := s.deps.RAG.Ingest(ctx, userID, docID, header.Filename, mime, content, mime)
		status := "ready"
		errMsg := ""
		if ingestErr != nil {
			status = "failed"
			errMsg = ingestErr.Error()
			s.deps.Log.Error("project doc ingestion failed", "id", docID, "err", ingestErr)
		}
		_ = s.deps.Docs.SetStatus(ctx, docID, userID, status, errMsg, chunks)
		if rep.InputTokens > 0 {
			model := s.deps.Cfg.RAG.EmbeddingModel
			_ = s.deps.Usage.Add(ctx, storage.UsageEvent{
				UserID:      userID,
				Kind:        "embedding",
				Model:       model,
				InputTokens: rep.InputTokens,
				Estimated:   rep.Estimated,
				CostMicros:  s.deps.Rates.RatesFor(model).ChatCostMicros(rep.InputTokens, 0),
			})
		}
	}()

	writeJSON(w, http.StatusAccepted, documentToJSON(doc))
}

func (s *Server) handleListProjectFiles(w http.ResponseWriter, r *http.Request) {
	if s.deps.Docs == nil {
		writeJSON(w, http.StatusOK, map[string]any{"files": []documentJSON{}})
		return
	}
	userID, _ := userIDFrom(r.Context())
	projectID := r.PathValue("id")
	docs, err := s.deps.Docs.ListByProject(r.Context(), projectID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list project files")
		return
	}
	out := make([]documentJSON, len(docs))
	for i, d := range docs {
		out[i] = documentToJSON(d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out})
}

func (s *Server) handleCreateProjectConversation(w http.ResponseWriter, r *http.Request) {
	if s.deps.Convos == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "conversations not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	projectID := r.PathValue("id")

	// Verify project exists
	p, err := s.deps.Projects.Get(r.Context(), projectID, userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load project")
		return
	}

	var req struct {
		Title string `json:"title"`
		Model string `json:"model"`
	}
	_ = decodeJSON(r, &req)

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Project chat"
	}

	ragOn := true
	var inst string
	if p.Instructions != "" {
		inst = "\nProject instructions: " + p.Instructions
	}
	sysPrompt := fmt.Sprintf("You are an assistant working on the project %q.%s\n\nAlways search the project's uploaded documents first using search_documents before searching the web or using external knowledge.", p.Name, inst)

	conv, err := s.deps.Convos.Create(r.Context(), userID, title, storage.ConversationPatch{
		ProjectID:    &projectID,
		Model:        &req.Model,
		SystemPrompt: &sysPrompt,
		RagEnabled:   &ragOn,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create conversation")
		return
	}
	writeJSON(w, http.StatusCreated, toConversationDTO(conv))
}

func (s *Server) handleListProjectConversations(w http.ResponseWriter, r *http.Request) {
	if s.deps.Convos == nil {
		writeJSON(w, http.StatusOK, map[string]any{"conversations": []conversationDTO{}})
		return
	}
	userID, _ := userIDFrom(r.Context())
	projectID := r.PathValue("id")
	clist, err := s.deps.Convos.ListByProject(r.Context(), projectID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list project conversations")
		return
	}
	out := make([]conversationDTO, len(clist))
	for i, c := range clist {
		out[i] = toConversationDTO(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}
