// Package oauth speaks OAuth 2.0 authorization-code + PKCE to social
// providers with the standard library only. Endpoint fields stay mutable
// so tests can point a provider at a local stub IdP.
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Profile struct {
	Subject  string
	Email    string
	Verified bool
}

type Endpoints struct {
	AuthURL    string
	TokenURL   string
	ProfileURL string
	EmailsURL  string
}

type Provider struct {
	ID         string
	Name       string
	ClientID   string
	Secret     string
	Redirect   string
	Scopes     []string
	Endpoints  Endpoints
	HTTPClient *http.Client
}

func Google(clientID, secret, redirect string) *Provider {
	return &Provider{
		ID: "google", Name: "Google",
		ClientID: clientID, Secret: secret, Redirect: redirect,
		Scopes: []string{"openid", "email", "profile"},
		Endpoints: Endpoints{
			AuthURL:    "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:   "https://oauth2.googleapis.com/token",
			ProfileURL: "https://openidconnect.googleapis.com/v1/userinfo",
		},
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func GitHub(clientID, secret, redirect string) *Provider {
	return &Provider{
		ID: "github", Name: "GitHub",
		ClientID: clientID, Secret: secret, Redirect: redirect,
		Scopes: []string{"read:user", "user:email"},
		Endpoints: Endpoints{
			AuthURL:    "https://github.com/login/oauth/authorize",
			TokenURL:   "https://github.com/login/oauth/access_token",
			ProfileURL: "https://api.github.com/user",
			EmailsURL:  "https://api.github.com/user/emails",
		},
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *Provider) StartURL(state, challenge string) string {
	q := url.Values{
		"client_id":             {p.ClientID},
		"redirect_uri":          {p.Redirect},
		"response_type":         {"code"},
		"scope":                 {strings.Join(p.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return p.Endpoints.AuthURL + "?" + q.Encode()
}

func (p *Provider) Exchange(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {p.ClientID},
		"client_secret": {p.Secret},
		"code":          {code},
		"redirect_uri":  {p.Redirect},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoints.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := p.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth token exchange: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("oauth token exchange: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oauth token exchange: status %d", res.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("oauth token exchange: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("oauth token exchange: %s", tok.Error)
	}
	return tok.AccessToken, nil
}

func (p *Provider) FetchProfile(ctx context.Context, token string) (Profile, error) {
	if p.ID == "github" {
		return p.fetchGitHubProfile(ctx, token)
	}
	return p.fetchOIDCProfile(ctx, token)
}

func (p *Provider) get(ctx context.Context, endpoint, token string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	res, err := p.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("oauth profile: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("oauth profile: status %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(v); err != nil {
		return fmt.Errorf("oauth profile: %w", err)
	}
	return nil
}

func (p *Provider) fetchOIDCProfile(ctx context.Context, token string) (Profile, error) {
	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := p.get(ctx, p.Endpoints.ProfileURL, token, &info); err != nil {
		return Profile{}, err
	}
	if info.Sub == "" {
		return Profile{}, fmt.Errorf("oauth profile: missing subject")
	}
	return Profile{Subject: info.Sub, Email: info.Email, Verified: info.EmailVerified}, nil
}

func (p *Provider) fetchGitHubProfile(ctx context.Context, token string) (Profile, error) {
	var user struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	}
	if err := p.get(ctx, p.Endpoints.ProfileURL, token, &user); err != nil {
		return Profile{}, err
	}
	if user.ID == 0 {
		return Profile{}, fmt.Errorf("oauth profile: missing subject")
	}
	prof := Profile{Subject: fmt.Sprintf("%d", user.ID), Email: user.Email}
	if p.Endpoints.EmailsURL != "" {
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := p.get(ctx, p.Endpoints.EmailsURL, token, &emails); err != nil {
			return Profile{}, err
		}
		for _, e := range emails {
			if e.Primary {
				prof.Email, prof.Verified = e.Email, e.Verified
			}
		}
	}
	return prof, nil
}

func NewState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("oauth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func NewPKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("oauth pkce: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
