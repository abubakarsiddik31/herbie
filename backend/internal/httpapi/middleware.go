package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
)

type statusResponseWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if !w.wrote {
		w.status = status
		w.wrote = true
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status, "dur", time.Since(start).String())
	})
}

type ctxKey int

const (
	userKey    ctxKey = 1
	roleKey    ctxKey = 2
	sessionKey ctxKey = 3
)

func withUser(ctx context.Context, userID string, role ...string) context.Context {
	r := "user"
	if len(role) > 0 && role[0] != "" {
		r = role[0]
	}
	ctx = context.WithValue(ctx, userKey, userID)
	return context.WithValue(ctx, roleKey, r)
}

func userIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userKey).(string)
	return id, ok
}

func withSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionKey, sessionID)
}

// sessionIDFrom returns the refresh-token session bound to the access token
// (empty for tokens minted before sessions existed).
func sessionIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(sessionKey).(string)
	return id
}

func userRoleFrom(ctx context.Context) string {
	role, ok := ctx.Value(roleKey).(string)
	if !ok || role == "" {
		return "user"
	}
	return role
}

// requireAuth validates the bearer token and puts the user ID and role on the
// request context for downstream handlers.
func requireAuth(tokens *auth.TokenMaker, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		token = strings.TrimSpace(token)
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
			return
		}
		userID, role, sessionID, err := tokens.VerifySession(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
			return
		}
		ctx := withUser(r.Context(), userID, role)
		next.ServeHTTP(w, r.WithContext(withSession(ctx, sessionID)))
	})
}

// requireRole enforces that the authenticated caller has the required role (e.g. "admin").
func requireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userRole := userRoleFrom(r.Context())
		if userRole != role {
			writeError(w, http.StatusForbidden, "forbidden", role+" role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withCORS is the outermost wrapper: it stamps CORS headers on every
// response (including error paths deeper in the stack) and answers
// preflights directly.
func withCORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
