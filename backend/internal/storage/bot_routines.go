package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BotRoutine is a recurring schedule attached to an always-on bot: a cron
// expression, the timezone it is evaluated in, and the prompt fired on each
// tick. LastRunAt is the scheduler's high-water mark against double-firing.
type BotRoutine struct {
	ID        string
	BotID     string
	UserID    string
	Name      string
	Schedule  string
	Timezone  string
	Prompt    string
	Enabled   bool
	LastRunAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// BotRoutinePatch carries optional routine changes. Pointer fields left nil
// are left unchanged (BotPatch semantics).
type BotRoutinePatch struct {
	Name     *string
	Schedule *string
	Timezone *string
	Prompt   *string
	Enabled  *bool
}

type BotRoutines struct{ pool *pgxpool.Pool }

func NewBotRoutines(pool *pgxpool.Pool) *BotRoutines { return &BotRoutines{pool: pool} }

// last_run_at is nullable and scans straight into the *time.Time field, so —
// unlike bot_runs' error column — no COALESCE is needed here.
const botRoutineColumns = `id, bot_id, user_id, name, schedule, timezone, prompt, enabled, last_run_at, created_at, updated_at`

func scanBotRoutine(row interface{ Scan(...any) error }, routine *BotRoutine) error {
	return row.Scan(&routine.ID, &routine.BotID, &routine.UserID, &routine.Name, &routine.Schedule,
		&routine.Timezone, &routine.Prompt, &routine.Enabled, &routine.LastRunAt, &routine.CreatedAt, &routine.UpdatedAt)
}

func (r *BotRoutines) Create(ctx context.Context, routine BotRoutine) (BotRoutine, error) {
	// Zero-valued optional fields fall back to the schema defaults, the same
	// way Bots.Create applies its defaults in Go. A plain bool cannot express
	// "unspecified", so routines start enabled; callers disable via Update.
	if routine.Timezone == "" {
		routine.Timezone = "UTC"
	}
	routine.Enabled = true
	row := r.pool.QueryRow(ctx, `
		INSERT INTO bot_routines (bot_id, user_id, name, schedule, timezone, prompt, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING `+botRoutineColumns,
		routine.BotID, routine.UserID, routine.Name, routine.Schedule, routine.Timezone, routine.Prompt, routine.Enabled)
	if err := scanBotRoutine(row, &routine); err != nil {
		return BotRoutine{}, err
	}
	return routine, nil
}

func (r *BotRoutines) Get(ctx context.Context, userID, id string) (BotRoutine, error) {
	var routine BotRoutine
	if err := scanBotRoutine(r.pool.QueryRow(ctx,
		`SELECT `+botRoutineColumns+` FROM bot_routines WHERE id=$1 AND user_id=$2`, id, userID), &routine); err != nil {
		if errors.Is(err, pgxErrNoRows) {
			return BotRoutine{}, ErrNotFound
		}
		return BotRoutine{}, err
	}
	return routine, nil
}

func (r *BotRoutines) ListByBot(ctx context.Context, userID, botID string) ([]BotRoutine, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+botRoutineColumns+` FROM bot_routines
		WHERE bot_id=$1 AND user_id=$2 ORDER BY created_at DESC`, botID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BotRoutine
	for rows.Next() {
		var routine BotRoutine
		if err := scanBotRoutine(rows, &routine); err != nil {
			return nil, err
		}
		out = append(out, routine)
	}
	return out, rows.Err()
}

// ListEnabled returns every enabled routine across ALL users: the routine
// scheduler is per-process and ticks on behalf of everyone, so it must not be
// scoped to one user's rows.
func (r *BotRoutines) ListEnabled(ctx context.Context) ([]BotRoutine, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+botRoutineColumns+` FROM bot_routines
		WHERE enabled=true ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BotRoutine
	for rows.Next() {
		var routine BotRoutine
		if err := scanBotRoutine(rows, &routine); err != nil {
			return nil, err
		}
		out = append(out, routine)
	}
	return out, rows.Err()
}

func (r *BotRoutines) Update(ctx context.Context, userID, id string, p BotRoutinePatch) (BotRoutine, error) {
	routine, err := r.Get(ctx, userID, id)
	if err != nil {
		return BotRoutine{}, err
	}
	if p.Name != nil {
		routine.Name = *p.Name
	}
	if p.Schedule != nil {
		routine.Schedule = *p.Schedule
	}
	if p.Timezone != nil {
		routine.Timezone = *p.Timezone
	}
	if p.Prompt != nil {
		routine.Prompt = *p.Prompt
	}
	if p.Enabled != nil {
		routine.Enabled = *p.Enabled
	}
	row := r.pool.QueryRow(ctx, `
		UPDATE bot_routines SET name=$3, schedule=$4, timezone=$5, prompt=$6, enabled=$7, updated_at=now()
		WHERE id=$1 AND user_id=$2
		RETURNING `+botRoutineColumns,
		id, userID, routine.Name, routine.Schedule, routine.Timezone, routine.Prompt, routine.Enabled)
	if err := scanBotRoutine(row, &routine); err != nil {
		if errors.Is(err, pgxErrNoRows) {
			return BotRoutine{}, ErrNotFound
		}
		return BotRoutine{}, err
	}
	return routine, nil
}

func (r *BotRoutines) Delete(ctx context.Context, userID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM bot_routines WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetLastRun stamps the routine's last tick so the scheduler does not
// double-fire the same occurrence.
func (r *BotRoutines) SetLastRun(ctx context.Context, id string, at time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE bot_routines SET last_run_at=$2 WHERE id=$1`, id, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
