package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
)

func TestBotRoutinesLifecycle(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user, err := NewUsers(pool).Create(ctx, "bot-routines@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		user, err = NewUsers(pool).ByEmail(ctx, "bot-routines@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	bot, err := NewBots(pool).Create(ctx, Bot{UserID: user.ID, Name: "Herbie"})
	if err != nil {
		t.Fatalf("bot setup: %v", err)
	}

	store := NewBotRoutines(pool)
	first, err := store.Create(ctx, BotRoutine{
		BotID: bot.ID, UserID: user.ID, Name: "morning digest",
		Schedule: "0 8 * * *", Prompt: "Draft the morning digest.",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if first.ID == "" || first.Timezone != "UTC" || !first.Enabled || first.LastRunAt != nil {
		t.Fatalf("defaults not applied: %+v", first)
	}
	if first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", first)
	}

	// Get round-trips; a missing id maps to ErrNotFound.
	got, err := store.Get(ctx, user.ID, first.ID)
	if err != nil || got.Name != "morning digest" || got.Schedule != "0 8 * * *" || got.Prompt != "Draft the morning digest." {
		t.Fatalf("get: %v %+v", err, got)
	}
	zero := "00000000-0000-0000-0000-000000000000"
	if _, err := store.Get(ctx, zero, zero); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing: want ErrNotFound, got %v", err)
	}

	// ListByBot returns the bot's routines, newest first.
	second, err := store.Create(ctx, BotRoutine{
		BotID: bot.ID, UserID: user.ID, Name: "evening sweep",
		Schedule: "30 18 * * 1-5", Timezone: "Europe/Berlin", Prompt: "Sweep the inbox.",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.Timezone != "Europe/Berlin" {
		t.Fatalf("explicit timezone: %q", second.Timezone)
	}
	list, err := store.ListByBot(ctx, user.ID, bot.ID)
	if err != nil || len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("list by bot (newest first): %+v %v", list, err)
	}

	// Update applies only the patched fields; nil fields keep their values.
	name := "morning digest v2"
	updated, err := store.Update(ctx, user.ID, first.ID, BotRoutinePatch{Name: &name})
	if err != nil || updated.Name != "morning digest v2" || updated.Schedule != "0 8 * * *" ||
		updated.Timezone != "UTC" || updated.Prompt != "Draft the morning digest." || !updated.Enabled {
		t.Fatalf("partial update: %v %+v", err, updated)
	}
	schedule := "15 7 * * *"
	timezone := "Asia/Tokyo"
	prompt := "Draft the early digest."
	disable := false
	updated, err = store.Update(ctx, user.ID, first.ID, BotRoutinePatch{
		Schedule: &schedule, Timezone: &timezone, Prompt: &prompt, Enabled: &disable,
	})
	if err != nil || updated.Schedule != "15 7 * * *" || updated.Timezone != "Asia/Tokyo" ||
		updated.Prompt != "Draft the early digest." || updated.Enabled {
		t.Fatalf("full update: %v %+v", err, updated)
	}
	if _, err := store.Update(ctx, zero, zero, BotRoutinePatch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}

	// SetLastRun stamps the scheduler's high-water mark without touching the
	// routine's user-editable fields.
	stamp := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.SetLastRun(ctx, first.ID, stamp); err != nil {
		t.Fatalf("set last run: %v", err)
	}
	got, err = store.Get(ctx, user.ID, first.ID)
	if err != nil || got.LastRunAt == nil || !got.LastRunAt.Equal(stamp) {
		t.Fatalf("after set last run: %v %+v", err, got)
	}
	if err := store.SetLastRun(ctx, zero, stamp); !errors.Is(err, ErrNotFound) {
		t.Fatalf("set last run missing: want ErrNotFound, got %v", err)
	}

	// Delete removes the routine; a second delete maps to ErrNotFound.
	if err := store.Delete(ctx, user.ID, second.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, user.ID, second.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: want ErrNotFound, got %v", err)
	}
	if err := store.Delete(ctx, user.ID, second.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}

	// Cascade cleanup: deleting the bot wipes its routines (bot_routines.bot_id
	// ON DELETE CASCADE), so a re-run's global ListEnabled sees no leftovers.
	if err := NewBots(pool).Delete(ctx, user.ID, bot.ID); err != nil {
		t.Fatalf("bot cleanup: %v", err)
	}
}

func TestBotRoutinesListEnabled(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	userA, err := NewUsers(pool).Create(ctx, "bot-routines-a@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		userA, err = NewUsers(pool).ByEmail(ctx, "bot-routines-a@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("user a setup: %v", err)
	}
	userB, err := NewUsers(pool).Create(ctx, "bot-routines-b@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		userB, err = NewUsers(pool).ByEmail(ctx, "bot-routines-b@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("user b setup: %v", err)
	}
	botA, err := NewBots(pool).Create(ctx, Bot{UserID: userA.ID, Name: "Ada"})
	if err != nil {
		t.Fatalf("bot a setup: %v", err)
	}
	botB, err := NewBots(pool).Create(ctx, Bot{UserID: userB.ID, Name: "Bolt"})
	if err != nil {
		t.Fatalf("bot b setup: %v", err)
	}

	// One enabled and one disabled routine per user.
	store := NewBotRoutines(pool)
	onA, err := store.Create(ctx, BotRoutine{BotID: botA.ID, UserID: userA.ID, Name: "on A", Schedule: "0 9 * * *", Prompt: "go"})
	if err != nil {
		t.Fatalf("create on a: %v", err)
	}
	offA, err := store.Create(ctx, BotRoutine{BotID: botA.ID, UserID: userA.ID, Name: "off A", Schedule: "0 10 * * *", Prompt: "go"})
	if err != nil {
		t.Fatalf("create off a: %v", err)
	}
	onB, err := store.Create(ctx, BotRoutine{BotID: botB.ID, UserID: userB.ID, Name: "on B", Schedule: "0 11 * * *", Prompt: "go"})
	if err != nil {
		t.Fatalf("create on b: %v", err)
	}
	offB, err := store.Create(ctx, BotRoutine{BotID: botB.ID, UserID: userB.ID, Name: "off B", Schedule: "0 12 * * *", Prompt: "go"})
	if err != nil {
		t.Fatalf("create off b: %v", err)
	}
	disable := false
	if _, err := store.Update(ctx, userA.ID, offA.ID, BotRoutinePatch{Enabled: &disable}); err != nil {
		t.Fatalf("disable off a: %v", err)
	}
	if _, err := store.Update(ctx, userB.ID, offB.ID, BotRoutinePatch{Enabled: &disable}); err != nil {
		t.Fatalf("disable off b: %v", err)
	}

	// ListEnabled spans ALL users (the scheduler is per-process) and excludes
	// disabled routines. Assert membership rather than exact length so
	// leftovers from interrupted runs cannot flake the count.
	enabled, err := store.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	seen := make(map[string]bool)
	for _, routine := range enabled {
		seen[routine.ID] = true
		if routine.ID == offA.ID || routine.ID == offB.ID {
			t.Fatalf("disabled routine returned: %+v", routine)
		}
	}
	if !seen[onA.ID] || !seen[onB.ID] {
		t.Fatalf("enabled routines missing: %v", seen)
	}

	// Cascade cleanup: deleting the bots wipes both users' routines
	// (bot_routines.bot_id ON DELETE CASCADE), so a re-run's global ListEnabled
	// starts from an empty set.
	if err := NewBots(pool).Delete(ctx, userA.ID, botA.ID); err != nil {
		t.Fatalf("bot a cleanup: %v", err)
	}
	if err := NewBots(pool).Delete(ctx, userB.ID, botB.ID); err != nil {
		t.Fatalf("bot b cleanup: %v", err)
	}
}
