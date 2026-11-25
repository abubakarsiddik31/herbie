package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
)

type fakeMail struct {
	To      string
	Subject string
	Body    string
}

type fakeMailer struct {
	mu   sync.Mutex
	msgs []fakeMail
}

func (f *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, fakeMail{To: to, Subject: subject, Body: body})
	return nil
}

func (f *fakeMailer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

func (f *fakeMailer) last() fakeMail {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.msgs) == 0 {
		return fakeMail{}
	}
	return f.msgs[len(f.msgs)-1]
}

func newLifecycleServer(t *testing.T, rateMax int) (*auth.Service, *fakeMailer, http.Handler) {
	t.Helper()
	users := authtest.NewFakeUsers()
	refresh := authtest.NewFakeRefreshStore()
	tokens := authtest.NewFakeAuthTokens(users)
	tokens.BindRefresh(refresh)
	svc := authtest.NewServiceWithStores(testSecret, users, refresh, tokens)
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	mail := &fakeMailer{}
	h := NewServer(ServerDeps{
		Cfg: config.Config{
			FrontendOrigin:      "http://localhost:5173",
			AppURL:              "http://localhost:5173",
			AuthRateLimitMax:    rateMax,
			AuthRateLimitWindow: time.Minute,
		},
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:     svc,
		Tokens:   tm,
		Profiles: newFakeProfiles(),
		Mailer:   mail,
	})
	return svc, mail, h
}

func doJSON(h http.Handler, method, path, bearer string, body any) *httptest.ResponseRecorder {
	req := reqJSON(method, path, body)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func authErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return body.Error.Code
}

func tokenFromMail(body string) string {
	i := strings.Index(body, "token=")
	if i < 0 {
		return ""
	}
	rest := body[i+len("token="):]
	if j := strings.IndexAny(rest, " \r\n"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func TestForgotPasswordDoesNotEnumerateAndResets(t *testing.T) {
	_, mail, h := newLifecycleServer(t, 0)

	rec := doJSON(h, http.MethodPost, "/api/auth/forgot-password", "", map[string]string{"email": "nobody@test.dev"})
	if rec.Code != http.StatusAccepted || mail.count() != 0 {
		t.Fatalf("unknown email: %d mails=%d", rec.Code, mail.count())
	}

	rec = doJSON(h, http.MethodPost, "/api/auth/register", "", map[string]string{"email": "reset@test.dev", "password": "longenough1"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(h, http.MethodPost, "/api/auth/forgot-password", "", map[string]string{"email": "reset@test.dev"})
	if rec.Code != http.StatusAccepted || mail.count() != 1 {
		t.Fatalf("known email: %d mails=%d", rec.Code, mail.count())
	}
	token := tokenFromMail(mail.last().Body)
	if token == "" || mail.last().To != "reset@test.dev" {
		t.Fatalf("mail: %+v", mail.last())
	}

	if rec := doJSON(h, http.MethodPost, "/api/auth/reset-password", "", map[string]string{"token": "bad", "password": "longenough2"}); rec.Code != http.StatusBadRequest || authErrorCode(t, rec) != "invalid_token" {
		t.Fatalf("bad token: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(h, http.MethodPost, "/api/auth/reset-password", "", map[string]string{"token": token, "password": "short"}); rec.Code != http.StatusBadRequest || authErrorCode(t, rec) != "weak_password" {
		t.Fatalf("weak: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(h, http.MethodPost, "/api/auth/reset-password", "", map[string]string{"token": token, "password": "longenough2"}); rec.Code != http.StatusNoContent {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(h, http.MethodPost, "/api/auth/login", "", map[string]string{"email": "reset@test.dev", "password": "longenough1"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old password: %d", rec.Code)
	}
	if rec := doJSON(h, http.MethodPost, "/api/auth/login", "", map[string]string{"email": "reset@test.dev", "password": "longenough2"}); rec.Code != http.StatusOK {
		t.Fatalf("new password: %d %s", rec.Code, rec.Body.String())
	}
}

func TestVerificationRequiredFlow(t *testing.T) {
	svc, mail, h := newLifecycleServer(t, 0)
	svc.SetEmailVerificationRequired(true)

	rec := doJSON(h, http.MethodPost, "/api/auth/register", "", map[string]string{"email": "verify@test.dev", "password": "longenough1"})
	if rec.Code != http.StatusCreated || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("register: %d cookies=%d body=%s", rec.Code, len(rec.Result().Cookies()), rec.Body.String())
	}
	var body struct {
		VerificationRequired bool `json:"verificationRequired"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || !body.VerificationRequired {
		t.Fatalf("register body: %s err=%v", rec.Body.String(), err)
	}
	if mail.count() != 1 {
		t.Fatalf("verification mail count = %d", mail.count())
	}
	if rec := doJSON(h, http.MethodPost, "/api/auth/login", "", map[string]string{"email": "verify@test.dev", "password": "longenough1"}); rec.Code != http.StatusForbidden || authErrorCode(t, rec) != "email_unverified" {
		t.Fatalf("unverified login: %d %s", rec.Code, rec.Body.String())
	}
	token := tokenFromMail(mail.last().Body)
	if rec := doJSON(h, http.MethodPost, "/api/auth/verify-email", "", map[string]string{"token": token}); rec.Code != http.StatusNoContent {
		t.Fatalf("verify: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(h, http.MethodPost, "/api/auth/login", "", map[string]string{"email": "verify@test.dev", "password": "longenough1"}); rec.Code != http.StatusOK {
		t.Fatalf("verified login: %d %s", rec.Code, rec.Body.String())
	}

	before := mail.count()
	if rec := doJSON(h, http.MethodPost, "/api/auth/verify-email/resend", "", map[string]string{"email": "nobody@test.dev"}); rec.Code != http.StatusAccepted {
		t.Fatalf("resend unknown: %d", rec.Code)
	}
	if mail.count() != before {
		t.Fatal("resend must not mail unknown addresses")
	}
}

func TestSessionListAndRevoke(t *testing.T) {
	_, _, h := newLifecycleServer(t, 0)

	recA := doJSON(h, http.MethodPost, "/api/auth/register", "", map[string]string{"email": "a@test.dev", "password": "longenough1"})
	if recA.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", recA.Code, recA.Body.String())
	}
	tokenA := accessToken(t, recA)
	recB := doJSON(h, http.MethodPost, "/api/auth/login", "", map[string]string{"email": "a@test.dev", "password": "longenough1"})
	tokenB := accessToken(t, recB)

	type sessionList struct {
		Sessions []struct {
			ID      string `json:"id"`
			Current bool   `json:"current"`
		} `json:"sessions"`
	}
	list := func(bearer string) sessionList {
		rec := doJSON(h, http.MethodGet, "/api/account/sessions", bearer, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("list sessions: %d %s", rec.Code, rec.Body.String())
		}
		var out sessionList
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	sessions := list(tokenB).Sessions
	if len(sessions) != 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
	current, other := "", ""
	for _, s := range sessions {
		if s.Current {
			current = s.ID
		} else {
			other = s.ID
		}
	}
	if current == "" || other == "" {
		t.Fatalf("current marking wrong: %+v", sessions)
	}

	if rec := doJSON(h, http.MethodDelete, "/api/account/sessions/not-a-uuid", tokenB, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", rec.Code)
	}

	// Another user cannot revoke this user's session.
	recC := doJSON(h, http.MethodPost, "/api/auth/register", "", map[string]string{"email": "c@test.dev", "password": "longenough1"})
	tokenC := accessToken(t, recC)
	otherSession := list(tokenC).Sessions[0].ID
	if rec := doJSON(h, http.MethodDelete, "/api/account/sessions/"+otherSession, tokenB, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user revoke: %d %s", rec.Code, rec.Body.String())
	}

	if rec := doJSON(h, http.MethodDelete, "/api/account/sessions/"+other, tokenB, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke other: %d %s", rec.Code, rec.Body.String())
	}
	if left := list(tokenB).Sessions; len(left) != 1 || !left[0].Current {
		t.Fatalf("after revoke: %+v", left)
	}

	rec := doJSON(h, http.MethodDelete, "/api/account/sessions", tokenB, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke all: %d %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge >= 0 {
		t.Fatalf("revoke all should clear the refresh cookie: %+v", cookies)
	}
	if rec := doJSON(h, http.MethodGet, "/api/account/sessions", tokenB, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked access token must be rejected: %d %s", rec.Code, rec.Body.String())
	}
	_ = tokenA
}

func accessToken(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode token from %s: %v", rec.Body.String(), err)
	}
	if body.AccessToken == "" {
		t.Fatalf("missing access token: %s", rec.Body.String())
	}
	return body.AccessToken
}

func TestAuthRateLimit(t *testing.T) {
	_, _, h := newLifecycleServer(t, 1)
	if rec := doJSON(h, http.MethodPost, "/api/auth/register", "", map[string]string{"email": "rate@test.dev", "password": "longenough1"}); rec.Code != http.StatusCreated {
		t.Fatalf("register: %d", rec.Code)
	}
	if rec := doJSON(h, http.MethodPost, "/api/auth/login", "", map[string]string{"email": "rate@test.dev", "password": "wrong"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("first login: %d", rec.Code)
	}
	rec := doJSON(h, http.MethodPost, "/api/auth/login", "", map[string]string{"email": "rate@test.dev", "password": "wrong"})
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("second login: %d retry=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if code := authErrorCode(t, rec); code != "rate_limited" {
		t.Fatalf("code = %q", code)
	}
}
