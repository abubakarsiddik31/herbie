package storage

import (
	"context"
	"errors"
	"testing"
)

func TestConversationSearch(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	convs := NewConversations(pool)
	user, err := NewUsers(pool).Create(ctx, "search@test.dev", "hash")
	if err != nil {
		t.Skipf("user setup: %v", err)
	}
	for _, title := range []string{"Moon landing plans", "Grocery list", "MOONSHOT ideas"} {
		if _, err := convs.Create(ctx, user.ID, title, ConversationPatch{}); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	all, err := convs.List(ctx, user.ID, "", false, false)
	if err != nil || len(all) < 3 {
		t.Fatalf("unfiltered list: %+v %v", all, err)
	}
	moon, err := convs.List(ctx, user.ID, "moon", false, false)
	if err != nil || len(moon) != 2 {
		t.Fatalf("case-insensitive title search: %+v %v", moon, err)
	}
	none, err := convs.List(ctx, user.ID, "zzz-no-match", false, false)
	if err != nil || len(none) != 0 {
		t.Fatalf("empty search: %+v %v", none, err)
	}
	literal, err := convs.List(ctx, user.ID, "%", false, false)
	if err != nil || len(literal) != 0 {
		t.Fatalf("metacharacters must match literally: %+v %v", literal, err)
	}
	foreign, err := convs.List(ctx, "00000000-0000-0000-0000-000000000000", "moon", false, false)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("cross-user leak: %+v %v", foreign, err)
	}
}

func TestNullByteSanitizationIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	convs := NewConversations(pool)
	msgs := NewMessages(pool)

	user, err := users.Create(ctx, "nullbyte@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "nullbyte@test.dev")
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}

	conv, err := convs.Create(ctx, user.ID, "Conv\x00Title", ConversationPatch{})
	if err != nil {
		t.Fatalf("create conv with null byte: %v", err)
	}
	if conv.Title != "ConvTitle" {
		t.Fatalf("expected title without null byte, got %q", conv.Title)
	}

	if err := convs.RenameTitle(ctx, conv.ID, user.ID, "Updated\x00Title"); err != nil {
		t.Fatalf("set title with null byte: %v", err)
	}

	// Insert message containing null bytes in content and data
	msg := Message{
		ConversationID: conv.ID,
		UserID:         user.ID,
		Role:           "user",
		Content:        "Hello\x00World from PDF extraction!",
		Data:           []byte(`{"role":"user","content":"Hello\u0000World from PDF extraction!"}`),
		Model:          "test\x00model",
	}
	if _, err := msgs.Add(ctx, msg); err != nil {
		t.Fatalf("msgs.Add with null bytes failed: %v", err)
	}

	list, err := msgs.ForConversation(ctx, conv.ID, user.ID)
	if err != nil || len(list) == 0 {
		t.Fatalf("messages lookup: %v", err)
	}
	if list[0].Content != "HelloWorld from PDF extraction!" {
		t.Fatalf("expected sanitized content, got %q", list[0].Content)
	}
}

func TestConversationOrganizationIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	convs := NewConversations(pool)
	users := NewUsers(pool)
	user, err := users.Create(ctx, "organization@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "organization@test.dev")
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	other, err := users.Create(ctx, "organization-other@test.dev", "hash")
	if err != nil {
		other, err = users.ByEmail(ctx, "organization-other@test.dev")
	}
	if err != nil {
		t.Fatalf("other user setup: %v", err)
	}
	// Keep the test re-runnable against the shared integration database.
	if _, err := pool.Exec(ctx, `DELETE FROM conversations WHERE user_id = $1`, user.ID); err != nil {
		t.Fatalf("clean conversations: %v", err)
	}

	plain, err := convs.Create(ctx, user.ID, "Plain moon", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := convs.Create(ctx, user.ID, "Pinned moon", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := convs.Create(ctx, user.ID, "Archived moon", ConversationPatch{})
	if err != nil {
		t.Fatal(err)
	}
	if plain.TitleSource != "heuristic" {
		t.Fatalf("new conversation title source = %q, want heuristic", plain.TitleSource)
	}
	manual := "manual"
	if conv, err := convs.Create(ctx, user.ID, "Manual title", ConversationPatch{TitleSource: &manual}); err != nil || conv.TitleSource != "manual" {
		t.Fatalf("manual create = %+v %v", conv, err)
	}

	if err := convs.RenameTitle(ctx, plain.ID, user.ID, "Renamed moon"); err != nil {
		t.Fatal(err)
	}
	if got, _ := convs.ByID(ctx, plain.ID, user.ID); got.TitleSource != "manual" || got.Title != "Renamed moon" {
		t.Fatalf("rename = %+v", got)
	}
	if err := convs.SetHeuristicTitle(ctx, plain.ID, user.ID, "Overwritten"); err != nil {
		t.Fatal(err)
	}
	if got, _ := convs.ByID(ctx, plain.ID, user.ID); got.Title != "Renamed moon" {
		t.Fatalf("heuristic overwrote a manual title: %q", got.Title)
	}

	if err := convs.SetPinned(ctx, pinned.ID, user.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := convs.SetArchived(ctx, archived.ID, user.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := convs.SetArchived(ctx, plain.ID, user.ID, true); err != nil {
		t.Fatal(err)
	}

	active, err := convs.List(ctx, user.ID, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 {
		t.Fatalf("default list = %+v", active)
	}
	for _, c := range active {
		if c.ArchivedAt != nil {
			t.Fatalf("default list leaked archived row %+v", c)
		}
	}
	pinnedRows, err := convs.List(ctx, user.ID, "", true, false)
	if err != nil || len(pinnedRows) != 1 || pinnedRows[0].ID != pinned.ID || pinnedRows[0].PinnedAt == nil {
		t.Fatalf("pinned list = %+v %v", pinnedRows, err)
	}
	archivedRows, err := convs.List(ctx, user.ID, "", false, true)
	if err != nil || len(archivedRows) != 2 {
		t.Fatalf("archived list = %+v %v", archivedRows, err)
	}
	moon, err := convs.List(ctx, user.ID, "moon", false, false)
	if err != nil || len(moon) != 1 || moon[0].ID != pinned.ID {
		t.Fatalf("q must exclude archived rows: %+v %v", moon, err)
	}

	if err := convs.SetPinned(ctx, plain.ID, other.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign pin = %v", err)
	}
	if err := convs.SetArchived(ctx, "00000000-0000-0000-0000-000000000000", user.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown archive = %v", err)
	}
	if err := convs.RenameTitle(ctx, plain.ID, other.ID, "Hijack"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign rename = %v", err)
	}
	if err := convs.SetSettings(ctx, plain.ID, other.ID, ConversationPatch{Model: &manual}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign settings = %v", err)
	}
}

// TestArchivedConversationsHiddenFromProjectAndBotSurfaces covers the review
// finding that archiving only filtered the main sidebar list: project trees,
// bot lists, and the bot's latest-conversation lookup must hide archived rows
// (routines then start a fresh chat instead of appending to a hidden one).
func TestArchivedConversationsHiddenFromProjectAndBotSurfaces(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "archive-surfaces@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "archive-surfaces@test.dev")
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})
	convs := NewConversations(pool)
	project, err := NewProjects(pool).Create(ctx, Project{UserID: user.ID, Name: "P"})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := NewBots(pool).Create(ctx, Bot{UserID: user.ID, Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	pid, bid := project.ID, bot.ID
	projectLive, _ := convs.Create(ctx, user.ID, "project live", ConversationPatch{ProjectID: &pid})
	projectArchived, _ := convs.Create(ctx, user.ID, "project archived", ConversationPatch{ProjectID: &pid})
	botLive, _ := convs.Create(ctx, user.ID, "bot live", ConversationPatch{BotID: &bid})
	botArchived, _ := convs.Create(ctx, user.ID, "bot archived", ConversationPatch{BotID: &bid})
	_ = convs.SetArchived(ctx, projectArchived.ID, user.ID, true)
	_ = convs.SetArchived(ctx, botArchived.ID, user.ID, true)

	rows, err := convs.ListByProject(ctx, pid, user.ID)
	if err != nil || len(rows) != 1 || rows[0].ID != projectLive.ID {
		t.Fatalf("project list = %+v %v", rows, err)
	}
	rows, err = convs.ListByBot(ctx, bid, user.ID)
	if err != nil || len(rows) != 1 || rows[0].ID != botLive.ID {
		t.Fatalf("bot list = %+v %v", rows, err)
	}
	latest, err := convs.LatestForBot(ctx, bid, user.ID)
	if err != nil || latest.ID != botLive.ID {
		t.Fatalf("latest bot conversation = %+v %v", latest, err)
	}
}

func TestSetAutoTitleOnlyFromHeuristic(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "autotitle@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "autotitle@test.dev")
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	convs := NewConversations(pool)
	conv, err := convs.Create(ctx, user.ID, "first message", ConversationPatch{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = convs.Delete(context.Background(), conv.ID, user.ID) })

	if err := convs.SetAutoTitle(ctx, conv.ID, user.ID, "Model Title"); err != nil {
		t.Fatalf("set auto: %v", err)
	}
	got, err := convs.ByID(ctx, conv.ID, user.ID)
	if err != nil || got.Title != "Model Title" || got.TitleSource != "auto" {
		t.Fatalf("auto title = %q / %q (%v)", got.Title, got.TitleSource, err)
	}

	// The heuristic and a later auto attempt cannot rewrite an auto title.
	if err := convs.SetHeuristicTitle(ctx, conv.ID, user.ID, "heuristic again"); err != nil {
		t.Fatal(err)
	}
	if err := convs.SetAutoTitle(ctx, conv.ID, user.ID, "Second Auto"); err != nil {
		t.Fatal(err)
	}
	if got, _ := convs.ByID(ctx, conv.ID, user.ID); got.Title != "Model Title" {
		t.Fatalf("auto title overwritten: %q", got.Title)
	}

	// A manual rename wins and blocks further auto titles.
	if err := convs.RenameTitle(ctx, conv.ID, user.ID, "My Title"); err != nil {
		t.Fatal(err)
	}
	if err := convs.SetAutoTitle(ctx, conv.ID, user.ID, "Late Auto"); err != nil {
		t.Fatal(err)
	}
	if got, _ := convs.ByID(ctx, conv.ID, user.ID); got.Title != "My Title" || got.TitleSource != "manual" {
		t.Fatalf("manual title = %q / %q", got.Title, got.TitleSource)
	}
}
