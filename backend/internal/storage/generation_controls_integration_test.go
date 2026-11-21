package storage

import (
	"context"
	"testing"
)

func TestGenerationControlsRoundTrip(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "generation@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "generation@test.dev")
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}

	top := 0.8
	if err := users.SetGenerationDefaults(ctx, user.ID, GenerationDefaults{TopP: &top, MaxOutputTokens: 4096, Reasoning: "medium"}); err != nil {
		t.Fatal(err)
	}
	defaults, err := users.GenerationDefaults(ctx, user.ID)
	if err != nil || defaults.TopP == nil || *defaults.TopP != 0.8 || defaults.MaxOutputTokens != 4096 || defaults.Reasoning != "medium" {
		t.Fatalf("user defaults = %+v %v", defaults, err)
	}

	projects := NewProjects(pool)
	project, err := projects.Create(ctx, Project{
		UserID: user.ID, Name: "generation project",
		DefaultTopP: float64Ptr(0.6), DefaultMaxOutputTokens: 2048, DefaultReasoning: "low",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = projects.Delete(context.Background(), project.ID, user.ID) })
	gotProject, err := projects.Get(ctx, project.ID, user.ID)
	if err != nil || gotProject.DefaultTopP == nil || *gotProject.DefaultTopP != 0.6 ||
		gotProject.DefaultMaxOutputTokens != 2048 || gotProject.DefaultReasoning != "low" {
		t.Fatalf("project defaults = %+v %v", gotProject, err)
	}

	convs := NewConversations(pool)
	maxTokens := 3000
	conv, err := convs.Create(ctx, user.ID, "generation chat", ConversationPatch{
		TopP: float64Ptr(0.5), MaxOutputTokens: &maxTokens, Reasoning: strPtr("high"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = convs.Delete(context.Background(), conv.ID, user.ID) })
	gotConv, err := convs.ByID(ctx, conv.ID, user.ID)
	if err != nil || gotConv.TopP == nil || *gotConv.TopP != 0.5 || gotConv.MaxOutputTokens != 3000 || gotConv.Reasoning != "high" {
		t.Fatalf("conversation controls = %+v %v", gotConv, err)
	}
	newTop := 0.2
	newMax := 1024
	newReasoning := "low"
	if err := convs.SetSettings(ctx, conv.ID, user.ID, ConversationPatch{
		TopP: &newTop, MaxOutputTokens: &newMax, Reasoning: &newReasoning,
	}); err != nil {
		t.Fatal(err)
	}
	gotConv, _ = convs.ByID(ctx, conv.ID, user.ID)
	if gotConv.TopP == nil || *gotConv.TopP != 0.2 || gotConv.MaxOutputTokens != 1024 || gotConv.Reasoning != "low" {
		t.Fatalf("conversation update = %+v", gotConv)
	}

	bots := NewBots(pool)
	bot, err := bots.Create(ctx, Bot{
		UserID: user.ID, Name: "generation bot",
		TopP: float64Ptr(0.7), MaxOutputTokens: 8192, Reasoning: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bots.Delete(context.Background(), user.ID, bot.ID) })
	gotBot, err := bots.Get(ctx, user.ID, bot.ID)
	if err != nil || gotBot.TopP == nil || *gotBot.TopP != 0.7 || gotBot.MaxOutputTokens != 8192 || gotBot.Reasoning != "high" {
		t.Fatalf("bot controls = %+v %v", gotBot, err)
	}
}

func float64Ptr(v float64) *float64 { return &v }
func strPtr(v string) *string       { return &v }
