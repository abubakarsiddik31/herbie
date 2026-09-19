package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// DocStore is the documents-table slice the handlers need; *storage.Documents
// satisfies it.
type DocStore interface {
	Create(ctx context.Context, d storage.Document) (storage.Document, error)
	Get(ctx context.Context, id, userID string) (storage.Document, error)
	List(ctx context.Context, userID string) ([]storage.Document, error)
	ListByProject(ctx context.Context, projectID, userID string) ([]storage.Document, error)
	SetStatus(ctx context.Context, id, userID, status, errMsg string, chunkCount int) error
	Delete(ctx context.Context, id, userID string) error
}

// RagRunner is the ingestion entry point; *rag.Service satisfies it.
// Nil ServerDeps fields of this shape mean RAG is disabled.
type RagRunner interface {
	Ingest(ctx context.Context, userID, documentID, docTitle, mime string, content []byte, contentType string) (int, rag.EmbedUsage, error)
}

// allowedUploadTypes maps the filename extension to the extraction mime.
var allowedUploadTypes = map[string]string{
	".txt":      "text/plain",
	".md":       "text/markdown",
	".markdown": "text/markdown",
	".pdf":      "application/pdf",
	".docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx":     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx":     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".csv":      "text/csv",
	".tsv":      "text/tab-separated-values",
	".json":     "application/json",
	".yaml":     "application/x-yaml",
	".yml":      "application/x-yaml",
	".xml":      "application/xml",
	".html":     "text/html",
	".py":       "text/x-python",
	".js":       "text/javascript",
	".ts":       "text/typescript",
	".tsx":      "text/typescript-jsx",
	".jsx":      "text/javascript-jsx",
	".go":       "text/x-go",
	".rs":       "text/x-rust",
	".sh":       "text/x-shellscript",
	".sql":      "text/x-sql",
}

type documentJSON struct {
	ID         string `json:"id"`
	Filename   string `json:"filename"`
	Mime       string `json:"mime"`
	SizeBytes  int64  `json:"sizeBytes"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	ChunkCount int    `json:"chunkCount"`
	CreatedAt  string `json:"createdAt"`
}

func documentToJSON(d storage.Document) documentJSON {
	return documentJSON{
		ID: d.ID, Filename: d.Filename, Mime: d.Mime, SizeBytes: d.SizeBytes,
		Status: d.Status, Error: d.Error, ChunkCount: d.ChunkCount, CreatedAt: d.CreatedAt.UTC().Format(timeRFC3339),
	}
}

// ragDisabled reports whether the retrieval stack (and with it the
// documents API) is switched off.
func (s *Server) ragDisabled() bool { return s.deps.RAG == nil || s.deps.Docs == nil }

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	if s.ragDisabled() {
		writeError(w, http.StatusServiceUnavailable, "rag_disabled", "document upload requires RAG_ENABLED=true and the rag compose profile")
		return
	}
	userID, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "multipart form required")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", `multipart field "file" required`)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	mime, ok := allowedUploadTypes[ext]
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "allowed types: pdf, docx, xlsx, pptx, txt, md, csv, tsv, json, code files")
		return
	}

	max := s.deps.Cfg.RAG.MaxUploadBytes
	if max <= 0 {
		max = 20 << 20
	}
	content, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "could not read upload")
		return
	}
	if int64(len(content)) > max {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "upload exceeds the size cap")
		return
	}

	// The id is generated here so the object key can carry it; Ingest
	// derives the identical key from the same inputs.
	docID := uuid.NewString()
	objectKey := rag.ObjectKey(userID, docID, header.Filename)
	doc, err := s.deps.Docs.Create(r.Context(), storage.Document{
		ID: docID, UserID: userID, ObjectKey: objectKey,
		Filename: header.Filename, Mime: mime, SizeBytes: int64(len(content)),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store document")
		return
	}
	// Row created (status=processing), now run the pipeline; a failure
	// marks the row failed and keeps object and row so the upload is
	// diagnosable and re-ingestion stays possible.
	chunks, embedUsage, ingestErr := s.deps.RAG.Ingest(r.Context(), userID, docID, header.Filename, mime, content, mime)
	if ingestErr == nil && embedUsage.InputTokens > 0 {
		model := s.deps.Cfg.RAG.EmbeddingModel
		docIDRef := docID
		_ = s.deps.Usage.Add(r.Context(), storage.UsageEvent{
			UserID: userID, Kind: "embedding", Model: model,
			DocumentID:  &docIDRef,
			InputTokens: embedUsage.InputTokens,
			Estimated:   embedUsage.Estimated,
			CostMicros:  s.deps.Rates.RatesFor(model).ChatCostMicros(embedUsage.InputTokens, 0),
		})
	}
	if ingestErr != nil {
		s.deps.Log.Error("ingest failed", "document", docID, "err", ingestErr)
		_ = s.deps.Docs.SetStatus(r.Context(), docID, userID, "failed", ingestErr.Error(), 0)
		doc, _ = s.deps.Docs.Get(r.Context(), docID, userID)
		writeJSON(w, http.StatusOK, documentToJSON(doc))
		return
	}
	if err := s.deps.Docs.SetStatus(r.Context(), docID, userID, "ready", "", chunks); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not finalize document")
		return
	}
	doc, _ = s.deps.Docs.Get(r.Context(), docID, userID)
	writeJSON(w, http.StatusCreated, documentToJSON(doc))
}

func (s *Server) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	if s.ragDisabled() {
		writeError(w, http.StatusServiceUnavailable, "rag_disabled", "documents require RAG_ENABLED=true and the rag compose profile")
		return
	}
	userID, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return
	}
	docs, err := s.deps.Docs.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list documents")
		return
	}
	out := make([]documentJSON, 0, len(docs))
	for _, d := range docs {
		out = append(out, documentToJSON(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": out})
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	if s.ragDisabled() {
		writeError(w, http.StatusServiceUnavailable, "rag_disabled", "documents require RAG_ENABLED=true and the rag compose profile")
		return
	}
	userID, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return
	}
	docID := r.PathValue("id")
	doc, err := s.deps.Docs.Get(r.Context(), docID, userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such document")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load document")
		return
	}
	// Best-effort cleanup of the derived data; the row is removed either
	// way so the document disappears from the UI.
	if s.deps.Vectors != nil {
		if err := s.deps.Vectors.DeleteDocument(r.Context(), docID); err != nil {
			s.deps.Log.Error("weaviate delete failed", "document", docID, "err", err)
		}
	}
	if s.deps.Objects != nil {
		if err := s.deps.Objects.Delete(r.Context(), doc.ObjectKey); err != nil {
			s.deps.Log.Error("object delete failed", "key", doc.ObjectKey, "err", err)
		}
		_ = s.deps.Objects.Delete(r.Context(), rag.ParsedObjectKey(userID, docID))
	}
	if err := s.deps.Docs.Delete(r.Context(), docID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not delete document")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetDocumentContent(w http.ResponseWriter, r *http.Request) {
	if s.ragDisabled() {
		writeError(w, http.StatusServiceUnavailable, "rag_disabled", "documents require RAG_ENABLED=true and the rag compose profile")
		return
	}
	userID, ok := userIDFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return
	}
	docID := r.PathValue("id")
	doc, err := s.deps.Docs.Get(r.Context(), docID, userID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load document")
		return
	}

	var text string
	if s.deps.Objects != nil {
		// 1. Check MinIO bucket for cached parsed markdown
		parsedKey := rag.ParsedObjectKey(userID, docID)
		if obj, err := s.deps.Objects.Get(r.Context(), parsedKey); err == nil && obj != nil {
			b, readErr := io.ReadAll(obj)
			_ = obj.Close()
			if readErr == nil && len(b) > 0 {
				text = string(b)
			}
		}

		// 2. If not yet in MinIO, extract from original file and save parsed.md to MinIO
		if text == "" && doc.ObjectKey != "" {
			obj, err := s.deps.Objects.Get(r.Context(), doc.ObjectKey)
			if err == nil && obj != nil {
				defer obj.Close()
				extracted, err := rag.ExtractTextWithFilename(doc.Mime, doc.Filename, obj)
				if err == nil && extracted != "" {
					text = extracted
					_ = s.deps.Objects.Put(r.Context(), parsedKey, "text/markdown", strings.NewReader(text), int64(len(text)))
				}
			}
		}
	}

	if text == "" && s.deps.Vectors != nil && doc.ChunkCount > 0 {
		chunks, err := s.deps.Vectors.ExpandRange(r.Context(), userID, docID, 0, doc.ChunkCount)
		if err == nil && len(chunks) > 0 {
			sort.Slice(chunks, func(i, j int) bool {
				return chunks[i].Index < chunks[j].Index
			})
			var sb strings.Builder
			for i, c := range chunks {
				if i > 0 {
					sb.WriteString("\n\n")
				}
				sb.WriteString(c.Content)
			}
			text = sb.String()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         doc.ID,
		"filename":   doc.Filename,
		"mime":       doc.Mime,
		"sizeBytes":  doc.SizeBytes,
		"status":     doc.Status,
		"error":      doc.Error,
		"chunkCount": doc.ChunkCount,
		"createdAt":  doc.CreatedAt.UTC().Format(timeRFC3339),
		"text":       text,
	})
}

func (s *Server) handleExtractText(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "file too large or invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "missing file")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	mime := allowedUploadTypes[ext]
	if mime == "" {
		mime = "text/plain"
	}
	text, err := rag.ExtractTextWithFilename(mime, header.Filename, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "extract_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"filename": header.Filename,
		"text":     text,
		"size":     header.Size,
	})
}
