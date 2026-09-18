package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/auth/oauth"
)

const (
	oauthStateCookie = "oauth_state"
	oauthStateTTL    = 10 * time.Minute
)

func (s *Server) findOAuthProvider(id string) *oauth.Provider {
	for _, p := range s.deps.OAuth {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (s *Server) handleOAuthProviders(w http.ResponseWriter, _ *http.Request) {
	providers := make([]map[string]string, 0, len(s.deps.OAuth))
	for _, p := range s.deps.OAuth {
		providers = append(providers, map[string]string{"id": p.ID, "name": p.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	p := s.findOAuthProvider(r.PathValue("provider"))
	if p == nil {
		writeError(w, http.StatusNotFound, "oauth_unknown", "unknown oauth provider")
		return
	}
	state, err := oauth.NewState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not start oauth")
		return
	}
	verifier, challenge, err := oauth.NewPKCE()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not start oauth")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie,
		Value: oauth.Seal([]byte(s.deps.Cfg.JWTSecret), state, verifier,
			time.Now().Add(oauthStateTTL)),
		Path: "/api/auth", MaxAge: int(oauthStateTTL.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: s.isSecure(r),
	})
	http.Redirect(w, r, p.StartURL(state, challenge), http.StatusFound)
}

func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	p := s.findOAuthProvider(r.PathValue("provider"))
	if p == nil {
		writeError(w, http.StatusNotFound, "oauth_unknown", "unknown oauth provider")
		return
	}
	fail := func(reason string) {
		http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Path: "/api/auth", MaxAge: -1, Secure: s.isSecure(r)})
		http.Redirect(w, r, s.deps.Cfg.FrontendOrigin+"/auth/callback?error="+reason, http.StatusFound)
	}
	query := r.URL.Query()
	if query.Get("error") != "" {
		fail("denied")
		return
	}
	cookie, err := r.Cookie(oauthStateCookie)
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Path: "/api/auth", MaxAge: -1, Secure: s.isSecure(r)})
	if err != nil {
		fail("invalid_state")
		return
	}
	state, verifier, ok := oauth.Open([]byte(s.deps.Cfg.JWTSecret), cookie.Value, time.Now())
	if !ok || subtle.ConstantTimeCompare([]byte(state), []byte(query.Get("state"))) != 1 {
		fail("invalid_state")
		return
	}
	code := query.Get("code")
	if code == "" {
		fail("invalid_state")
		return
	}
	token, err := p.Exchange(r.Context(), code, verifier)
	if err != nil {
		fail("provider_error")
		return
	}
	prof, err := p.FetchProfile(r.Context(), token)
	if err != nil {
		fail("provider_error")
		return
	}
	if prof.Email == "" || !prof.Verified {
		fail("unverified_email")
		return
	}
	res, err := s.deps.Auth.OAuthLogin(r.Context(), p.ID, prof.Subject, prof.Email)
	if err != nil {
		if errors.Is(err, auth.ErrOAuthConflict) {
			fail("account_taken")
			return
		}
		fail("provider_error")
		return
	}
	handoff, err := s.deps.Auth.IssueOAuthCode(r.Context(), res.User.ID)
	if err != nil {
		fail("provider_error")
		return
	}
	http.Redirect(w, r, s.deps.Cfg.FrontendOrigin+"/auth/callback?code="+handoff, http.StatusFound)
}

func (s *Server) handleOAuthConsume(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "oauth code is required")
		return
	}
	res, err := s.deps.Auth.ConsumeOAuthCode(r.Context(), req.Code)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_code", "invalid or expired oauth code")
		return
	}
	s.setRefreshCookie(w, r, res)
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken": res.AccessToken,
		"user":        map[string]string{"id": res.User.ID, "email": res.User.Email},
	})
}
