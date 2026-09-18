package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	svc := authtest.NewService("0123456789abcdef0123456789abcdef")
	tm, err := auth.NewTokenMaker("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewServer(ServerDeps{
		Cfg:      config.Config{FrontendOrigin: "http://localhost:5173"},
		Log:      log,
		Auth:     svc,
		Tokens:   tm,
		Profiles: newFakeProfiles(),
	})
}

func TestRegisterLoginMe(t *testing.T) {
	h := newTestServer(t)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSON(http.MethodPost, "/api/auth/register",
		map[string]string{"email": "a@b.co", "password": "longenough1"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	var reg struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &reg)

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+reg.AccessToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated me should 401, got %d", rec.Code)
	}
}

func TestRefreshSetsRotatedCookie(t *testing.T) {
	h := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSON(http.MethodPost, "/api/auth/register",
		map[string]string{"email": "a@b.co", "password": "longenough1"}))
	cookie := rec.Result().Cookies()[0]

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body.String())
	}
	if c := rec.Result().Cookies()[0]; c.Value == cookie.Value {
		t.Fatal("refresh cookie must rotate")
	}
}

func reqJSON(method, path string, body any) *http.Request {
	b, _ := json.Marshal(body)
	return httptest.NewRequest(method, path, bytes.NewReader(b))
}
