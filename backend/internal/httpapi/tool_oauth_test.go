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

func newToolOAuthTestServer(t *testing.T, wfStore *fakeWorkflowStore, auditStore *fakeAuditStore) (*Server, http.Handler, string) {
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
			GoogleCalendar: config.OAuthProviderConfig{
				ClientID: "google_client_123",
				Secret:   "google_secret_456",
			},
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

	mcpStore := newFakeMCPServerStore()

	s, h := newServer(ServerDeps{
		Cfg:        cfg,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:       svc,
		Tokens:     tm,
		Workflows:  wfStore,
		MCPServers: mcpStore,
		Audits:     auditStore,
		Vault:      vault.New(secret),
	})
	return s, h, tok
}

func TestToolOAuthProvidersEndpoint(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{}
	_, handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

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

	if len(resp.Providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(resp.Providers))
	}
	if resp.Providers[0].ID != "google_calendar" || !resp.Providers[0].Configured {
		t.Errorf("expected configured google_calendar provider, got %+v", resp.Providers[0])
	}
	if resp.Providers[1].ID != "github" || !resp.Providers[1].Configured {
		t.Errorf("expected configured github provider, got %+v", resp.Providers[1])
	}
	if resp.Providers[2].ID != "slack" || !resp.Providers[2].Configured {
		t.Errorf("expected configured slack provider, got %+v", resp.Providers[2])
	}
}

func TestToolOAuthProvidersWithMCP(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{}
	server, handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

	// 1. Link an MCP server to GitHub app
	ghApp := "github"
	_, err := server.deps.MCPServers.Create(context.Background(), storage.MCPServer{
		UserID:    "test-user-id",
		Name:      "GitHub Copilot MCP",
		URL:       "http://localhost:3000/mcp",
		Transport: "http",
		AppID:     &ghApp,
		Enabled:   true,
	})
	if err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	// 2. Query /api/tool-oauth/providers
	req := httptest.NewRequest(http.MethodGet, "/api/tool-oauth/providers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Providers []toolProviderDTO `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var ghProvider *toolProviderDTO
	for _, p := range resp.Providers {
		if p.ID == "github" {
			ghProvider = &p
			break
		}
	}
	if ghProvider == nil || !ghProvider.Connected || ghProvider.ConnectedVia != "mcp" {
		t.Fatalf("expected github connected via mcp, got %+v", ghProvider)
	}
	if ghProvider.MCPServerName != "GitHub Copilot MCP" {
		t.Errorf("expected MCPServerName 'GitHub Copilot MCP', got %q", ghProvider.MCPServerName)
	}

	// 3. Disconnect provider
	req = httptest.NewRequest(http.MethodPost, "/api/tool-oauth/github/disconnect", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Query again, should be disconnected
	req = httptest.NewRequest(http.MethodGet, "/api/tool-oauth/providers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	for _, p := range resp.Providers {
		if p.ID == "github" && p.Connected {
			t.Fatalf("expected github to be disconnected after unlink, got %+v", p)
		}
	}
}

func TestToolOAuthStartEndpoint(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{}
	_, handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

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

func TestToolOAuthStartGoogleCalendar_WithReturnTo(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{}
	_, handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

	req := httptest.NewRequest(http.MethodGet, "/api/tool-oauth/google_calendar/start?return_to=/chat/conv_abc", nil)
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

	if !strings.HasPrefix(resp.URL, "https://accounts.google.com/o/oauth2/v2/auth?") {
		t.Fatalf("unexpected auth url: %s", resp.URL)
	}
	if !strings.Contains(resp.URL, "access_type=offline") || !strings.Contains(resp.URL, "prompt=consent") {
		t.Fatalf("missing offline or prompt=consent in url: %s", resp.URL)
	}
	if !strings.Contains(resp.URL, "calendar.readonly") || !strings.Contains(resp.URL, "calendar.events") {
		t.Fatalf("missing calendar scopes in url: %s", resp.URL)
	}

	cookies := rec.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == toolOAuthCookie {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected state cookie to be set")
	}

	// Verify state decodes return_to
	_, _, _, returnTo, ok := openToolOAuthState([]byte("0123456789abcdef0123456789abcdef"), stateCookie.Value, time.Now())
	if !ok || returnTo != "/chat/conv_abc" {
		t.Fatalf("expected returnTo='/chat/conv_abc', got %q (ok=%v)", returnTo, ok)
	}
}

func TestToolOAuthCallbackGoogleCalendar_Success(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{}
	server, handler, _ := newToolOAuthTestServer(t, wfStore, auditStore)

	// Mock Google Token endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"access_token": "mock_google_access_token_123",
				"expires_in": 3600,
				"refresh_token": "mock_google_refresh_token_xyz",
				"scope": "https://www.googleapis.com/auth/calendar.readonly https://www.googleapis.com/auth/calendar.events",
				"token_type": "Bearer"
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	server.deps.Cfg.ToolOAuth.CalendarBaseURL = ts.URL

	state := "random_test_state_123"
	sealed := sealToolOAuthState([]byte(server.deps.Cfg.JWTSecret), "test-user-id", "google_calendar", state, "/chat/conv_999", time.Now().Add(10*time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/api/tool-oauth/google_calendar/callback?code=mock_code&state="+state, nil)
	req.AddCookie(&http.Cookie{
		Name:  toolOAuthCookie,
		Value: sealed,
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 Found, got %d: %s", rec.Code, rec.Body.String())
	}

	loc := rec.Header().Get("Location")
	if loc != "http://localhost:5173/chat/conv_999?connected=google_calendar" {
		t.Fatalf("expected redirect to chat with connected param, got: %s", loc)
	}

	// Verify credential was saved and encrypted
	cred, err := wfStore.GetCredentialByProvider(context.Background(), "test-user-id", "google_calendar")
	if err != nil {
		t.Fatalf("expected credential in store, got err: %v", err)
	}
	dec := server.decryptCredentialData(cred.Data)
	if dec["token"] != "mock_google_access_token_123" || dec["refresh_token"] != "mock_google_refresh_token_xyz" {
		t.Fatalf("unexpected stored decrypted data: %+v", dec)
	}
	if cred.ExpiresAt == nil || time.Until(*cred.ExpiresAt) < 30*time.Minute {
		t.Fatalf("expected expiration ~1h in future, got %v", cred.ExpiresAt)
	}
}

func TestGetValidGoogleCalendarToken_AutoRefresh(t *testing.T) {
	wfStore := newFakeWorkflowStore()
	auditStore := &fakeAuditStore{}
	server, _, _ := newToolOAuthTestServer(t, wfStore, auditStore)

	refreshed := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			refreshed = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"access_token": "brand_new_refreshed_access_token",
				"expires_in": 3600
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	server.deps.Cfg.ToolOAuth.CalendarBaseURL = ts.URL

	// Seed credential expiring in 1 minute (< 5 minute threshold)
	expiringSoon := time.Now().Add(1 * time.Minute)
	credData := map[string]string{
		"token":         "old_expired_token",
		"refresh_token": "valid_refresh_token_abc",
	}
	rawBytes, _ := json.Marshal(credData)
	enc, _ := server.deps.Vault.Encrypt(rawBytes)

	_, _ = wfStore.CreateCredential(context.Background(), storage.WorkflowCredential{
		UserID:    "test-user-id",
		Name:      "google_calendar",
		Type:      "oauth2",
		Provider:  "google_calendar",
		ExpiresAt: &expiringSoon,
		Data:      enc,
	})

	token, err := server.getValidGoogleCalendarToken(context.Background(), "test-user-id")
	if err != nil {
		t.Fatalf("getValidGoogleCalendarToken: %v", err)
	}
	if !refreshed {
		t.Fatal("expected token to be refreshed via Google OAuth refresh_token flow")
	}
	if token != "brand_new_refreshed_access_token" {
		t.Fatalf("expected new token, got: %s", token)
	}

	// Verify database record was updated with new expiration and new token
	cred, _ := wfStore.GetCredentialByProvider(context.Background(), "test-user-id", "google_calendar")
	updatedDec := server.decryptCredentialData(cred.Data)
	if updatedDec["token"] != "brand_new_refreshed_access_token" {
		t.Fatalf("expected updated credential in DB, got: %+v", updatedDec)
	}
	if time.Until(*cred.ExpiresAt) < 30*time.Minute {
		t.Fatalf("expected updated expiration in DB, got: %v", cred.ExpiresAt)
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
	_, handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

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

	_, handler, token := newToolOAuthTestServer(t, wfStore, auditStore)

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
