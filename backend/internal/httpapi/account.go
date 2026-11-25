package httpapi

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/rag"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

// AccountStore owns the account export and deletion transactions.
type AccountStore interface {
	WriteExport(ctx context.Context, userID string, w io.Writer) ([]storage.ExportFile, error)
	Delete(ctx context.Context, userID, password string) error
}

// safeEntryName normalizes a user-controlled filename into one ZIP path
// segment. Backslashes count as separators on Windows and are not handled by
// path.Base on Unix.
func safeEntryName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		name = "document"
	}
	if runes := []rune(name); len(runes) > 200 {
		name = string(runes[:200])
	}
	return name
}

// handleExportAccount streams a ZIP to a temp file first so a database or
// object-store failure is a clean 500 instead of a truncated download.
func (s *Server) handleExportAccount(w http.ResponseWriter, r *http.Request) {
	if s.deps.Account == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "account export not configured")
		return
	}
	userID, _ := userIDFrom(r.Context())
	tmp, err := os.CreateTemp("", "herbie-export-*.zip")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not prepare export")
		return
	}
	name := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(name)
	}()

	zw := zip.NewWriter(tmp)
	entry, err := zw.Create("export.json")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not prepare export")
		return
	}
	files, err := s.deps.Account.WriteExport(r.Context(), userID, entry)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not build export")
		return
	}
	index, err := zw.Create("documents/index.json")
	if err == nil {
		_ = json.NewEncoder(index).Encode(map[string]any{"files": files})
	}
	var skipped []string
	for _, f := range files {
		if s.deps.Objects == nil || f.ObjectKey == "" {
			skipped = append(skipped, f.Filename)
			continue
		}
		rc, err := s.deps.Objects.Get(r.Context(), f.ObjectKey)
		if err != nil || rc == nil {
			skipped = append(skipped, f.Filename)
			continue
		}
		entry, err := zw.CreateHeader(&zip.FileHeader{
			Name:     "documents/" + f.DocumentID + "/" + safeEntryName(f.Filename),
			Method:   zip.Deflate,
			Modified: f.CreatedAt,
		})
		if err == nil {
			_, err = io.Copy(entry, rc)
		}
		rc.Close()
		if err != nil {
			// A partial entry would corrupt the archive; fail before headers.
			writeError(w, http.StatusInternalServerError, "internal", "could not read document")
			return
		}
	}
	if manifest, err := zw.Create("export-manifest.json"); err == nil {
		_ = json.NewEncoder(manifest).Encode(map[string]any{
			"generatedAt":  time.Now().UTC().Format(time.RFC3339),
			"skippedFiles": skipped,
		})
	}
	if err := zw.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not finish export")
		return
	}
	info, err := tmp.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not finish export")
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read export")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="herbie-export.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	if _, err := io.Copy(w, tmp); err != nil && s.deps.Log != nil {
		s.deps.Log.Warn("export stream interrupted", "err", err)
	}
}

// handleDeleteAccount verifies the password, deletes the account atomically,
// then removes external RAG state (objects + vectors) that Postgres cannot
// cascade.
func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if s.deps.Account == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "account deletion not configured")
		return
	}
	var req struct {
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Confirm != "DELETE" {
		writeError(w, http.StatusBadRequest, "confirm_required", `type "DELETE" to confirm`)
		return
	}
	if !s.allowAuth(w, r, "delete", "") {
		return
	}
	userID, _ := userIDFrom(r.Context())

	var docs []storage.Document
	if s.deps.Docs != nil {
		docs, _ = s.deps.Docs.List(r.Context(), userID)
	}
	err := s.deps.Account.Delete(r.Context(), userID, req.Password)
	switch {
	case errors.Is(err, auth.ErrInvalidPassword):
		writeError(w, http.StatusForbidden, "invalid_password", "password is incorrect")
		return
	case errors.Is(err, auth.ErrLastAdmin):
		writeError(w, http.StatusConflict, "last_admin", "the last admin cannot delete their account")
		return
	case errors.Is(err, storage.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "could not delete account")
		return
	}
	for _, doc := range docs {
		if doc.ObjectKey != "" && s.deps.Objects != nil {
			_ = s.deps.Objects.Delete(r.Context(), doc.ObjectKey)
			_ = s.deps.Objects.Delete(r.Context(), string(rag.ParsedObjectKey(doc.ObjectKey)))
		}
		if s.deps.Vectors != nil {
			_ = s.deps.Vectors.DeleteDocument(r.Context(), doc.ID)
		}
	}
	s.clearRefreshCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}
