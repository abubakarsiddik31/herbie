package httpapi

import (
	"context"
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
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem-chatbot/internal/vault"
)

type fakeAuditStore struct {
	audits []storage.ToolAuditLog
}

func (f *fakeAuditStore) RecordToolAudit(_ context.Context, log storage.ToolAuditLog) error {
	f.audits = append(f.audits, log)
	return nil
}

func (f *fakeAuditStore) ListToolAudits(_ context.Context, userID string, limit int) ([]storage.ToolAuditLog, error) {
	var res []storage.ToolAuditLog
	for i := len(f.audits) - 1; i >= 0; i-- {
		if f.audits[i].UserID == userID {
			res = append(res, f.audits[i])
			if len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

func newToolOAuthTestServer(t *testing.T, wfStore *fakeWorkflowStore, auditStore *fakeAuditStore) (http.Handler, string) {
	t.Helper()
	secret := "0123456789abcdef0123456789abcdef"
	svc := authtest.NewService(secret)
	tm, err := auth.NewTokenMaker(secret)
	if err != nil {
		t.Fatal(err)
	}

	tok, _, err := tm.Issue("test-user-id", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		JWTSecret:      secret,
		FrontendOrigin: "http://localhost:5173",
		ToolOAuth: config.ToolOAuthConfig{
			GitHub: config.OAuthProviderConfig{
				ClientID: "gh_client_123",
				Secret:   "gh_secret_456",
			},
			Slack: config.OAuthProviderConfig{
				ClientID: "slack_client_789",
				Secret:   "slack_secret_012",
			},
			RedirectBase: "http://localhost:8090",
		},
	}

	h := NewServer(ServerDeps{
		Cfg:       cfg,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:      svc,
		Tokens:    tm,
		Workflows: wfStore,
		Audits:    auditStore,
		Vault:     vault.New(secret),
	})
	return h, tok
}

func TestToolOAuthProvidersEndpoint(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{}
	handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

	req := httptest.NewRequest(http.MethodGet, "/api/tool-oauth/providers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Providers []toolProviderDTO `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(resp.Providers))
	}
	if resp.Providers[0].ID != "github" || !resp.Providers[0].Configured {
		t.Errorf("expected configured github provider, got %+v", resp.Providers[0])
	}
	if resp.Providers[1].ID != "slack" || !resp.Providers[1].Configured {
		t.Errorf("expected configured slack provider, got %+v", resp.Providers[1])
	}
}

func TestToolOAuthStartEndpoint(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{}
	handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

	req := httptest.NewRequest(http.MethodGet, "/api/tool-oauth/github/start", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !strings.HasPrefix(resp.URL, "https://github.com/login/oauth/authorize?") {
		t.Fatalf("unexpected auth url: %s", resp.URL)
	}
	if !strings.Contains(resp.URL, "client_id=gh_client_123") {
		t.Fatalf("missing client id in url: %s", resp.URL)
	}

	cookies := rec.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == toolOAuthCookie {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatal("expected state cookie to be set")
	}
}

func TestToolAuditLogsEndpoint(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{
		audits: []storage.ToolAuditLog{
			{
				ID:           "a1",
				UserID:       "test-user-id",
				CallerType:   "chat_agent",
				ToolName:     "web_search",
				Action:       "execute",
				InputSummary: `{"q":"golang"}`,
				Status:       "success",
				DurationMs:   120,
				CreatedAt:    time.Now(),
			},
		},
	}
	handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

	req := httptest.NewRequest(http.MethodGet, "/api/tool-audit-logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Audits []storage.ToolAuditLog `json:"audits"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Audits) != 1 || resp.Audits[0].ToolName != "web_search" {
		t.Fatalf("unexpected audits response: %+v", resp.Audits)
	}
}

func TestToolOAuthDisconnectEndpoint(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{}
	// Seed a connected github credential
	wfStore.credentials["cred-1"] = storage.WorkflowCredential{
		ID:       "cred-1",
		UserID:   "test-user-id",
		Name:     "github",
		Type:     "oauth2",
		Provider: "github",
		Data:     json.RawMessage(`{"token":"secret-token"}`),
	}

	handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

	req := httptest.NewRequest(http.MethodPost, "/api/tool-oauth/github/disconnect", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if _, ok := wfStore.credentials["cred-1"]; ok {
		t.Fatal("expected credential to be deleted")
	}

	// Verify audit log was written
	if len(auditStore.audits) != 1 || auditStore.audits[0].Action != "oauth_disconnect" {
		t.Fatalf("expected oauth_disconnect audit log, got %+v", auditStore.audits)
	}
}
