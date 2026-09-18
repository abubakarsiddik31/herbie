package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
)

func TestHealthz(t *testing.T) {
	h := NewServer(ServerDeps{Cfg: config.Config{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("healthz: %d %q", rec.Code, rec.Body.String())
	}
}

func TestStatusResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusResponseWriter{ResponseWriter: rec, status: http.StatusOK}
	sw.WriteHeader(http.StatusNotFound)
	_, _ = sw.Write([]byte("not found"))
	sw.Flush()

	if sw.status != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, sw.status)
	}
	if sw.Unwrap() != rec {
		t.Fatalf("unwrap mismatch")
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("recorder code = %d", rec.Code)
	}
}
