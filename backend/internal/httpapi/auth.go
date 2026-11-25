package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/google/uuid"
)

const refreshCookie = "refresh_token"

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) sessionMeta(r *http.Request) auth.SessionMeta {
	return auth.SessionMeta{UserAgent: r.UserAgent(), IP: s.clientIP(r)}
}

// clientIP prefers the rightmost untrusted X-Forwarded-For hop when the
// direct peer is a configured trusted proxy; otherwise RemoteAddr.
func (s *Server) clientIP(r *http.Request) string {
	remote := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	if len(s.trustedProxies) == 0 {
		return remote
	}
	peer, err := netip.ParseAddr(remote)
	if err != nil || !prefixContains(s.trustedProxies, peer) {
		return remote
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" {
			continue
		}
		addr, err := netip.ParseAddr(candidate)
		if err != nil {
			return remote
		}
		if !prefixContains(s.trustedProxies, addr) {
			return candidate
		}
	}
	return remote
}

func prefixContains(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// allowAuth enforces the auth limiter and writes 429 when it trips. Keys
// cover the client IP plus, for account-scoped actions, the email.
func (s *Server) allowAuth(w http.ResponseWriter, r *http.Request, scope, email string) bool {
	ip := s.clientIP(r)
	keys := []string{scope + "|" + ip}
	if email != "" {
		// Keep the account key scoped to the caller's IP: a global per-email
		// bucket would let anyone lock a known account out of login/reset.
		keys = append(keys, scope+"|"+ip+"|email|"+strings.ToLower(strings.TrimSpace(email)))
	}
	ok, retry := s.authLimiter.Allow(keys...)
	if ok {
		return true
	}
	secs := int(retry.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts; try again later")
	return false
}

func (s *Server) authLink(path, token string) string {
	base := s.deps.Cfg.AppURL
	if base == "" {
		base = s.deps.Cfg.FrontendOrigin
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	// Fragments never reach the server, logs, or Referer headers.
	u.Fragment = "token=" + url.QueryEscape(token)
	return u.String()
}

func (s *Server) sendAuthEmail(ctx context.Context, to, subject, body string) {
	if s.deps.Mailer == nil {
		if s.deps.Log != nil {
			s.deps.Log.Info("auth email dropped: mailer not configured", "to", to, "subject", subject)
		}
		return
	}
	if err := s.deps.Mailer.Send(ctx, to, subject, body); err != nil && s.deps.Log != nil {
		s.deps.Log.Error("send auth email", "to", to, "subject", subject, "err", err)
	}
}

func (s *Server) sendVerificationEmail(ctx context.Context, to, token string) {
	link := s.authLink("/verify-email", token)
	if link == "" {
		return
	}
	s.sendAuthEmail(ctx, to, "Verify your Herbie email",
		"Confirm your email address to finish setting up your Herbie account:\n\n"+link+"\n\nThis link expires in 24 hours.")
}

func (s *Server) sendResetEmail(ctx context.Context, to, token string) {
	link := s.authLink("/reset-password", token)
	if link == "" {
		return
	}
	s.sendAuthEmail(ctx, to, "Reset your Herbie password",
		"Use this link to choose a new password:\n\n"+link+"\n\nThis link expires in 30 minutes. If you did not request it, ignore this email.")
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if !s.allowAuth(w, r, "register", req.Email) {
		return
	}
	res, err := s.deps.Auth.Register(r.Context(), req.Email, req.Password, s.sessionMeta(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrEmailTaken):
			writeError(w, http.StatusConflict, "email_taken", "that email is already registered")
		case errors.Is(err, auth.ErrInvalidEmail):
			writeError(w, http.StatusBadRequest, "invalid_email", "invalid email address format")
		case errors.Is(err, auth.ErrWeakPassword):
			writeError(w, http.StatusBadRequest, "weak_password", "password must be at least 10 characters")
		default:
			writeError(w, http.StatusInternalServerError, "internal", "registration failed")
		}
		return
	}
	role := res.User.Role
	if role == "" {
		role = "user"
	}
	if res.VerificationRequired {
		s.sendVerificationEmail(r.Context(), res.User.Email, res.VerificationToken)
		writeJSON(w, http.StatusCreated, map[string]any{
			"verificationRequired": true,
			"user":                 map[string]string{"id": res.User.ID, "email": res.User.Email, "role": role},
		})
		return
	}
	s.setRefreshCookie(w, r, res)
	writeJSON(w, http.StatusCreated, map[string]any{
		"accessToken": res.AccessToken,
		"user":        map[string]string{"id": res.User.ID, "email": res.User.Email, "role": role},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if !s.allowAuth(w, r, "login", req.Email) {
		return
	}
	res, err := s.deps.Auth.Login(r.Context(), req.Email, req.Password, s.sessionMeta(r))
	if err != nil {
		if errors.Is(err, auth.ErrEmailUnverified) {
			writeError(w, http.StatusForbidden, "email_unverified", "verify your email address before signing in")
			return
		}
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	role := res.User.Role
	if role == "" {
		role = "user"
	}
	s.setRefreshCookie(w, r, res)
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken": res.AccessToken,
		"user":        map[string]string{"id": res.User.ID, "email": res.User.Email, "role": role},
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing refresh cookie")
		return
	}
	res, err := s.deps.Auth.Refresh(r.Context(), cookie.Value, s.sessionMeta(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "refresh rejected")
		return
	}
	role := res.User.Role
	if role == "" {
		role = "user"
	}
	s.setRefreshCookie(w, r, res)
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken": res.AccessToken,
		"user":        map[string]string{"id": res.User.ID, "email": res.User.Email, "role": role},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookie); err == nil {
		_ = s.deps.Auth.Logout(r.Context(), cookie.Value)
	}
	s.clearRefreshCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if !s.allowAuth(w, r, "forgot", req.Email) {
		return
	}
	token, to, err := s.deps.Auth.IssuePasswordReset(r.Context(), req.Email)
	if err != nil && s.deps.Log != nil {
		s.deps.Log.Error("issue password reset", "err", err)
	}
	if token != "" {
		s.sendResetEmail(r.Context(), to, token)
	}
	// Always the same answer: never reveal whether the address exists.
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if !s.allowAuth(w, r, "reset", "") {
		return
	}
	err := s.deps.Auth.ResetPassword(r.Context(), req.Token, req.Password)
	switch {
	case errors.Is(err, auth.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "weak_password", "password must be at least 10 characters")
	case errors.Is(err, auth.ErrInvalidToken):
		writeError(w, http.StatusBadRequest, "invalid_token", "this reset link is invalid or has expired")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", "could not reset password")
	default:
		s.clearRefreshCookie(w, r)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if !s.allowAuth(w, r, "verify", "") {
		return
	}
	if err := s.deps.Auth.VerifyEmail(r.Context(), req.Token); err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			writeError(w, http.StatusBadRequest, "invalid_token", "this verification link is invalid or has expired")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not verify email")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if !s.allowAuth(w, r, "resend", req.Email) {
		return
	}
	token, to, err := s.deps.Auth.IssueEmailVerification(r.Context(), req.Email)
	if err != nil && s.deps.Log != nil {
		s.deps.Log.Error("issue verification", "err", err)
	}
	if token != "" {
		s.sendVerificationEmail(r.Context(), to, token)
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

type sessionJSON struct {
	ID         string    `json:"id"`
	UserAgent  string    `json:"userAgent"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	Current    bool      `json:"current"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	current := sessionIDFrom(r.Context())
	sessions, err := s.deps.Auth.Sessions(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list sessions")
		return
	}
	out := make([]sessionJSON, len(sessions))
	for i, sess := range sessions {
		out[i] = sessionJSON{
			ID: sess.ID, UserAgent: sess.UserAgent, IP: sess.IP,
			CreatedAt: sess.CreatedAt, LastUsedAt: sess.LastUsedAt,
			Current: sess.ID == current,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (s *Server) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	sessionID := r.PathValue("id")
	if _, err := uuid.Parse(sessionID); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid session id")
		return
	}
	if err := s.deps.Auth.RevokeSession(r.Context(), userID, sessionID); err != nil {
		if errors.Is(err, auth.ErrSessionNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not revoke session")
		return
	}
	if sessionID == sessionIDFrom(r.Context()) {
		s.clearRefreshCookie(w, r)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	if err := s.deps.Auth.RevokeAllSessions(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not revoke sessions")
		return
	}
	s.clearRefreshCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := userIDFrom(r.Context())
	role := userRoleFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "role": role})
}

// setRefreshCookie sends the opaque refresh token as an HttpOnly cookie
// scoped to the auth endpoints; the access token goes in the JSON body.
func (s *Server) setRefreshCookie(w http.ResponseWriter, r *http.Request, res auth.AuthResult) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    res.RefreshToken,
		Path:     "/api/auth",
		Expires:  res.RefreshExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isSecure(r),
	})
}

func (s *Server) clearRefreshCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: "", Path: "/api/auth",
		MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.isSecure(r)})
}
