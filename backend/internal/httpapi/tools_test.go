package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type fakeToolStore struct {
	mu    sync.Mutex
	rows  map[string]storage.UserTool // by id
	seq   int
	count int
}

func newFakeToolStore() *fakeToolStore {
	return &fakeToolStore{rows: map[string]storage.UserTool{}}
}

var _ ToolStore = (*fakeToolStore)(nil)

func (f *fakeToolStore) Create(_ context.Context, t storage.UserTool) (storage.UserTool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.rows {
		if row.UserID == t.UserID && row.Name == t.Name {
			return storage.UserTool{}, storage.ErrDuplicate
		}
	}
	f.seq++
	t.ID = "tool-" + string(rune('0'+f.seq))
	f.rows[t.ID] = t
	return t, nil
}

func (f *fakeToolStore) List(_ context.Context, userID string) ([]storage.UserTool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []storage.UserTool
	for _, row := range f.rows {
		if row.UserID == userID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeToolStore) ByID(_ context.Context, id, userID string) (storage.UserTool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok || row.UserID != userID {
		return storage.UserTool{}, storage.ErrNotFound
	}
	return row, nil
}

func (f *fakeToolStore) Update(_ context.Context, t storage.UserTool) (storage.UserTool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[t.ID]
	if !ok || row.UserID != t.UserID {
		return storage.UserTool{}, storage.ErrNotFound
	}
	for id, other := range f.rows {
		if id != t.ID && other.UserID == t.UserID && other.Name == t.Name {
			return storage.UserTool{}, storage.ErrDuplicate
		}
	}
	f.rows[t.ID] = t
	return t, nil
}

func (f *fakeToolStore) Delete(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok || row.UserID != userID {
		return storage.ErrNotFound
	}
	delete(f.rows, id)
	return nil
}

func (f *fakeToolStore) Count(_ context.Context, userID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, row := range f.rows {
		if row.UserID == userID {
			n++
		}
	}
	return n, nil
}

func validWeatherBody() map[string]any {
	return map[string]any{
		"name":        "get_weather",
		"description": "Get current weather",
		"method":      "GET",
		"urlTemplate": "https://api.open-meteo.com/v1/forecast",
		"params": []map[string]any{
			{"name": "latitude", "in": "query", "type": "number", "required": true, "description": "lat"},
		},
		"headers": map[string]string{"Authorization": "Bearer sk-secret"},
	}
}

func doTools(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		req = reqJSON(method, path, body)
	} else {
		req, _ = http.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateToolMasksHeaders(t *testing.T) {
	tools := newFakeToolStore()
	h, token := newHandlerServerWithTools(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), newFakeUsage(), tools)

	rec := doTools(t, h, token, http.MethodPost, "/api/tools", validWeatherBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body %s", rec.Code, rec.Body.String())
	}
	var dto toolDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.Headers["Authorization"] != headerMask {
		t.Fatalf("header not masked: %v", dto.Headers)
	}
	if dto.Enabled != true || dto.ID == "" {
		t.Fatalf("dto: %+v", dto)
	}

	list := doTools(t, h, token, http.MethodGet, "/api/tools", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), headerMask) {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), "sk-secret") {
		t.Fatal("secret leaked in list response")
	}
}

func TestCreateToolValidationAndDuplicate(t *testing.T) {
	tools := newFakeToolStore()
	h, token := newHandlerServerWithTools(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), newFakeUsage(), tools)

	bad := validWeatherBody()
	bad["name"] = "Get-Weather"
	if rec := doTools(t, h, token, http.MethodPost, "/api/tools", bad); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad name status = %d", rec.Code)
	}

	if rec := doTools(t, h, token, http.MethodPost, "/api/tools", validWeatherBody()); rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d", rec.Code)
	}
	rec := doTools(t, h, token, http.MethodPost, "/api/tools", validWeatherBody())
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d body %s", rec.Code, rec.Body.String())
	}
}

func TestPatchToolHeaderSemantics(t *testing.T) {
	tools := newFakeToolStore()
	h, token := newHandlerServerWithTools(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), newFakeUsage(), tools)

	created := doTools(t, h, token, http.MethodPost, "/api/tools", validWeatherBody())
	var dto toolDTO
	_ = json.Unmarshal(created.Body.Bytes(), &dto)

	// Masked value keeps the stored secret; null deletes the key.
	patch := map[string]any{
		"headers": map[string]any{
			"Authorization": headerMask,
			"X-Old":         nil,
		},
		"requireApproval": true,
	}
	rec := doTools(t, h, token, http.MethodPatch, "/api/tools/"+dto.ID, patch)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d body %s", rec.Code, rec.Body.String())
	}

	stored, err := tools.ByID(context.Background(), dto.ID, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	var headers map[string]string
	if err := json.Unmarshal(stored.Headers, &headers); err != nil {
		t.Fatal(err)
	}
	if headers["Authorization"] != "Bearer sk-secret" {
		t.Fatalf("masked value did not preserve secret: %v", headers)
	}
	if _, ok := headers["X-Old"]; ok {
		t.Fatalf("null did not delete header: %v", headers)
	}
	if !stored.RequireApproval {
		t.Fatal("requireApproval not applied")
	}
}

func TestPatchToolPreservesHeadersWhenAbsent(t *testing.T) {
	tools := newFakeToolStore()
	h, token := newHandlerServerWithTools(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), newFakeUsage(), tools)

	created := doTools(t, h, token, http.MethodPost, "/api/tools", validWeatherBody())
	var dto toolDTO
	_ = json.Unmarshal(created.Body.Bytes(), &dto)

	rec := doTools(t, h, token, http.MethodPatch, "/api/tools/"+dto.ID, map[string]any{"description": "updated"})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d", rec.Code)
	}
	stored, _ := tools.ByID(context.Background(), dto.ID, "u-1")
	var headers map[string]string
	_ = json.Unmarshal(stored.Headers, &headers)
	if headers["Authorization"] != "Bearer sk-secret" {
		t.Fatalf("headers lost on unrelated patch: %v", headers)
	}
	if stored.Description != "updated" {
		t.Fatalf("description = %q", stored.Description)
	}
}

func TestDeleteAndUnknownTool(t *testing.T) {
	tools := newFakeToolStore()
	h, token := newHandlerServerWithTools(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), newFakeUsage(), tools)

	created := doTools(t, h, token, http.MethodPost, "/api/tools", validWeatherBody())
	var dto toolDTO
	_ = json.Unmarshal(created.Body.Bytes(), &dto)

	if rec := doTools(t, h, token, http.MethodDelete, "/api/tools/"+dto.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if rec := doTools(t, h, token, http.MethodPatch, "/api/tools/"+dto.ID, map[string]any{"description": "x"}); rec.Code != http.StatusNotFound {
		t.Fatalf("patch after delete status = %d", rec.Code)
	}
	if rec := doTools(t, h, token, http.MethodPatch, "/api/tools/nope", map[string]any{"description": "x"}); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id status = %d", rec.Code)
	}
}

func TestToolsRequireAuth(t *testing.T) {
	tools := newFakeToolStore()
	h, _ := newHandlerServerWithTools(t, nil, newFakeConvos(newFakeMsgs()), newFakeMsgs(), newFakeUsage(), tools)
	req, _ := http.NewRequest(http.MethodGet, "/api/tools", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", rec.Code)
	}
}
