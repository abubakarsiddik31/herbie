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
	var body struct {
		AccessToken string `json:"accessToken"`
		User        struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode refresh body: %v", err)
	}
	if body.AccessToken == "" || body.User.Email != "a@b.co" {
		t.Fatalf("expected accessToken and user, got %+v", body)
	}
	if c := rec.Result().Cookies()[0]; c.Value == cookie.Value {
		t.Fatal("refresh cookie must rotate")
	}
}

func TestCookieSecureFlag(t *testing.T) {
	svc := authtest.NewService("0123456789abcdef0123456789abcdef")
	tm, _ := auth.NewTokenMaker("0123456789abcdef0123456789abcdef")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Insecure server (http origin, no forwarded proto)
	insecureServer := NewServer(ServerDeps{
		Cfg:      config.Config{FrontendOrigin: "http://localhost:5173"},
		Log:      log,
		Auth:     svc,
		Tokens:   tm,
		Profiles: newFakeProfiles(),
	})
	rec := httptest.NewRecorder()
	insecureServer.ServeHTTP(rec, reqJSON(http.MethodPost, "/api/auth/register",
		map[string]string{"email": "insecure@b.co", "password": "longenough1"}))
	if len(rec.Result().Cookies()) == 0 || rec.Result().Cookies()[0].Secure {
		t.Fatal("expected non-secure cookie on plain http")
	}

	// Secure server (https origin or X-Forwarded-Proto)
	secureServer := NewServer(ServerDeps{
		Cfg:      config.Config{FrontendOrigin: "https://app.example.com"},
		Log:      log,
		Auth:     svc,
		Tokens:   tm,
		Profiles: newFakeProfiles(),
	})
	recSecure := httptest.NewRecorder()
	req := reqJSON(http.MethodPost, "/api/auth/register",
		map[string]string{"email": "secure@b.co", "password": "longenough1"})
	req.Header.Set("X-Forwarded-Proto", "https")
	secureServer.ServeHTTP(recSecure, req)
	if len(recSecure.Result().Cookies()) == 0 || !recSecure.Result().Cookies()[0].Secure {
		t.Fatal("expected secure cookie on https")
	}
}

func reqJSON(method, path string, body any) *http.Request {
	b, _ := json.Marshal(body)
	return httptest.NewRequest(method, path, bytes.NewReader(b))
}
