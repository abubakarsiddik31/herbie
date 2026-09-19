package httpapi

import (
	"errors"
	"net/http"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
)

const refreshCookie = "refresh_token"

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	res, err := s.deps.Auth.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrEmailTaken):
			writeError(w, http.StatusConflict, "email_taken", "that email is already registered")
		case errors.Is(err, auth.ErrWeakPassword):
			writeError(w, http.StatusBadRequest, "weak_password", "password must be at least 10 characters")
		default:
			writeError(w, http.StatusInternalServerError, "internal", "registration failed")
		}
		return
	}
	s.setRefreshCookie(w, r, res)
	writeJSON(w, http.StatusCreated, map[string]any{
		"accessToken": res.AccessToken,
		"user":        map[string]string{"id": res.User.ID, "email": res.User.Email},
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	res, err := s.deps.Auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	s.setRefreshCookie(w, r, res)
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken": res.AccessToken,
		"user":        map[string]string{"id": res.User.ID, "email": res.User.Email},
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing refresh cookie")
		return
	}
	res, err := s.deps.Auth.Refresh(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "refresh rejected")
		return
	}
	s.setRefreshCookie(w, r, res)
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken": res.AccessToken,
		"user":        map[string]string{"id": res.User.ID, "email": res.User.Email},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookie); err == nil {
		_ = s.deps.Auth.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: "", Path: "/api/auth",
		MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.isSecure(r)})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := userIDFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
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
