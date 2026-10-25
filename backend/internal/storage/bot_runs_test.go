package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/auth"
)

func TestBotRunsLifecycle(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user, err := NewUsers(pool).Create(ctx, "bot-runs@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		user, err = NewUsers(pool).ByEmail(ctx, "bot-runs@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	bot, err := NewBots(pool).Create(ctx, Bot{UserID: user.ID, Name: "Herbie"})
	if err != nil {
		t.Fatalf("bot setup: %v", err)
	}
	conv, err := NewConversations(pool).Create(ctx, user.ID, "run", ConversationPatch{})
	if err != nil {
		t.Fatalf("conversation setup: %v", err)
	}

	store := NewBotRuns(pool)
	first, err := store.Create(ctx, BotRun{BotID: bot.ID, ConversationID: conv.ID, UserID: user.ID, Task: "draft the weekly digest"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if first.ID == "" || first.Status != "queued" || first.TriggerSource != "manual" || first.CreatedAt == nil {
		t.Fatalf("defaults not applied: %+v", first)
	}
	if string(first.Activity) != "[]" {
		t.Fatalf("activity default: %q", first.Activity)
	}

	// Create→ClaimNext returns the oldest queued run with status running.
	claimed, err := store.ClaimNext(ctx)
	if err != nil || claimed == nil || claimed.ID != first.ID || claimed.BotID != bot.ID || claimed.Task != "draft the weekly digest" {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if claimed.Status != "running" {
		t.Fatalf("claimed status: %q", claimed.Status)
	}
	got, err := store.Get(ctx, user.ID, first.ID)
	if err != nil || got.Status != "running" || got.StartedAt == nil {
		t.Fatalf("get after claim: %v %+v", err, got)
	}

	// An empty queue returns (nil, nil), not an error.
	if again, err := store.ClaimNext(ctx); err != nil || again != nil {
		t.Fatalf("empty claim: %v %+v", err, again)
	}

	// SetFinished updates usage totals and stamps finished_at.
	if err := store.SetFinished(ctx, first.ID, "succeeded", 120, 80, 3, 1500, ""); err != nil {
		t.Fatalf("set finished: %v", err)
	}
	got, err = store.Get(ctx, user.ID, first.ID)
	if err != nil || got.Status != "succeeded" || got.InputTokens != 120 || got.OutputTokens != 80 ||
		got.Requests != 3 || got.CostMicroUsd != 1500 || got.FinishedAt == nil {
		t.Fatalf("after finish: %v %+v", err, got)
	}
	if err := store.SetFinished(ctx, "00000000-0000-0000-0000-000000000000", "failed", 0, 0, 0, 0, "boom"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("finish missing: want ErrNotFound, got %v", err)
	}

	// RecoverRunning flips running→queued and clears started_at.
	second, err := store.Create(ctx, BotRun{BotID: bot.ID, ConversationID: conv.ID, UserID: user.ID, Task: "routine sweep", TriggerSource: "routine"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.TriggerSource != "routine" {
		t.Fatalf("explicit trigger source: %q", second.TriggerSource)
	}
	if _, err := store.ClaimNext(ctx); err != nil {
		t.Fatalf("claim second: %v", err)
	}
	if err := store.RecoverRunning(ctx); err != nil {
		t.Fatalf("recover: %v", err)
	}
	got, err = store.Get(ctx, user.ID, second.ID)
	if err != nil || got.Status != "queued" || got.StartedAt != nil {
		t.Fatalf("after recover: %v %+v", err, got)
	}

	// Leave the recovered run finished so re-runs start from a clean queue.
	if requeued, err := store.ClaimNext(ctx); err != nil || requeued == nil || requeued.ID != second.ID {
		t.Fatalf("claim recovered: %v %+v", err, requeued)
	}
	if err := store.SetFinished(ctx, second.ID, "cancelled", 0, 0, 0, 0, ""); err != nil {
		t.Fatalf("cancel recovered: %v", err)
	}
}

func TestBotRunsActiveAndLatest(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user, err := NewUsers(pool).Create(ctx, "bot-runs-active@test.dev", "hash")
	if errors.Is(err, auth.ErrEmailTaken) {
		user, err = NewUsers(pool).ByEmail(ctx, "bot-runs-active@test.dev") // keep the test re-runnable
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	bot, err := NewBots(pool).Create(ctx, Bot{UserID: user.ID, Name: "Dot"})
	if err != nil {
		t.Fatalf("bot setup: %v", err)
	}
	convA, err := NewConversations(pool).Create(ctx, user.ID, "a", ConversationPatch{})
	if err != nil {
		t.Fatalf("conversation a setup: %v", err)
	}
	convB, err := NewConversations(pool).Create(ctx, user.ID, "b", ConversationPatch{})
	if err != nil {
		t.Fatalf("conversation b setup: %v", err)
	}

	store := NewBotRuns(pool)
	zero := "00000000-0000-0000-0000-000000000000"
	if _, err := store.Get(ctx, zero, zero); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing: want ErrNotFound, got %v", err)
	}

	// A queued run keeps the conversation active; a finished one does not.
	first, err := store.Create(ctx, BotRun{BotID: bot.ID, ConversationID: convA.ID, UserID: user.ID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if active, err := store.HasActiveForConversation(ctx, user.ID, convA.ID); err != nil || !active {
		t.Fatalf("queued active: %v %v", active, err)
	}
	if err := store.SetFinished(ctx, first.ID, "failed", 1, 2, 1, 9, "model overloaded"); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if active, err := store.HasActiveForConversation(ctx, user.ID, convA.ID); err != nil || active {
		t.Fatalf("finished inactive: %v %v", active, err)
	}

	// waiting_approval also counts as active; other conversations are untouched.
	second, err := store.Create(ctx, BotRun{BotID: bot.ID, ConversationID: convA.ID, UserID: user.ID})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if err := store.SetWaitingApproval(ctx, second.ID); err != nil {
		t.Fatalf("waiting approval: %v", err)
	}
	if err := store.SetWaitingApproval(ctx, zero); !errors.Is(err, ErrNotFound) {
		t.Fatalf("waiting approval missing: want ErrNotFound, got %v", err)
	}
	if active, err := store.HasActiveForConversation(ctx, user.ID, convA.ID); err != nil || !active {
		t.Fatalf("waiting active: %v %v", active, err)
	}
	if active, err := store.HasActiveForConversation(ctx, user.ID, convB.ID); err != nil || active {
		t.Fatalf("other conversation: %v %v", active, err)
	}

	// UpdateActivity overwrites the journal; LatestPerBot and ListByBot see the
	// newest run for the bot.
	if err := store.UpdateActivity(ctx, second.ID, []byte(`[{"step":"reading inbox"}]`)); err != nil {
		t.Fatalf("update activity: %v", err)
	}
	got, err := store.Get(ctx, user.ID, second.ID)
	if err != nil || string(got.Activity) != `[{"step": "reading inbox"}]` { // jsonb normalizes whitespace
		t.Fatalf("activity after update: %v %q", err, got.Activity)
	}
	third, err := store.Create(ctx, BotRun{BotID: bot.ID, ConversationID: convB.ID, UserID: user.ID})
	if err != nil {
		t.Fatalf("create third: %v", err)
	}
	latest, err := store.LatestPerBot(ctx, user.ID)
	if err != nil || len(latest) != 1 || latest[bot.ID].ID != third.ID {
		t.Fatalf("latest per bot: %v %v", latest, err)
	}
	list, err := store.ListByBot(ctx, user.ID, bot.ID, 2)
	if err != nil || len(list) != 2 || list[0].ID != third.ID || list[1].ID != second.ID {
		t.Fatalf("list by bot (newest first): %+v %v", list, err)
	}
	limited, err := store.ListByBot(ctx, user.ID, bot.ID, 1)
	if err != nil || len(limited) != 1 || limited[0].ID != third.ID {
		t.Fatalf("list limit: %+v %v", limited, err)
	}
}
