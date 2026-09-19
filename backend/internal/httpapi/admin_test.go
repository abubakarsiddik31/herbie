package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type fakeAdminUsersStore struct {
	users []auth.UserRecord
}

func (f *fakeAdminUsersStore) List(_ context.Context) ([]auth.UserRecord, error) {
	return f.users, nil
}

func (f *fakeAdminUsersStore) Count(_ context.Context) (int, error) {
	return len(f.users), nil
}

func (f *fakeAdminUsersStore) SetRole(_ context.Context, userID, role string) error {
	for i, u := range f.users {
		if u.ID == userID {
			f.users[i].Role = role
			return nil
		}
	}
	return storage.ErrNotFound
}

func setupAdminTestServer(t *testing.T, userStore *fakeAdminUsersStore, auditStore storage.AuditStore) (*Server, http.Handler, *auth.TokenMaker) {
	t.Helper()
	secret := "0123456789abcdef0123456789abcdef"
	tm, err := auth.NewTokenMaker(secret)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	fakeUsers := authtest.NewFakeUsers()
	svc, err := auth.NewService(fakeUsers, authtest.NewFakeRefreshStore(), authtest.NewFakeOAuthStore(fakeUsers), secret)
	if err != nil {
		t.Fatal(err)
	}

	s, h := newServer(ServerDeps{
		Cfg:    config.Config{FrontendOrigin: "http://localhost:5173"},
		Log:    log,
		Auth:   svc,
		Tokens: tm,
		Users:  userStore,
		Audits: auditStore,
	})
	return s, h, tm
}

func TestAdminEndpointsRBAC(t *testing.T) {
	userStore := &fakeAdminUsersStore{
		users: []auth.UserRecord{
			{ID: "admin-1", Email: "admin@test.dev", Role: "admin", CreatedAt: time.Now()},
			{ID: "user-1", Email: "user@test.dev", Role: "user", CreatedAt: time.Now()},
		},
	}
	fakeAudits := &fakeAuditStore{
		audits: []storage.ToolAuditLog{
			{ID: "a-1", UserID: "admin-1", ToolName: "test_tool", Action: "execute", CreatedAt: time.Now()},
			{ID: "a-2", UserID: "user-1", ToolName: "test_tool", Action: "execute", CreatedAt: time.Now()},
		},
	}

	_, h, tm := setupAdminTestServer(t, userStore, fakeAudits)
	adminToken, _, _ := tm.IssueWithRole("admin-1", "admin", time.Now())
	userToken, _, _ := tm.IssueWithRole("user-1", "user", time.Now())

	// 1. Unauthenticated request to /api/admin/users must 401
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthed request, got %d", rec.Code)
	}

	// 2. Regular user request to /api/admin/users must 403 Forbidden
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for regular user, got %d", rec.Code)
	}

	// 3. Admin request to /api/admin/users must 200 OK and list users
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin, got %d: %s", rec.Code, rec.Body.String())
	}
	var usersResp struct {
		Users []adminUserDTO `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &usersResp); err != nil {
		t.Fatalf("unmarshal users: %v", err)
	}
	if len(usersResp.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(usersResp.Users))
	}

	// 4. Admin updates user role
	body, _ := json.Marshal(map[string]string{"role": "admin"})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/admin/users/user-1/role", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK updating role, got %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Admin stats
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for stats, got %d", rec.Code)
	}

	// 6. Admin audits
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/admin/audits", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin audits, got %d", rec.Code)
	}
	var auditsResp struct {
		Audits []storage.ToolAuditLog `json:"audits"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &auditsResp)
	// 2 initial + 1 from the role update above
	if len(auditsResp.Audits) != 3 {
		t.Fatalf("expected 3 audits, got %d", len(auditsResp.Audits))
	}

	// 7. Regular user calling /api/tool-audit-logs?all=true should only see their own logs
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/tool-audit-logs?all=true", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var userAuditsResp struct {
		Audits []storage.ToolAuditLog `json:"audits"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &userAuditsResp)
	if len(userAuditsResp.Audits) != 1 || userAuditsResp.Audits[0].UserID != "user-1" {
		t.Fatalf("expected non-admin to only see own audit, got %+v", userAuditsResp.Audits)
	}

	// 8. Admin calling /api/tool-audit-logs?all=true should see all audits
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/tool-audit-logs?all=true", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var adminAllAuditsResp struct {
		Audits []storage.ToolAuditLog `json:"audits"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &adminAllAuditsResp)
	if len(adminAllAuditsResp.Audits) != 3 {
		t.Fatalf("expected admin to see all audits when all=true, got %d", len(adminAllAuditsResp.Audits))
	}
}

func TestAdminDemoteLockoutPrevention(t *testing.T) {
	userStore := &fakeAdminUsersStore{
		users: []auth.UserRecord{
			{ID: "sole-admin", Email: "admin@test.dev", Role: "admin"},
			{ID: "regular-user", Email: "user@test.dev", Role: "user"},
		},
	}
	_, h, tm := setupAdminTestServer(t, userStore, nil)
	adminToken, _, _ := tm.IssueWithRole("sole-admin", "admin", time.Now())

	// Sole admin attempting to demote themselves
	body, _ := json.Marshal(map[string]string{"role": "user"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/sole-admin/role", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request to prevent sole admin lockout, got %d: %s", rec.Code, rec.Body.String())
	}
}
