package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BotRun is one execution of an always-on bot: created as queued, claimed by
// the runner (status running), then finished with usage totals. Activity is
// the runner's progress journal (jsonb array).
type BotRun struct {
	ID             string
	BotID          string
	ConversationID string
	UserID         string
	Status         string // queued | running | waiting_approval | succeeded | failed | cancelled
	Task           string
	TriggerSource  string // manual | routine | api
	Error          string
	Activity       []byte
	InputTokens    int
	OutputTokens   int
	Requests       int
	CostMicroUsd   int64
	CreatedAt      *time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
}

type BotRuns struct{ pool *pgxpool.Pool }

func NewBotRuns(pool *pgxpool.Pool) *BotRuns { return &BotRuns{pool: pool} }

// botRunColumns matches ClaimNext's scan (no lifecycle timestamps); the Full
// variant adds started_at/finished_at for reads. COALESCE keeps the nullable
// error scannable into the plain string field, per documentColumns.
const botRunColumns = `id, bot_id, conversation_id, user_id, status, task, trigger_source, COALESCE(error, ''), activity, input_tokens, output_tokens, requests, cost_micro_usd, created_at`

const botRunColumnsFull = botRunColumns + `, started_at, finished_at`

func scanBotRun(row interface{ Scan(...any) error }, run *BotRun) error {
	return row.Scan(&run.ID, &run.BotID, &run.ConversationID, &run.UserID, &run.Status, &run.Task,
		&run.TriggerSource, &run.Error, &run.Activity, &run.InputTokens, &run.OutputTokens,
		&run.Requests, &run.CostMicroUsd, &run.CreatedAt)
}

func scanBotRunFull(row interface{ Scan(...any) error }, run *BotRun) error {
	return row.Scan(&run.ID, &run.BotID, &run.ConversationID, &run.UserID, &run.Status, &run.Task,
		&run.TriggerSource, &run.Error, &run.Activity, &run.InputTokens, &run.OutputTokens,
		&run.Requests, &run.CostMicroUsd, &run.CreatedAt, &run.StartedAt, &run.FinishedAt)
}

func (r *BotRuns) Create(ctx context.Context, run BotRun) (BotRun, error) {
	// Zero-valued optional fields fall back to the schema defaults, the same
	// way Bots.Create applies its defaults in Go.
	if run.Status == "" {
		run.Status = "queued"
	}
	if run.TriggerSource == "" {
		run.TriggerSource = "manual"
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO bot_runs (bot_id, conversation_id, user_id, status, task, trigger_source)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING `+botRunColumns,
		run.BotID, run.ConversationID, run.UserID, run.Status, run.Task, run.TriggerSource)
	if err := scanBotRun(row, &run); err != nil {
		return BotRun{}, err
	}
	return run, nil
}

func (r *BotRuns) Get(ctx context.Context, userID, id string) (BotRun, error) {
	var run BotRun
	if err := scanBotRunFull(r.pool.QueryRow(ctx,
		`SELECT `+botRunColumnsFull+` FROM bot_runs WHERE id=$1 AND user_id=$2`, id, userID), &run); err != nil {
		if errors.Is(err, pgxErrNoRows) {
			return BotRun{}, ErrNotFound
		}
		return BotRun{}, err
	}
	return run, nil
}

func (r *BotRuns) ListByBot(ctx context.Context, userID, botID string, limit int) ([]BotRun, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+botRunColumnsFull+` FROM bot_runs
		WHERE bot_id=$1 AND user_id=$2 ORDER BY created_at DESC LIMIT $3`, botID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BotRun
	for rows.Next() {
		var run BotRun
		if err := scanBotRunFull(rows, &run); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// LatestPerBot returns the newest run per bot for the user, keyed by bot ID —
// the "last run" snapshot for bot cards.
func (r *BotRuns) LatestPerBot(ctx context.Context, userID string) (map[string]BotRun, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (bot_id) `+botRunColumnsFull+`
		FROM bot_runs WHERE user_id=$1
		ORDER BY bot_id, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]BotRun)
	for rows.Next() {
		var run BotRun
		if err := scanBotRunFull(rows, &run); err != nil {
			return nil, err
		}
		out[run.BotID] = run
	}
	return out, rows.Err()
}

// ClaimNext atomically takes the oldest queued run and marks it running.
// FOR UPDATE SKIP LOCKED keeps concurrent runners safe: rows already locked
// by another runner are skipped instead of blocking, and an empty queue
// returns (nil, nil) rather than an error.
func (r *BotRuns) ClaimNext(ctx context.Context) (*BotRun, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	run := &BotRun{}
	row := tx.QueryRow(ctx, `
		SELECT `+botRunColumns+`
		FROM bot_runs WHERE status='queued' ORDER BY created_at
		FOR UPDATE SKIP LOCKED LIMIT 1`)
	if err := scanBotRun(row, run); err != nil {
		if errors.Is(err, pgxErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE bot_runs SET status='running', started_at=now() WHERE id=$1`, run.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	run.Status = "running"
	return run, nil
}

// SetWaitingApproval parks a run whose tool call is awaiting user approval.
func (r *BotRuns) SetWaitingApproval(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE bot_runs SET status='waiting_approval' WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFinished records the terminal status, usage totals, and error, and
// stamps finished_at. runErr is stored as-is ("" for clean runs).
func (r *BotRuns) SetFinished(ctx context.Context, id, status string, inputTokens, outputTokens, requests int, costMicroUsd int64, runErr string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE bot_runs SET status=$2, error=$3, input_tokens=$4, output_tokens=$5, requests=$6, cost_micro_usd=$7, finished_at=now()
		WHERE id=$1`, id, status, runErr, inputTokens, outputTokens, requests, costMicroUsd)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateActivity overwrites the run's activity journal so the runner can
// stream progress without versioning.
func (r *BotRuns) UpdateActivity(ctx context.Context, id string, activity []byte) error {
	_, err := r.pool.Exec(ctx, `UPDATE bot_runs SET activity=$2 WHERE id=$1`, id, activity)
	return err
}

// RecoverRunning requeues runs stranded in running at startup (crashed
// runner) so they are claimed again; started_at is cleared because the
// previous start no longer describes a live attempt.
func (r *BotRuns) RecoverRunning(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `UPDATE bot_runs SET status='queued', started_at=NULL WHERE status='running'`)
	return err
}

// HasActiveForConversation reports whether the conversation still has a run
// queued, running, or waiting approval — used to keep one live run per bot
// conversation.
func (r *BotRuns) HasActiveForConversation(ctx context.Context, userID, conversationID string) (bool, error) {
	var active bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM bot_runs
			WHERE user_id=$1 AND conversation_id=$2 AND status IN ('queued','running','waiting_approval'))`,
		userID, conversationID).Scan(&active)
	return active, err
}
