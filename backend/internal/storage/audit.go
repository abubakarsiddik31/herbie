package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ToolAuditLog struct {
	ID            string    `json:"id"`
	UserID        string    `json:"userId"`
	CallerType    string    `json:"callerType"` // chat_agent, workflow, manual, webhook
	CallerID      string    `json:"callerId"`   // conversation_id or workflow_id
	ToolName      string    `json:"toolName"`
	Action        string    `json:"action"` // execute, approval_granted, approval_rejected, oauth_connect
	InputSummary  string    `json:"inputSummary"`
	OutputSummary string    `json:"outputSummary"`
	Status        string    `json:"status"` // success, failed, approval_pending, rejected
	Error         *string   `json:"error,omitempty"`
	DurationMs    int64     `json:"durationMs"`
	CreatedAt     time.Time `json:"createdAt"`
}

type AuditStore interface {
	RecordToolAudit(ctx context.Context, log ToolAuditLog) error
	ListToolAudits(ctx context.Context, userID string, limit int) ([]ToolAuditLog, error)
}

type Audits struct {
	pool *pgxpool.Pool
}

func NewAudits(pool *pgxpool.Pool) *Audits {
	return &Audits{pool: pool}
}

const auditColumns = `id, user_id::text, caller_type, caller_id, tool_name, action,
	input_summary, output_summary, status, error, duration_ms, created_at`

func scanToolAuditLog(row pgx.Row) (ToolAuditLog, error) {
	var l ToolAuditLog
	err := row.Scan(
		&l.ID, &l.UserID, &l.CallerType, &l.CallerID, &l.ToolName, &l.Action,
		&l.InputSummary, &l.OutputSummary, &l.Status, &l.Error, &l.DurationMs, &l.CreatedAt,
	)
	return l, err
}

func (a *Audits) RecordToolAudit(ctx context.Context, log ToolAuditLog) error {
	if a == nil || a.pool == nil {
		return nil
	}
	if log.ID == "" {
		log.ID = uuid.NewString()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	if len(log.InputSummary) > 2000 {
		log.InputSummary = log.InputSummary[:2000]
	}
	if len(log.OutputSummary) > 2000 {
		log.OutputSummary = log.OutputSummary[:2000]
	}

	_, err := a.pool.Exec(ctx,
		`INSERT INTO tool_audit_logs (id, user_id, caller_type, caller_id, tool_name, action, input_summary, output_summary, status, error, duration_ms, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		log.ID, log.UserID, log.CallerType, log.CallerID, log.ToolName, log.Action,
		log.InputSummary, log.OutputSummary, log.Status, log.Error, log.DurationMs, log.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("record tool audit log: %w", err)
	}
	return nil
}

func (a *Audits) ListToolAudits(ctx context.Context, userID string, limit int) ([]ToolAuditLog, error) {
	if a == nil || a.pool == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := a.pool.Query(ctx,
		`SELECT `+auditColumns+` FROM tool_audit_logs WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list tool audit logs: %w", err)
	}
	defer rows.Close()

	var logs []ToolAuditLog
	for rows.Next() {
		l, err := scanToolAuditLog(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tool audit log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
