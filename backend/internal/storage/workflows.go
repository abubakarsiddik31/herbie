package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Workflow struct {
	ID              string          `json:"id"`
	UserID          string          `json:"userId"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	TriggerType     string          `json:"triggerType"`
	WebhookSlug     *string         `json:"webhookSlug"`
	WebhookSecret   string          `json:"webhookSecret"`
	Nodes           json.RawMessage `json:"nodes"`
	Edges           json.RawMessage `json:"edges"`
	ExposeAsTool    bool            `json:"exposeAsTool"`
	ToolName        string          `json:"toolName"`
	ToolDescription string          `json:"toolDescription"`
	IsActive        bool            `json:"isActive"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

type WorkflowPatch struct {
	Name            *string
	Description     *string
	TriggerType     *string
	WebhookSlug     *string
	WebhookSecret   *string
	Nodes           *json.RawMessage
	Edges           *json.RawMessage
	ExposeAsTool    *bool
	ToolName        *string
	ToolDescription *string
	IsActive        *bool
}

type WorkflowRun struct {
	ID            string          `json:"id"`
	WorkflowID    string          `json:"workflowId"`
	UserID        string          `json:"userId"`
	Status        string          `json:"status"`        // pending, running, success, failed
	TriggerSource string          `json:"triggerSource"` // manual, webhook, schedule, chat
	InputData     json.RawMessage `json:"inputData"`
	OutputData    json.RawMessage `json:"outputData"`
	NodeResults   json.RawMessage `json:"nodeResults"`
	Error         *string         `json:"error"`
	DurationMs    int64           `json:"durationMs"`
	CreatedAt     time.Time       `json:"createdAt"`
	FinishedAt    *time.Time      `json:"finishedAt"`
}

type WorkflowCredential struct {
	ID        string          `json:"id"`
	UserID    string          `json:"userId"`
	Name      string          `json:"name"`
	Type      string          `json:"type"` // bearer_token, api_key, basic_auth, custom_header
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type Workflows struct {
	pool *pgxpool.Pool
}

func NewWorkflows(pool *pgxpool.Pool) *Workflows {
	return &Workflows{pool: pool}
}

const workflowColumns = `id, user_id::text, name, description, trigger_type, webhook_slug, webhook_secret,
	nodes, edges, expose_as_tool, tool_name, tool_description, is_active, created_at, updated_at`

func scanWorkflow(row pgx.Row) (Workflow, error) {
	var w Workflow
	err := row.Scan(
		&w.ID, &w.UserID, &w.Name, &w.Description, &w.TriggerType, &w.WebhookSlug, &w.WebhookSecret,
		&w.Nodes, &w.Edges, &w.ExposeAsTool, &w.ToolName, &w.ToolDescription, &w.IsActive,
		&w.CreatedAt, &w.UpdatedAt,
	)
	return w, err
}

func (s *Workflows) Create(ctx context.Context, w Workflow) (Workflow, error) {
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	if len(w.Nodes) == 0 {
		w.Nodes = json.RawMessage("[]")
	}
	if len(w.Edges) == 0 {
		w.Edges = json.RawMessage("[]")
	}
	if w.TriggerType == "" {
		w.TriggerType = "manual"
	}
	if w.WebhookSlug != nil && strings.TrimSpace(*w.WebhookSlug) == "" {
		w.WebhookSlug = nil
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO workflows (id, user_id, name, description, trigger_type, webhook_slug, webhook_secret,
		 nodes, edges, expose_as_tool, tool_name, tool_description, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING `+workflowColumns,
		w.ID, w.UserID, w.Name, w.Description, w.TriggerType, w.WebhookSlug, w.WebhookSecret,
		w.Nodes, w.Edges, w.ExposeAsTool, w.ToolName, w.ToolDescription, w.IsActive,
	)
	out, err := scanWorkflow(row)
	if err != nil {
		if isDuplicate(err) {
			return Workflow{}, ErrDuplicate
		}
		return Workflow{}, fmt.Errorf("create workflow: %w", err)
	}
	return out, nil
}

func (s *Workflows) Get(ctx context.Context, id, userID string) (Workflow, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+workflowColumns+` FROM workflows WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	out, err := scanWorkflow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, ErrNotFound
	}
	if err != nil {
		return Workflow{}, fmt.Errorf("get workflow: %w", err)
	}
	return out, nil
}

func (s *Workflows) GetByWebhookSlug(ctx context.Context, slug string) (Workflow, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+workflowColumns+` FROM workflows WHERE webhook_slug = $1 AND is_active = true`,
		slug,
	)
	out, err := scanWorkflow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, ErrNotFound
	}
	if err != nil {
		return Workflow{}, fmt.Errorf("get workflow by webhook: %w", err)
	}
	return out, nil
}

func (s *Workflows) List(ctx context.Context, userID string) ([]Workflow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+workflowColumns+` FROM workflows WHERE user_id = $1 ORDER BY updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	defer rows.Close()

	var out []Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan workflow: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Workflows) ListActiveTools(ctx context.Context, userID string) ([]Workflow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+workflowColumns+` FROM workflows
		 WHERE user_id = $1 AND is_active = true AND expose_as_tool = true
		 ORDER BY name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list active tools: %w", err)
	}
	defer rows.Close()

	var out []Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan active tool: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Workflows) Update(ctx context.Context, id, userID string, patch WorkflowPatch) (Workflow, error) {
	sets := []string{"updated_at = now()"}
	args := []any{id, userID}

	if patch.Name != nil {
		args = append(args, *patch.Name)
		sets = append(sets, fmt.Sprintf("name = $%d", len(args)))
	}
	if patch.Description != nil {
		args = append(args, *patch.Description)
		sets = append(sets, fmt.Sprintf("description = $%d", len(args)))
	}
	if patch.TriggerType != nil {
		args = append(args, *patch.TriggerType)
		sets = append(sets, fmt.Sprintf("trigger_type = $%d", len(args)))
	}
	if patch.WebhookSlug != nil {
		if strings.TrimSpace(*patch.WebhookSlug) == "" {
			patch.WebhookSlug = nil
		}
		args = append(args, patch.WebhookSlug)
		sets = append(sets, fmt.Sprintf("webhook_slug = $%d", len(args)))
	}
	if patch.WebhookSecret != nil {
		args = append(args, *patch.WebhookSecret)
		sets = append(sets, fmt.Sprintf("webhook_secret = $%d", len(args)))
	}
	if patch.Nodes != nil {
		args = append(args, *patch.Nodes)
		sets = append(sets, fmt.Sprintf("nodes = $%d", len(args)))
	}
	if patch.Edges != nil {
		args = append(args, *patch.Edges)
		sets = append(sets, fmt.Sprintf("edges = $%d", len(args)))
	}
	if patch.ExposeAsTool != nil {
		args = append(args, *patch.ExposeAsTool)
		sets = append(sets, fmt.Sprintf("expose_as_tool = $%d", len(args)))
	}
	if patch.ToolName != nil {
		args = append(args, *patch.ToolName)
		sets = append(sets, fmt.Sprintf("tool_name = $%d", len(args)))
	}
	if patch.ToolDescription != nil {
		args = append(args, *patch.ToolDescription)
		sets = append(sets, fmt.Sprintf("tool_description = $%d", len(args)))
	}
	if patch.IsActive != nil {
		args = append(args, *patch.IsActive)
		sets = append(sets, fmt.Sprintf("is_active = $%d", len(args)))
	}

	query := fmt.Sprintf(
		`UPDATE workflows SET %s WHERE id = $1 AND user_id = $2 RETURNING `+workflowColumns,
		strings.Join(sets, ", "),
	)
	row := s.pool.QueryRow(ctx, query, args...)
	out, err := scanWorkflow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, ErrNotFound
	}
	if err != nil {
		if isDuplicate(err) {
			return Workflow{}, ErrDuplicate
		}
		return Workflow{}, fmt.Errorf("update workflow: %w", err)
	}
	return out, nil
}

func (s *Workflows) Delete(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM workflows WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete workflow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// WorkflowRuns store methods

const workflowRunColumns = `id, workflow_id::text, user_id::text, status, trigger_source,
	input_data, output_data, node_results, error, duration_ms, created_at, finished_at`

func scanWorkflowRun(row pgx.Row) (WorkflowRun, error) {
	var r WorkflowRun
	err := row.Scan(
		&r.ID, &r.WorkflowID, &r.UserID, &r.Status, &r.TriggerSource,
		&r.InputData, &r.OutputData, &r.NodeResults, &r.Error, &r.DurationMs,
		&r.CreatedAt, &r.FinishedAt,
	)
	return r, err
}

func (s *Workflows) CreateRun(ctx context.Context, r WorkflowRun) (WorkflowRun, error) {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	if r.Status == "" {
		r.Status = "pending"
	}
	if len(r.InputData) == 0 {
		r.InputData = json.RawMessage("{}")
	}
	if len(r.OutputData) == 0 {
		r.OutputData = json.RawMessage("{}")
	}
	if len(r.NodeResults) == 0 {
		r.NodeResults = json.RawMessage("{}")
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO workflow_runs (id, workflow_id, user_id, status, trigger_source, input_data, output_data, node_results, error, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING `+workflowRunColumns,
		r.ID, r.WorkflowID, r.UserID, r.Status, r.TriggerSource, r.InputData, r.OutputData, r.NodeResults, r.Error, r.DurationMs,
	)
	out, err := scanWorkflowRun(row)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("create workflow run: %w", err)
	}
	return out, nil
}

func (s *Workflows) GetRun(ctx context.Context, id, userID string) (WorkflowRun, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+workflowRunColumns+` FROM workflow_runs WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	out, err := scanWorkflowRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkflowRun{}, ErrNotFound
	}
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("get workflow run: %w", err)
	}
	return out, nil
}

func (s *Workflows) ListRuns(ctx context.Context, workflowID, userID string, limit int) ([]WorkflowRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+workflowRunColumns+` FROM workflow_runs
		 WHERE workflow_id = $1 AND user_id = $2
		 ORDER BY created_at DESC LIMIT $3`,
		workflowID, userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list workflow runs: %w", err)
	}
	defer rows.Close()

	var out []WorkflowRun
	for rows.Next() {
		r, err := scanWorkflowRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan workflow run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Workflows) UpdateRun(ctx context.Context, r WorkflowRun) error {
	now := time.Now()
	if r.FinishedAt == nil && (r.Status == "success" || r.Status == "failed") {
		r.FinishedAt = &now
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE workflow_runs
		 SET status = $3, output_data = $4, node_results = $5, error = $6, duration_ms = $7, finished_at = $8
		 WHERE id = $1 AND user_id = $2`,
		r.ID, r.UserID, r.Status, r.OutputData, r.NodeResults, r.Error, r.DurationMs, r.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("update workflow run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// WorkflowCredentials store methods

const workflowCredentialColumns = `id, user_id::text, name, type, data, created_at, updated_at`

func scanWorkflowCredential(row pgx.Row) (WorkflowCredential, error) {
	var c WorkflowCredential
	err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Data, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Workflows) CreateCredential(ctx context.Context, c WorkflowCredential) (WorkflowCredential, error) {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if len(c.Data) == 0 {
		c.Data = json.RawMessage("{}")
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO workflow_credentials (id, user_id, name, type, data)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+workflowCredentialColumns,
		c.ID, c.UserID, c.Name, c.Type, c.Data,
	)
	out, err := scanWorkflowCredential(row)
	if err != nil {
		if isDuplicate(err) {
			return WorkflowCredential{}, ErrDuplicate
		}
		return WorkflowCredential{}, fmt.Errorf("create workflow credential: %w", err)
	}
	return out, nil
}

func (s *Workflows) GetCredential(ctx context.Context, id, userID string) (WorkflowCredential, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+workflowCredentialColumns+` FROM workflow_credentials WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	out, err := scanWorkflowCredential(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkflowCredential{}, ErrNotFound
	}
	if err != nil {
		return WorkflowCredential{}, fmt.Errorf("get workflow credential: %w", err)
	}
	return out, nil
}

func (s *Workflows) ListCredentials(ctx context.Context, userID string) ([]WorkflowCredential, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+workflowCredentialColumns+` FROM workflow_credentials WHERE user_id = $1 ORDER BY name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list workflow credentials: %w", err)
	}
	defer rows.Close()

	var out []WorkflowCredential
	for rows.Next() {
		c, err := scanWorkflowCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("scan workflow credential: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Workflows) UpdateCredential(ctx context.Context, c WorkflowCredential) (WorkflowCredential, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE workflow_credentials
		 SET name = $3, type = $4, data = $5, updated_at = now()
		 WHERE id = $1 AND user_id = $2
		 RETURNING `+workflowCredentialColumns,
		c.ID, c.UserID, c.Name, c.Type, c.Data,
	)
	out, err := scanWorkflowCredential(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkflowCredential{}, ErrNotFound
	}
	if err != nil {
		if isDuplicate(err) {
			return WorkflowCredential{}, ErrDuplicate
		}
		return WorkflowCredential{}, fmt.Errorf("update workflow credential: %w", err)
	}
	return out, nil
}

func (s *Workflows) DeleteCredential(ctx context.Context, id, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM workflow_credentials WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete workflow credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
