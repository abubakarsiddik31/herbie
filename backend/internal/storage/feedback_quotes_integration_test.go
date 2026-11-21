package storage

import (
	"context"
	"testing"
)

func TestMessageQuotesAndFeedbackIntegration(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	users := NewUsers(pool)
	user, err := users.Create(ctx, "quote-feedback@test.dev", "hash")
	if err != nil {
		user, err = users.ByEmail(ctx, "quote-feedback@test.dev")
	}
	if err != nil {
		t.Fatalf("user setup: %v", err)
	}
	other, err := users.Create(ctx, "quote-feedback-other@test.dev", "hash")
	if err != nil {
		other, err = users.ByEmail(ctx, "quote-feedback-other@test.dev")
	}
	if err != nil {
		t.Fatalf("other user setup: %v", err)
	}

	convs := NewConversations(pool)
	msgs := NewMessages(pool)
	fb := NewFeedback(pool)

	conv, err := convs.Create(ctx, user.ID, "Quote feedback", ConversationPatch{})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	t.Cleanup(func() { _ = convs.Delete(context.Background(), conv.ID, user.ID) })

	targetID, err := msgs.Add(ctx, Message{ConversationID: conv.ID, UserID: user.ID, Data: []byte("{}"), Role: "user", Content: "target"})
	if err != nil {
		t.Fatalf("add target: %v", err)
	}
	replyID, err := msgs.Add(ctx, Message{
		ConversationID: conv.ID, UserID: user.ID, Data: []byte("{}"), Role: "user", ParentID: targetID,
		ReplyToID: targetID, QuoteText: "target", Content: "reply",
	})
	if err != nil {
		t.Fatalf("add reply: %v", err)
	}

	got, err := msgs.ByID(ctx, replyID, user.ID)
	if err != nil {
		t.Fatalf("load reply: %v", err)
	}
	if got.ReplyToID != targetID || got.QuoteText != "target" || got.Content != "reply" {
		t.Fatalf("stored reply = replyTo %q quote %q content %q", got.ReplyToID, got.QuoteText, got.Content)
	}

	// Upsert keeps one row per (user, message) and the conversation read is
	// scoped to the owner.
	if err := fb.Set(ctx, user.ID, replyID, "up"); err != nil {
		t.Fatalf("set up: %v", err)
	}
	if err := fb.Set(ctx, user.ID, replyID, "down"); err != nil {
		t.Fatalf("set down: %v", err)
	}
	ratings, err := fb.ForConversation(ctx, conv.ID, user.ID)
	if err != nil || ratings[replyID] != "down" {
		t.Fatalf("ratings = %+v %v", ratings, err)
	}
	if theirs, err := fb.ForConversation(ctx, conv.ID, other.ID); err != nil || len(theirs) != 0 {
		t.Fatalf("cross-user ratings = %+v %v", theirs, err)
	}
	if err := fb.Set(ctx, other.ID, replyID, "up"); err != nil {
		t.Fatalf("other user set: %v", err)
	}
	if ratings, _ := fb.ForConversation(ctx, conv.ID, user.ID); ratings[replyID] != "down" {
		t.Fatalf("other user's rating leaked: %+v", ratings)
	}
	if err := fb.Clear(ctx, user.ID, replyID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if ratings, _ := fb.ForConversation(ctx, conv.ID, user.ID); len(ratings) != 0 {
		t.Fatalf("ratings after clear = %+v", ratings)
	}

	// Forking rewrites the quote pointer to the copied target.
	target, err := msgs.ByID(ctx, targetID, user.ID)
	if err != nil {
		t.Fatalf("load target: %v", err)
	}
	fork, err := convs.Create(ctx, user.ID, "Fork", ConversationPatch{})
	if err != nil {
		t.Fatalf("create fork: %v", err)
	}
	t.Cleanup(func() { _ = convs.Delete(context.Background(), fork.ID, user.ID) })
	if _, err := msgs.CopyPath(ctx, conv.ID, user.ID, fork.ID, []Message{target, got}); err != nil {
		t.Fatalf("copy path: %v", err)
	}
	forkRows, err := msgs.ForConversation(ctx, fork.ID, user.ID)
	if err != nil {
		t.Fatalf("load fork rows: %v", err)
	}
	var copiedTarget, copiedReply Message
	for _, m := range forkRows {
		switch m.Content {
		case "target":
			copiedTarget = m
		case "reply":
			copiedReply = m
		}
	}
	if copiedReply.ReplyToID != copiedTarget.ID || copiedReply.QuoteText != "target" {
		t.Fatalf("forked reply = replyTo %q quote %q (target %q)", copiedReply.ReplyToID, copiedReply.QuoteText, copiedTarget.ID)
	}

	// Deleting the quoted sibling drops the pointer (FK SET NULL) but leaves
	// the snapshot the model saw.
	rootID, _ := msgs.Add(ctx, Message{ConversationID: conv.ID, UserID: user.ID, Data: []byte("{}"), Role: "user", Content: "root"})
	quoteID, _ := msgs.Add(ctx, Message{ConversationID: conv.ID, UserID: user.ID, Data: []byte("{}"), Role: "user", ParentID: rootID, Content: "quote me"})
	answerID, _ := msgs.Add(ctx, Message{
		ConversationID: conv.ID, UserID: user.ID, Data: []byte("{}"), Role: "user", ParentID: rootID,
		ReplyToID: quoteID, QuoteText: "quote me", Content: "responder",
	})
	if err := msgs.DeleteBranch(ctx, conv.ID, quoteID, user.ID); err != nil {
		t.Fatalf("delete quoted branch: %v", err)
	}
	survivor, err := msgs.ByID(ctx, answerID, user.ID)
	if err != nil {
		t.Fatalf("load survivor: %v", err)
	}
	if survivor.ReplyToID != "" || survivor.QuoteText != "quote me" {
		t.Fatalf("survivor = replyTo %q quote %q", survivor.ReplyToID, survivor.QuoteText)
	}

	// Feedback cascades with the rated message.
	if err := fb.Set(ctx, user.ID, answerID, "up"); err != nil {
		t.Fatalf("set survivor: %v", err)
	}
	if err := msgs.DeleteBranch(ctx, conv.ID, answerID, user.ID); err != nil {
		t.Fatalf("delete rated branch: %v", err)
	}
	if ratings, _ := fb.ForConversation(ctx, conv.ID, user.ID); len(ratings) != 0 {
		t.Fatalf("feedback survived message delete: %+v", ratings)
	}
}
