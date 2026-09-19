package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
)

type UserAdminStore interface {
	List(ctx context.Context) ([]auth.UserRecord, error)
	SetRole(ctx context.Context, userID, role string) error
	Count(ctx context.Context) (int, error)
}

type adminUserDTO struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	if s.deps.Users == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "user management not configured")
		return
	}
	users, err := s.deps.Users.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list users")
		return
	}
	out := make([]adminUserDTO, len(users))
	for i, u := range users {
		created := ""
		if !u.CreatedAt.IsZero() {
			created = u.CreatedAt.UTC().Format(timeRFC3339)
		}
		role := u.Role
		if role == "" {
			role = "user"
		}
		out[i] = adminUserDTO{
			ID:        u.ID,
			Email:     u.Email,
			Role:      role,
			CreatedAt: created,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (s *Server) handleAdminUpdateUserRole(w http.ResponseWriter, r *http.Request) {
	if s.deps.Users == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "user management not configured")
		return
	}
	callerID, _ := userIDFrom(r.Context())
	targetID := r.PathValue("id")
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "user id is required")
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	if req.Role != "admin" && req.Role != "user" {
		writeError(w, http.StatusBadRequest, "bad_request", "role must be 'admin' or 'user'")
		return
	}

	// Safety check: Prevent admin lockout
	if callerID == targetID && req.Role != "admin" {
		users, err := s.deps.Users.List(r.Context())
		if err == nil {
			adminCount := 0
			for _, u := range users {
				if u.Role == "admin" {
					adminCount++
				}
			}
			if adminCount <= 1 {
				writeError(w, http.StatusBadRequest, "lockout_prevented", "cannot demote the sole administrator")
				return
			}
		}
	}

	if err := s.deps.Users.SetRole(r.Context(), targetID, req.Role); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not update user role")
		return
	}

	if s.deps.Audits != nil {
		_ = s.deps.Audits.RecordToolAudit(r.Context(), storage.ToolAuditLog{
			UserID:       callerID,
			CallerType:   "admin",
			CallerID:     callerID,
			ToolName:     "rbac",
			Action:       "update_role",
			InputSummary: "target=" + targetID + ", new_role=" + req.Role,
			Status:       "success",
			DurationMs:   0,
			CreatedAt:    time.Now(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":   targetID,
		"role": req.Role,
	})
}

func (s *Server) handleAdminListAudits(w http.ResponseWriter, r *http.Request) {
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

	logs, err := s.deps.Audits.ListAllToolAudits(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list audit logs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audits": logs})
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	totalUsers := 0
	if s.deps.Users != nil {
		if n, err := s.deps.Users.Count(r.Context()); err == nil {
			totalUsers = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"totalUsers": totalUsers,
	})
}
