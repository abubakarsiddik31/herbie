package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

// ProviderKeyStore stores vault-encrypted per-user provider keys; the key
// material never leaves this layer.
type ProviderKeyStore interface {
	Upsert(ctx context.Context, userID, provider string, encrypted []byte) error
	Get(ctx context.Context, userID, provider string) ([]byte, error)
	List(ctx context.Context, userID string) ([]storage.UserProviderKey, error)
	Delete(ctx context.Context, userID, provider string) error
}

var _ ProviderKeyStore = (*storage.UserProviderKeys)(nil)

var byokProviders = []string{"gemini", "openai", "anthropic"}

func validProvider(provider string) bool {
	for _, p := range byokProviders {
		if p == provider {
			return true
		}
	}
	return false
}

func (s *Server) handleListProviderKeys(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	configured := map[string]bool{}
	if s.deps.Cfg.AllowUserProviderKeys && s.deps.UserProviderKeys != nil {
		rows, err := s.deps.UserProviderKeys.List(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not load provider keys")
			return
		}
		for _, row := range rows {
			configured[row.Provider] = true
		}
	}
	out := make([]map[string]any, 0, len(byokProviders))
	for _, p := range byokProviders {
		out = append(out, map[string]any{"provider": p, "configured": configured[p]})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"allowUserKeys": s.deps.Cfg.AllowUserProviderKeys,
		"providers":     out,
	})
}

func (s *Server) requireBYOK(w http.ResponseWriter, r *http.Request) bool {
	if !s.deps.Cfg.AllowUserProviderKeys {
		writeError(w, http.StatusForbidden, "byok_disabled", "user provider keys are disabled")
		return false
	}
	if s.deps.UserProviderKeys == nil || s.deps.Vault == nil {
		writeError(w, http.StatusNotImplemented, "byok_unavailable", "user provider keys are not configured")
		return false
	}
	if !validProvider(r.PathValue("provider")) {
		writeError(w, http.StatusBadRequest, "bad_request", "unknown provider")
		return false
	}
	return true
}

func (s *Server) handleSetProviderKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireBYOK(w, r) {
		return
	}
	userID, _ := userIDFrom(r.Context())
	var req struct {
		APIKey string `json:"apiKey"`
	}
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.APIKey) == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "apiKey is required")
		return
	}
	encrypted, err := s.deps.Vault.Encrypt([]byte(strings.TrimSpace(req.APIKey)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store provider key")
		return
	}
	provider := r.PathValue("provider")
	if err := s.deps.UserProviderKeys.Upsert(r.Context(), userID, provider, encrypted); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store provider key")
		return
	}
	s.auditProviderKey(r.Context(), userID, provider, "set", "success", "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteProviderKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireBYOK(w, r) {
		return
	}
	userID, _ := userIDFrom(r.Context())
	provider := r.PathValue("provider")
	err := s.deps.UserProviderKeys.Delete(r.Context(), userID, provider)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "provider key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not delete provider key")
		return
	}
	s.auditProviderKey(r.Context(), userID, provider, "delete", "success", "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestProviderKey(w http.ResponseWriter, r *http.Request) {
	if !s.requireBYOK(w, r) {
		return
	}
	userID, _ := userIDFrom(r.Context())
	provider := r.PathValue("provider")
	encrypted, err := s.deps.UserProviderKeys.Get(r.Context(), userID, provider)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "provider key not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load provider key")
		return
	}
	plain, err := s.deps.Vault.Decrypt(encrypted)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not decrypt provider key")
		return
	}
	keys := chat.ProviderKeys{}
	switch provider {
	case "gemini":
		keys.Gemini = string(plain)
	case "openai":
		keys.OpenAI = string(plain)
	case "anthropic":
		keys.Anthropic = string(plain)
	}
	var target string
	for _, m := range chat.Available(keys) {
		if m.Provider == provider {
			target = m.ID
			break
		}
	}
	if target == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "no "+provider+" model is available")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	client, err := chat.NewModelRegistry(keys).Resolve(chat.RunSpec{Model: target})
	if err == nil {
		_, err = client.Generate(ctx, model.Request{Messages: []model.Message{
			{Role: model.RoleUser, Content: "Reply with the single word OK."},
		}})
	}
	if err != nil {
		s.auditProviderKey(r.Context(), userID, provider, "test", "failed", err.Error())
		writeError(w, http.StatusBadRequest, "provider_key_invalid", "key test failed: "+err.Error())
		return
	}
	s.auditProviderKey(r.Context(), userID, provider, "test", "success", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) auditProviderKey(ctx context.Context, userID, provider, action, status, errText string) {
	if s.deps.Audits == nil {
		return
	}
	var errPtr *string
	if errText != "" {
		errPtr = &errText
	}
	_ = s.deps.Audits.RecordToolAudit(ctx, storage.ToolAuditLog{
		UserID: userID, CallerType: "byok", ToolName: "provider_key_" + provider,
		Action: action, Status: status, Error: errPtr, CreatedAt: time.Now(),
	})
}

// providerKeysFor merges the user's decrypted BYOK keys over the server keys.
func (s *Server) providerKeysFor(ctx context.Context, userID string) chat.ProviderKeys {
	keys := s.deps.ModelKeys
	if !s.deps.Cfg.AllowUserProviderKeys || s.deps.UserProviderKeys == nil || s.deps.Vault == nil {
		return keys
	}
	rows, err := s.deps.UserProviderKeys.List(ctx, userID)
	if err != nil {
		return keys
	}
	for _, row := range rows {
		plain, err := s.deps.Vault.Decrypt(row.Encrypted)
		if err != nil {
			s.deps.Log.Warn("could not decrypt user provider key", "provider", row.Provider, "err", err)
			continue
		}
		switch row.Provider {
		case "gemini":
			keys.Gemini = string(plain)
		case "openai":
			keys.OpenAI = string(plain)
		case "anthropic":
			keys.Anthropic = string(plain)
		}
	}
	return keys
}

// agentFor returns the server agent, or a per-user copy when the user has BYOK
// keys and the feature is enabled.
func (s *Server) agentFor(ctx context.Context, userID string) *chat.Agent {
	if !s.deps.Cfg.AllowUserProviderKeys || s.deps.UserProviderKeys == nil || s.deps.Vault == nil {
		return s.deps.Agent
	}
	rows, err := s.deps.UserProviderKeys.List(ctx, userID)
	if err != nil || len(rows) == 0 {
		return s.deps.Agent
	}
	keys := s.providerKeysFor(ctx, userID)
	return s.deps.Agent.WithRegistry(chat.NewModelRegistry(keys))
}
