package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/oauth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
)

const oauthTestSecret = "0123456789abcdef0123456789abcdef"

func stubOAuthIdP(verified bool) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "idp-token"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub": "idp-sub", "email": "social@example.com", "email_verified": verified,
		})
	})
	return httptest.NewServer(mux)
}

func newOAuthTestServer(t *testing.T, idp *httptest.Server) http.Handler {
	t.Helper()
	provider := &oauth.Provider{
		ID: "test", Name: "Test", ClientID: "cid", Secret: "sec",
		Redirect: "https://app.example/callback",
		Scopes:   []string{"openid"},
		Endpoints: oauth.Endpoints{
			AuthURL:    idp.URL + "/auth",
			TokenURL:   idp.URL + "/token",
			ProfileURL: idp.URL + "/userinfo",
		},
		HTTPClient: idp.Client(),
	}
	return NewServer(ServerDeps{
		Cfg:      config.Config{FrontendOrigin: "https://app.example", JWTSecret: oauthTestSecret},
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:     authtest.NewService(oauthTestSecret),
		Tokens:   mustTokenMaker(t),
		Profiles: newFakeProfiles(),
		OAuth:    []*oauth.Provider{provider},
	})
}

func mustTokenMaker(t *testing.T) *auth.TokenMaker {
	t.Helper()
	tm, err := auth.NewTokenMaker(oauthTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

func TestOAuthProvidersAndStart(t *testing.T) {
	idp := stubOAuthIdP(true)
	defer idp.Close()
	h := newOAuthTestServer(t, idp)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/providers", nil))
	var listed struct {
		Providers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"providers"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if len(listed.Providers) != 1 || listed.Providers[0].ID != "test" {
		t.Fatalf("providers: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/oauth/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown provider: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/oauth/test", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("start: %d", rec.Code)
	}
	loc := rec.Result().Header.Get("Location")
	if !strings.HasPrefix(loc, idp.URL+"/auth?") || !strings.Contains(loc, "code_challenge=") {
		t.Fatalf("start location: %s", loc)
	}
	var sealed *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "oauth_state" {
			sealed = c
		}
	}
	if sealed == nil || !sealed.HttpOnly || sealed.MaxAge <= 0 {
		t.Fatalf("state cookie: %+v", sealed)
	}
	if _, _, ok := oauth.Open([]byte(oauthTestSecret), sealed.Value, time.Now()); !ok {
		t.Fatal("state cookie does not verify")
	}
}

func TestOAuthCallbackFullLoop(t *testing.T) {
	idp := stubOAuthIdP(true)
	defer idp.Close()
	h := newOAuthTestServer(t, idp)

	sealed := oauth.Seal([]byte(oauthTestSecret), "s", "verifier", time.Now().Add(time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/test/callback?code=c&state=s", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: sealed})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("callback: %d %s", rec.Code, rec.Body.String())
	}
	loc := rec.Result().Header.Get("Location")
	code := strings.TrimPrefix(loc, "https://app.example/auth/callback?code=")
	if loc == code || code == "" || strings.Contains(code, "&") {
		t.Fatalf("callback location: %s", loc)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSON(http.MethodPost, "/api/auth/oauth/consume", map[string]string{"code": code}))
	if rec.Code != http.StatusOK {
		t.Fatalf("consume: %d %s", rec.Code, rec.Body.String())
	}
	var res struct {
		AccessToken string `json:"accessToken"`
		User        struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.AccessToken == "" || res.User.Email != "social@example.com" {
		t.Fatalf("consume body: %s", rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 || rec.Result().Cookies()[0].Name != "refresh_token" {
		t.Fatal("consume must set the refresh cookie")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqJSON(http.MethodPost, "/api/auth/oauth/consume", map[string]string{"code": code}))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("replay should 401, got %d", rec.Code)
	}
}

func TestOAuthCallbackRejects(t *testing.T) {
	idp := stubOAuthIdP(true)
	defer idp.Close()
	h := newOAuthTestServer(t, idp)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/oauth/test/callback?code=c&state=s", nil))
	if loc := rec.Result().Header.Get("Location"); loc != "https://app.example/auth/callback?error=invalid_state" {
		t.Fatalf("missing state: %s", loc)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/oauth/test/callback?error=access_denied", nil))
	if loc := rec.Result().Header.Get("Location"); loc != "https://app.example/auth/callback?error=denied" {
		t.Fatalf("provider error: %s", loc)
	}

	unver := stubOAuthIdP(false)
	defer unver.Close()
	h2 := newOAuthTestServer(t, unver)
	sealed := oauth.Seal([]byte(oauthTestSecret), "s", "verifier", time.Now().Add(time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth/test/callback?code=c&state=s", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: sealed})
	rec = httptest.NewRecorder()
	h2.ServeHTTP(rec, req)
	if loc := rec.Result().Header.Get("Location"); loc != "https://app.example/auth/callback?error=unverified_email" {
		t.Fatalf("unverified: %s", loc)
	}
}
