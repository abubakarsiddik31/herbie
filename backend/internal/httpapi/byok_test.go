package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/authtest"
	"github.com/abubakarsiddik31/golem-chatbot/internal/config"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem-chatbot/internal/vault"
)

type fakeProviderKeys struct {
	mu   sync.Mutex
	rows map[string][]byte
}

func newFakeProviderKeys() *fakeProviderKeys { return &fakeProviderKeys{rows: map[string][]byte{}} }

func (f *fakeProviderKeys) key(userID, provider string) string { return userID + "\x00" + provider }

func (f *fakeProviderKeys) Upsert(_ context.Context, userID, provider string, encrypted []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[f.key(userID, provider)] = encrypted
	return nil
}

func (f *fakeProviderKeys) Get(_ context.Context, userID, provider string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, ok := f.rows[f.key(userID, provider)]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return raw, nil
}

func (f *fakeProviderKeys) List(_ context.Context, userID string) ([]storage.UserProviderKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []storage.UserProviderKey
	for provider := range map[string]bool{"gemini": true, "openai": true, "anthropic": true} {
		if raw, ok := f.rows[f.key(userID, provider)]; ok {
			out = append(out, storage.UserProviderKey{Provider: provider, Encrypted: raw, UpdatedAt: time.Now()})
		}
	}
	return out, nil
}

func (f *fakeProviderKeys) Delete(_ context.Context, userID, provider string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rows[f.key(userID, provider)]; !ok {
		return storage.ErrNotFound
	}
	delete(f.rows, f.key(userID, provider))
	return nil
}

func newBYOKServer(t *testing.T, allow bool) (http.Handler, string, *fakeProviderKeys) {
	t.Helper()
	tm, err := auth.NewTokenMaker(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	keys := newFakeProviderKeys()
	h := NewServer(ServerDeps{
		Cfg:              config.Config{AllowUserProviderKeys: allow},
		Log:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		Auth:             authtest.NewService(testSecret),
		Tokens:           tm,
		Vault:            vault.New("test-master-secret"),
		UserProviderKeys: keys,
	})
	token, _, err := tm.Issue("u-1", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return h, token, keys
}

func TestBYOKLifecycle(t *testing.T) {
	h, token, keys := newBYOKServer(t, true)

	rec := doAuthed(t, h, token, http.MethodPut, "/api/provider-keys/gemini", map[string]string{"apiKey": "super-secret-key"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("set key: %d %s", rec.Code, rec.Body.String())
	}
	raw := keys.rows["u-1\x00gemini"]
	if len(raw) == 0 || string(raw) == "super-secret-key" {
		t.Fatalf("key not encrypted at rest: %q", raw)
	}

	rec = doAuthed(t, h, token, http.MethodGet, "/api/provider-keys", nil)
	if rec.Code != http.StatusOK || string(rec.Body.Bytes()) == "" {
		t.Fatalf("list keys: %d", rec.Code)
	}
	if containsSecret(rec.Body.Bytes(), "super-secret-key") {
		t.Fatal("provider key leaked from the list endpoint")
	}
	var list struct {
		AllowUserKeys bool `json:"allowUserKeys"`
		Providers     []struct {
			Provider   string `json:"provider"`
			Configured bool   `json:"configured"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if !list.AllowUserKeys || len(list.Providers) != 3 || !providerConfigured(list.Providers, "gemini") {
		t.Fatalf("list = %+v", list)
	}

	if rec := doAuthed(t, h, token, http.MethodDelete, "/api/provider-keys/gemini", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := doAuthed(t, h, token, http.MethodDelete, "/api/provider-keys/gemini", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", rec.Code)
	}
	if rec := doAuthed(t, h, token, http.MethodPut, "/api/provider-keys/nope", map[string]string{"apiKey": "x"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown provider = %d", rec.Code)
	}
	if rec := doAuthed(t, h, token, http.MethodPost, "/api/provider-keys/gemini/test", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("test without key = %d", rec.Code)
	}
}

func TestBYOKDisabled(t *testing.T) {
	h, token, _ := newBYOKServer(t, false)
	if rec := doAuthed(t, h, token, http.MethodPut, "/api/provider-keys/gemini", map[string]string{"apiKey": "x"}); rec.Code != http.StatusForbidden {
		t.Fatalf("disabled set = %d", rec.Code)
	}
	rec := doAuthed(t, h, token, http.MethodGet, "/api/provider-keys", nil)
	if rec.Code != http.StatusOK || !containsSecret(rec.Body.Bytes(), `"allowUserKeys":false`) {
		t.Fatalf("disabled list = %d %s", rec.Code, rec.Body.String())
	}
}

func containsSecret(raw []byte, secret string) bool {
	return len(secret) > 0 && json.Valid(raw) && stringContains(string(raw), secret)
}

func stringContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func providerConfigured(rows []struct {
	Provider   string `json:"provider"`
	Configured bool   `json:"configured"`
}, provider string) bool {
	for _, row := range rows {
		if row.Provider == provider {
			return row.Configured
		}
	}
	return false
}
