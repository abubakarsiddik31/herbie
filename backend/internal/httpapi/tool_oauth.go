package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

const (
	toolOAuthCookie = "tool_oauth_state"
	toolOAuthTTL    = 15 * time.Minute
)

// sealToolOAuthState creates a signed tamper-proof state for tool OAuth connections.
func sealToolOAuthState(secret []byte, userID, provider, state string, exp time.Time) string {
	payload := strings.Join([]string{userID, provider, state, strconv.FormatInt(exp.Unix(), 10)}, "|")
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// openToolOAuthState verifies the signature and expiration of tool OAuth state.
func openToolOAuthState(secret []byte, sealed string, now time.Time) (userID, provider, state string, ok bool) {
	parts := strings.Split(sealed, ".")
	if len(parts) != 2 {
		return "", "", "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", "", false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	if subtle.ConstantTimeCompare(mac.Sum(nil), sig) != 1 {
		return "", "", "", false
	}
	fields := strings.Split(string(payload), "|")
	if len(fields) != 4 {
		return "", "", "", false
	}
	exp, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || now.Unix() > exp {
		return "", "", "", false
	}
	return fields[0], fields[1], fields[2], true
}

func (s *Server) toolOAuthRedirectBase(r *http.Request) string {
	if s.deps.Cfg.ToolOAuth.RedirectBase != "" {
		return strings.TrimRight(s.deps.Cfg.ToolOAuth.RedirectBase, "/")
	}
	if s.deps.Cfg.OAuth.RedirectBase != "" {
		return strings.TrimRight(s.deps.Cfg.OAuth.RedirectBase, "/")
	}
	scheme := "http"
	if s.isSecure(r) {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

type toolProviderDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Configured     bool     `json:"configured"`
	Connected      bool     `json:"connected"`
	CredentialName string   `json:"credentialName,omitempty"`
	Scopes         []string `json:"scopes,omitempty"`
	ConnectedAt    string   `json:"connectedAt,omitempty"`
	ExpiresAt      *string  `json:"expiresAt,omitempty"`
}

func (s *Server) handleToolOAuthProviders(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())

	providers := []toolProviderDTO{
		{
			ID:         "github",
			Name:       "GitHub",
			Configured: s.deps.Cfg.ToolOAuth.GitHub.Enabled(),
			Scopes:     []string{"repo", "read:user"},
		},
		{
			ID:         "slack",
			Name:       "Slack",
			Configured: s.deps.Cfg.ToolOAuth.Slack.Enabled(),
			Scopes:     []string{"incoming-webhook", "chat:write"},
		},
	}

	if s.deps.Workflows != nil && userID != "" {
		creds, _ := s.deps.Workflows.ListCredentials(r.Context(), userID)
		for i, p := range providers {
			for _, c := range creds {
				if c.Provider == p.ID || (c.Type == "oauth2" && strings.HasPrefix(c.Name, p.ID)) {
					providers[i].Connected = true
					providers[i].CredentialName = c.Name
					providers[i].ConnectedAt = c.UpdatedAt.UTC().Format(timeRFC3339)
					if len(c.Scopes) > 0 {
						providers[i].Scopes = c.Scopes
					}
					if c.ExpiresAt != nil {
						exp := c.ExpiresAt.UTC().Format(timeRFC3339)
						providers[i].ExpiresAt = &exp
					}
					break
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

func (s *Server) handleToolOAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	userID, _ := userIDFrom(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not generate state")
		return
	}
	state := hex.EncodeToString(stateBytes)

	redirectURI := fmt.Sprintf("%s/api/tool-oauth/%s/callback", s.toolOAuthRedirectBase(r), provider)
	var authURL string

	switch provider {
	case "github":
		cfg := s.deps.Cfg.ToolOAuth.GitHub
		if !cfg.Enabled() {
			writeError(w, http.StatusBadRequest, "not_configured", "GitHub OAuth is not configured")
			return
		}
		q := url.Values{
			"client_id":    {cfg.ClientID},
			"redirect_uri": {redirectURI},
			"scope":        {"repo,read:user"},
			"state":        {state},
		}
		authURL = "https://github.com/login/oauth/authorize?" + q.Encode()

	case "slack":
		cfg := s.deps.Cfg.ToolOAuth.Slack
		if !cfg.Enabled() {
			writeError(w, http.StatusBadRequest, "not_configured", "Slack OAuth is not configured")
			return
		}
		q := url.Values{
			"client_id":    {cfg.ClientID},
			"redirect_uri": {redirectURI},
			"scope":        {"incoming-webhook,chat:write"},
			"state":        {state},
		}
		authURL = "https://slack.com/oauth/v2/authorize?" + q.Encode()

	default:
		writeError(w, http.StatusNotFound, "unknown_provider", "unsupported oauth provider")
		return
	}

	sealed := sealToolOAuthState([]byte(s.deps.Cfg.JWTSecret), userID, provider, state, time.Now().Add(toolOAuthTTL))
	http.SetCookie(w, &http.Cookie{
		Name:     toolOAuthCookie,
		Value:    sealed,
		Path:     "/api/tool-oauth",
		MaxAge:   int(toolOAuthTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isSecure(r),
	})

	writeJSON(w, http.StatusOK, map[string]any{"url": authURL})
}

func (s *Server) handleToolOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	targetRedirect := fmt.Sprintf("%s/workflows?connected=%s", s.deps.Cfg.FrontendOrigin, provider)

	fail := func(reason string) {
		http.SetCookie(w, &http.Cookie{
			Name:     toolOAuthCookie,
			Path:     "/api/tool-oauth",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   s.isSecure(r),
		})
		http.Redirect(w, r, fmt.Sprintf("%s/workflows?error=%s", s.deps.Cfg.FrontendOrigin, url.QueryEscape(reason)), http.StatusFound)
	}

	cookie, err := r.Cookie(toolOAuthCookie)
	if err != nil {
		fail("missing_state_cookie")
		return
	}

	userID, expectedProvider, expectedState, ok := openToolOAuthState([]byte(s.deps.Cfg.JWTSecret), cookie.Value, time.Now())
	if !ok || expectedProvider != provider {
		fail("invalid_or_expired_state")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state != expectedState {
		fail("state_mismatch")
		return
	}

	redirectURI := fmt.Sprintf("%s/api/tool-oauth/%s/callback", s.toolOAuthRedirectBase(r), provider)
	client := &http.Client{Timeout: 15 * time.Second}

	var credData map[string]string
	var scopes []string
	var expiresAt *time.Time

	switch provider {
	case "github":
		cfg := s.deps.Cfg.ToolOAuth.GitHub
		payload, _ := json.Marshal(map[string]string{
			"client_id":     cfg.ClientID,
			"client_secret": cfg.Secret,
			"code":          code,
			"redirect_uri":  redirectURI,
		})
		req, err := http.NewRequestWithContext(r.Context(), "POST", "https://github.com/login/oauth/access_token", bytes.NewReader(payload))
		if err != nil {
			fail("request_error")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			fail("token_exchange_failed")
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var res struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			Scope       string `json:"scope"`
			Error       string `json:"error"`
		}
		if err := json.Unmarshal(body, &res); err != nil || res.AccessToken == "" || res.Error != "" {
			fail("token_error_" + res.Error)
			return
		}

		if res.Scope != "" {
			scopes = strings.Split(res.Scope, ",")
			for i := range scopes {
				scopes[i] = strings.TrimSpace(scopes[i])
			}
		}
		credData = map[string]string{
			"token": res.AccessToken,
		}

	case "slack":
		cfg := s.deps.Cfg.ToolOAuth.Slack
		form := url.Values{
			"client_id":     {cfg.ClientID},
			"client_secret": {cfg.Secret},
			"code":          {code},
			"redirect_uri":  {redirectURI},
		}
		req, err := http.NewRequestWithContext(r.Context(), "POST", "https://slack.com/api/oauth.v2.access", strings.NewReader(form.Encode()))
		if err != nil {
			fail("request_error")
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := client.Do(req)
		if err != nil {
			fail("token_exchange_failed")
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var res struct {
			Ok              bool   `json:"ok"`
			Error           string `json:"error"`
			AccessToken     string `json:"access_token"`
			Scope           string `json:"scope"`
			IncomingWebhook struct {
				URL string `json:"url"`
			} `json:"incoming_webhook"`
		}
		if err := json.Unmarshal(body, &res); err != nil || !res.Ok {
			fail("slack_error_" + res.Error)
			return
		}

		if res.Scope != "" {
			scopes = strings.Split(res.Scope, ",")
		}
		credData = map[string]string{
			"token": res.AccessToken,
		}
		if res.IncomingWebhook.URL != "" {
			credData["webhook_url"] = res.IncomingWebhook.URL
		}

	default:
		fail("unsupported_provider")
		return
	}

	// Encrypt credential with AES-GCM vault
	var rawData []byte
	rawBytes, _ := json.Marshal(credData)
	if s.deps.Vault != nil {
		enc, err := s.deps.Vault.Encrypt(rawBytes)
		if err != nil {
			fail("encryption_failed")
			return
		}
		rawData = enc
	} else {
		rawData = rawBytes
	}

	// Persist in workflow_credentials table
	if s.deps.Workflows != nil {
		credName := provider
		existing, err := s.deps.Workflows.GetCredentialByProvider(r.Context(), userID, provider)
		if err == nil && existing.ID != "" {
			existing.Name = credName
			existing.Type = "oauth2"
			existing.Provider = provider
			existing.Scopes = scopes
			existing.ExpiresAt = expiresAt
			existing.Data = rawData
			_, _ = s.deps.Workflows.UpdateCredential(r.Context(), existing)
		} else {
			_, _ = s.deps.Workflows.CreateCredential(r.Context(), storage.WorkflowCredential{
				UserID:    userID,
				Name:      credName,
				Type:      "oauth2",
				Provider:  provider,
				Scopes:    scopes,
				ExpiresAt: expiresAt,
				Data:      rawData,
			})
		}
	}

	// Record security audit log
	if s.deps.Audits != nil {
		_ = s.deps.Audits.RecordToolAudit(r.Context(), storage.ToolAuditLog{
			UserID:       userID,
			CallerType:   "manual",
			CallerID:     "",
			ToolName:     provider,
			Action:       "oauth_connect",
			InputSummary: fmt.Sprintf("scopes: %v", scopes),
			Status:       "success",
			DurationMs:   0,
			CreatedAt:    time.Now(),
		})
	}

	// Clear cookie and redirect back to frontend
	http.SetCookie(w, &http.Cookie{
		Name:     toolOAuthCookie,
		Path:     "/api/tool-oauth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.isSecure(r),
	})
	http.Redirect(w, r, targetRedirect, http.StatusFound)
}

func (s *Server) handleToolOAuthDisconnect(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	userID, _ := userIDFrom(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}

	if s.deps.Workflows == nil {
		writeError(w, http.StatusNotFound, "not_found", "workflows not configured")
		return
	}

	cred, err := s.deps.Workflows.GetCredentialByProvider(r.Context(), userID, provider)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "no connected credential found for "+provider)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not find credential")
		return
	}

	if err := s.deps.Workflows.DeleteCredential(r.Context(), cred.ID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not delete credential")
		return
	}

	if s.deps.Audits != nil {
		_ = s.deps.Audits.RecordToolAudit(r.Context(), storage.ToolAuditLog{
			UserID:       userID,
			CallerType:   "manual",
			ToolName:     provider,
			Action:       "oauth_disconnect",
			InputSummary: fmt.Sprintf("disconnected %s (%s)", cred.Name, provider),
			Status:       "success",
			DurationMs:   0,
			CreatedAt:    time.Now(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListToolAuditLogs(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}

	if s.deps.Audits == nil {
		writeJSON(w, http.StatusOK, map[string]any{"audits": []any{}})
		return
	}

	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if l, err := strconv.Atoi(q); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	logs, err := s.deps.Audits.ListToolAudits(r.Context(), userID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list audit logs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"audits": logs})
}
