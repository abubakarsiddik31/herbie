package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/mcp"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type fakeMCPServerStore struct {
	servers map[string]storage.MCPServer
}

func newFakeMCPServerStore() *fakeMCPServerStore {
	return &fakeMCPServerStore{servers: make(map[string]storage.MCPServer)}
}

func (f *fakeMCPServerStore) Create(_ context.Context, s storage.MCPServer) (storage.MCPServer, error) {
	if s.ID == "" {
		s.ID = "mcp-" + strconv.Itoa(len(f.servers)+1)
	}
	for _, existing := range f.servers {
		if existing.UserID == s.UserID && existing.Name == s.Name {
			return storage.MCPServer{}, storage.ErrDuplicate
		}
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	f.servers[s.ID] = s
	return s, nil
}

func (f *fakeMCPServerStore) List(_ context.Context, userID string) ([]storage.MCPServer, error) {
	var res []storage.MCPServer
	for _, s := range f.servers {
		if s.UserID == userID {
			res = append(res, s)
		}
	}
	return res, nil
}

func (f *fakeMCPServerStore) ListEnabled(_ context.Context, userID string) ([]storage.MCPServer, error) {
	var res []storage.MCPServer
	for _, s := range f.servers {
		if s.UserID == userID && s.Enabled {
			res = append(res, s)
		}
	}
	return res, nil
}

func (f *fakeMCPServerStore) ByID(_ context.Context, id, userID string) (storage.MCPServer, error) {
	s, ok := f.servers[id]
	if !ok || s.UserID != userID {
		return storage.MCPServer{}, storage.ErrNotFound
	}
	return s, nil
}

func (f *fakeMCPServerStore) ByApp(_ context.Context, appID, userID string) (storage.MCPServer, error) {
	for _, s := range f.servers {
		if s.UserID == userID && s.AppID != nil && *s.AppID == appID && s.Enabled {
			return s, nil
		}
	}
	return storage.MCPServer{}, storage.ErrNotFound
}

func (f *fakeMCPServerStore) UnlinkApp(_ context.Context, appID, userID string) error {
	found := false
	for id, s := range f.servers {
		if s.UserID == userID && s.AppID != nil && *s.AppID == appID {
			s.AppID = nil
			s.UpdatedAt = time.Now()
			f.servers[id] = s
			found = true
		}
	}
	if !found {
		return storage.ErrNotFound
	}
	return nil
}

func (f *fakeMCPServerStore) Update(_ context.Context, s storage.MCPServer) (storage.MCPServer, error) {
	existing, ok := f.servers[s.ID]
	if !ok || existing.UserID != s.UserID {
		return storage.MCPServer{}, storage.ErrNotFound
	}
	s.UpdatedAt = time.Now()
	f.servers[s.ID] = s
	return s, nil
}

func (f *fakeMCPServerStore) Delete(_ context.Context, id, userID string) error {
	s, ok := f.servers[id]
	if !ok || s.UserID != userID {
		return storage.ErrNotFound
	}
	delete(f.servers, id)
	return nil
}

func newMCPTestServer(t *testing.T, mcpStore *fakeMCPServerStore) (http.Handler, string) {
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
		JWTSecret:             secret,
		FrontendOrigin:        "http://localhost:5173",
		ToolAllowPrivateHosts: true, // allow httptest local servers in tests
	}

	h := NewServer(ServerDeps{
		Cfg:        cfg,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:       svc,
		Tokens:     tm,
		MCPServers: mcpStore,
	})
	return h, tok
}

func TestMCPServersCRUD(t *testing.T) {
	store := newFakeMCPServerStore()
	handler, token := newMCPTestServer(t, store)

	// 1. Initially empty
	req := httptest.NewRequest(http.MethodGet, "/api/mcp/servers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var listResp struct {
		Servers []storage.MCPServer `json:"servers"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Servers) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(listResp.Servers))
	}

	// 2. Create an MCP server
	body := `{"name":"postgres_mcp","url":"http://localhost:3000/mcp"}`
	req = httptest.NewRequest(http.MethodPost, "/api/mcp/servers", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp struct {
		Server storage.MCPServer `json:"server"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)
	if createResp.Server.Name != "postgres_mcp" || !createResp.Server.Enabled {
		t.Fatalf("unexpected created server: %+v", createResp.Server)
	}

	// 3. Toggle enabled
	serverID := createResp.Server.ID
	req = httptest.NewRequest(http.MethodPost, "/api/mcp/servers/"+serverID+"/toggle", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var toggleResp struct {
		Server storage.MCPServer `json:"server"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &toggleResp)
	if toggleResp.Server.Enabled {
		t.Fatalf("expected server to be disabled after toggle")
	}

	// 4. Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/mcp/servers/"+serverID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(store.servers) != 0 {
		t.Fatal("expected server to be deleted")
	}
}

func TestMCPServerTestEndpoint(t *testing.T) {
	// Mock external MCP server
	mockMCP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mcp.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result: mcp.ListToolsResult{
				Tools: []mcp.Tool{
					{
						Name:        "query_database",
						Description: "Execute a read-only SQL query",
						InputSchema: json.RawMessage(`{"type":"object"}`),
					},
				},
			},
		})
	}))
	defer mockMCP.Close()

	store := newFakeMCPServerStore()
	handler, token := newMCPTestServer(t, store)

	body, _ := json.Marshal(map[string]string{
		"url": mockMCP.URL,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/mcp/servers/test", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var testResp struct {
		Ok    bool       `json:"ok"`
		Count int        `json:"count"`
		Tools []mcp.Tool `json:"tools"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &testResp)
	if !testResp.Ok || testResp.Count != 1 || testResp.Tools[0].Name != "query_database" {
		t.Fatalf("unexpected test response: %+v", testResp)
	}
}

func TestMCPEndpointJSONRPC(t *testing.T) {
	store := newFakeMCPServerStore()
	handler, token := newMCPTestServer(t, store)

	// Call POST /api/mcp with initialize
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp mcp.JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	// Call POST /api/mcp with tools/list
	reqBody = `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	req = httptest.NewRequest(http.MethodPost, "/api/mcp", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var listResp mcp.JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if listResp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", listResp.Error)
	}
}

func TestMCPServerAppLinking(t *testing.T) {
	store := newFakeMCPServerStore()
	handler, token := newMCPTestServer(t, store)

	// 1. Create an MCP server linked to "github" app
	body := `{"name":"github_mcp","url":"http://localhost:3000/mcp","appId":"github"}`
	req := httptest.NewRequest(http.MethodPost, "/api/mcp/servers", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp struct {
		Server storage.MCPServer `json:"server"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &createResp)
	if createResp.Server.AppID == nil || *createResp.Server.AppID != "github" {
		t.Fatalf("expected appId 'github', got %+v", createResp.Server.AppID)
	}
	serverID := createResp.Server.ID

	// 2. Unlink app
	req = httptest.NewRequest(http.MethodPost, "/api/mcp/servers/"+serverID+"/unlink-app", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var unlinkResp struct {
		Server storage.MCPServer `json:"server"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unlinkResp)
	if unlinkResp.Server.AppID != nil {
		t.Fatalf("expected appId to be nil, got %v", unlinkResp.Server.AppID)
	}

	// 3. Update server to link to "slack"
	updateBody := `{"name":"slack_mcp","appId":"slack"}`
	req = httptest.NewRequest(http.MethodPut, "/api/mcp/servers/"+serverID, bytes.NewReader([]byte(updateBody)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updateResp struct {
		Server storage.MCPServer `json:"server"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &updateResp)
	if updateResp.Server.AppID == nil || *updateResp.Server.AppID != "slack" {
		t.Fatalf("expected appId 'slack', got %v", updateResp.Server.AppID)
	}
}

func TestMCPCatalogOneClick(t *testing.T) {
	store := newFakeMCPServerStore()
	handler, token := newMCPTestServer(t, store)

	// 1. List Catalog
	req := httptest.NewRequest(http.MethodGet, "/api/mcp/catalog", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var catResp struct {
		Catalog []mcp.CatalogApp `json:"catalog"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &catResp)
	if len(catResp.Catalog) < 4 {
		t.Fatalf("expected at least 4 catalog apps, got %d", len(catResp.Catalog))
	}
	for _, app := range catResp.Catalog {
		if app.ID == "github" && app.Connected {
			t.Fatal("expected github to initially be disconnected")
		}
	}

	// 2. 1-Click Link GitHub
	req = httptest.NewRequest(http.MethodPost, "/api/mcp/catalog/github/link", bytes.NewReader([]byte(`{"token":"ghp_123"}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Query catalog again
	req = httptest.NewRequest(http.MethodGet, "/api/mcp/catalog", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	_ = json.Unmarshal(rec.Body.Bytes(), &catResp)
	var ghFound bool
	for _, app := range catResp.Catalog {
		if app.ID == "github" {
			ghFound = true
			if !app.Connected {
				t.Fatal("expected github to be connected after 1-click link")
			}
		}
	}
	if !ghFound {
		t.Fatal("github not found in catalog")
	}

	// 4. 1-Click Unlink GitHub
	req = httptest.NewRequest(http.MethodPost, "/api/mcp/catalog/github/unlink", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Query catalog again
	req = httptest.NewRequest(http.MethodGet, "/api/mcp/catalog", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	_ = json.Unmarshal(rec.Body.Bytes(), &catResp)
	for _, app := range catResp.Catalog {
		if app.ID == "github" && app.Connected {
			t.Fatal("expected github to be disconnected after unlink")
		}
	}
}
