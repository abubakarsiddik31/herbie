package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

type conversationDetail struct {
	Conversation conversationDTO `json:"conversation"`
	Messages     []msgDTO        `json:"messages"`
}

func getConversationDetail(t *testing.T, h http.Handler, token, convID string) conversationDetail {
	t.Helper()
	rec := doAuthed(t, h, token, http.MethodGet, "/api/conversations/"+convID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get conversation: %d %s", rec.Code, rec.Body.String())
	}
	var detail conversationDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	return detail
}

// pongModel answers with a fixed assistant turn; the testmodel records every
// request, so tests can assert the exact model-facing prompt.
func pongModel() *testmodel.Scripted {
	return testmodel.New().Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "ok"}})
}

func lastPrompt(t *testing.T, m *testmodel.Scripted) string {
	t.Helper()
	requests := m.Requests()
	if len(requests) == 0 {
		t.Fatal("model received no request")
	}
	msgs := requests[len(requests)-1].Messages
	if len(msgs) == 0 {
		t.Fatal("request carried no messages")
	}
	return msgs[len(msgs)-1].Content
}

func TestComposeUserContent(t *testing.T) {
	if got := composeUserContent("plain", ""); got != "plain" {
		t.Fatalf("empty quote must pass content through, got %q", got)
	}
	got := composeUserContent("follow up", "line one\nline two")
	want := "[Replying to an earlier message]\n> line one\n> line two\n\nfollow up"
	if got != want {
		t.Fatalf("compose = %q, want %q", got, want)
	}
}

func TestSnapshotQuoteCapsLongContent(t *testing.T) {
	long := strings.Repeat("a", maxQuoteRunes+25)
	got := snapshotQuote(long)
	if len([]rune(got)) != maxQuoteRunes+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("capped snapshot length = %d, suffix = %v", len([]rune(got)), strings.HasSuffix(got, "…"))
	}
	if snapshotQuote("  padded  ") != "padded" {
		t.Fatalf("snapshot must trim whitespace")
	}
}

func TestSendMessageQuoteSnapshotAndComposition(t *testing.T) {
	m := pongModel()
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())

	conv := convs.mustCreate("u-1", "")
	quotedID, err := msgs.Add(context.Background(), storage.Message{
		ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "original question",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", quotedID); err != nil {
		t.Fatal(err)
	}

	rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages",
		map[string]any{"content": "follow up", "replyToId": quotedID})
	if rec.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}

	var quoted storage.Message
	for _, m := range msgs.rows {
		if m.Content == "follow up" {
			quoted = m
		}
	}
	if quoted.ID == "" {
		t.Fatal("quoted user row not persisted")
	}
	if quoted.ReplyToID != quotedID || quoted.QuoteText != "original question" {
		t.Fatalf("stored quote = replyTo %q quote %q", quoted.ReplyToID, quoted.QuoteText)
	}
	var payload model.Message
	if err := json.Unmarshal(quoted.Data, &payload); err != nil {
		t.Fatal(err)
	}
	wantModelContent := composeUserContent("follow up", "original question")
	if payload.Content != wantModelContent {
		t.Fatalf("stored model content = %q, want %q", payload.Content, wantModelContent)
	}
	if got := lastPrompt(t, m); got != wantModelContent {
		t.Fatalf("model prompt = %q, want %q", got, wantModelContent)
	}

	detail := getConversationDetail(t, h, token, conv.ID)
	var seen bool
	for _, m := range detail.Messages {
		if m.ID == quoted.ID {
			seen = true
			if m.Content != "follow up" || m.ReplyToID != quotedID || m.QuoteText != "original question" {
				t.Fatalf("transcript quote = %+v", m)
			}
		}
	}
	if !seen {
		t.Fatal("quoted row missing from transcript")
	}
}

func TestSendMessageQuoteRejectsBadTargets(t *testing.T) {
	agent := newTestAgent(t, pongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	ctx := context.Background()

	conv := convs.mustCreate("u-1", "")
	other := convs.mustCreate("u-1", "other")
	foreign := convs.mustCreate("u-2", "foreign")
	inOther, _ := msgs.Add(ctx, storage.Message{ConversationID: other.ID, UserID: "u-1", Role: "user", Content: "elsewhere"})
	inForeign, _ := msgs.Add(ctx, storage.Message{ConversationID: foreign.ID, UserID: "u-2", Role: "user", Content: "secret"})
	empty, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "assistant", Content: ""})

	for name, replyTo := range map[string]string{
		"unknown":    "00000000-0000-0000-0000-000000000000",
		"cross-conv": inOther,
		"foreign":    inForeign,
	} {
		rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages",
			map[string]any{"content": "hi", "replyToId": replyTo})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s quote = %d %s", name, rec.Code, rec.Body.String())
		}
	}
	rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages",
		map[string]any{"content": "hi", "replyToId": empty})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty quote = %d %s", rec.Code, rec.Body.String())
	}
}

func TestEditCarriesQuote(t *testing.T) {
	m := pongModel()
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	ctx := context.Background()

	conv := convs.mustCreate("u-1", "")
	quotedID, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "original question"})
	_ = convs.SetActiveLeaf(ctx, conv.ID, "u-1", quotedID)

	if rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages",
		map[string]any{"content": "follow up", "replyToId": quotedID}); rec.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	var target storage.Message
	for _, m := range msgs.rows {
		if m.Content == "follow up" {
			target = m
		}
	}

	if rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+target.ID+"/edit",
		map[string]any{"content": "edited"}); rec.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body.String())
	}

	var edited storage.Message
	for _, m := range msgs.rows {
		if m.Content == "edited" {
			edited = m
		}
	}
	if edited.ID == "" {
		t.Fatal("edited row missing")
	}
	if edited.ParentID != target.ParentID || edited.ReplyToID != quotedID || edited.QuoteText != "original question" {
		t.Fatalf("edited row = parent %q replyTo %q quote %q", edited.ParentID, edited.ReplyToID, edited.QuoteText)
	}
	var payload model.Message
	if err := json.Unmarshal(edited.Data, &payload); err != nil {
		t.Fatal(err)
	}
	want := composeUserContent("edited", "original question")
	if payload.Content != want {
		t.Fatalf("edited model content = %q, want %q", payload.Content, want)
	}
	if got := lastPrompt(t, m); got != want {
		t.Fatalf("edit prompt = %q, want %q", got, want)
	}
}

func TestRegenerateUsesQuoteComposition(t *testing.T) {
	m := pongModel()
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	ctx := context.Background()

	conv := convs.mustCreate("u-1", "")
	quotedID, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "original question"})
	_ = convs.SetActiveLeaf(ctx, conv.ID, "u-1", quotedID)

	if rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages",
		map[string]any{"content": "follow up", "replyToId": quotedID}); rec.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}

	if rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/regenerate", nil); rec.Code != http.StatusOK {
		t.Fatalf("regenerate: %d %s", rec.Code, rec.Body.String())
	}
	want := composeUserContent("follow up", "original question")
	if got := lastPrompt(t, m); got != want {
		t.Fatalf("regenerate prompt = %q, want %q", got, want)
	}
}

func TestForkRemapsQuote(t *testing.T) {
	agent := newTestAgent(t, pongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	ctx := context.Background()

	conv := convs.mustCreate("u-1", "Quoted chat")
	targetID, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "target"})
	replyID, _ := msgs.Add(ctx, storage.Message{
		ConversationID: conv.ID, UserID: "u-1", Role: "user", ParentID: targetID,
		ReplyToID: targetID, QuoteText: "target", Content: "reply",
	})
	_ = convs.SetActiveLeaf(ctx, conv.ID, "u-1", replyID)

	rec := doAuthed(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/fork", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("fork: %d %s", rec.Code, rec.Body.String())
	}
	var forked conversationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &forked); err != nil {
		t.Fatal(err)
	}

	copied := map[string]storage.Message{}
	for _, m := range msgs.rows {
		if m.ConversationID == forked.ID {
			copied[m.Content] = m
		}
	}
	target, reply := copied["target"], copied["reply"]
	if target.ID == "" || reply.ID == "" {
		t.Fatalf("fork rows missing: %+v", copied)
	}
	if reply.ReplyToID != target.ID || reply.QuoteText != "target" {
		t.Fatalf("forked reply = replyTo %q quote %q (target %q)", reply.ReplyToID, reply.QuoteText, target.ID)
	}
}

func TestFeedbackEndpoints(t *testing.T) {
	agent := newTestAgent(t, pongModel())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	ctx := context.Background()

	conv := convs.mustCreate("u-1", "")
	userID, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "q"})
	answerID, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "assistant", ParentID: userID, Content: "a"})
	_ = convs.SetActiveLeaf(ctx, conv.ID, "u-1", answerID)

	path := "/api/conversations/" + conv.ID + "/messages/" + answerID + "/feedback"
	for _, rating := range []string{"up", "down"} {
		if rec := doAuthed(t, h, token, http.MethodPut, path, map[string]string{"rating": rating}); rec.Code != http.StatusNoContent {
			t.Fatalf("set %s: %d %s", rating, rec.Code, rec.Body.String())
		}
		detail := getConversationDetail(t, h, token, conv.ID)
		if detail.Messages[len(detail.Messages)-1].Feedback != rating {
			t.Fatalf("stored rating = %q, want %q", detail.Messages[len(detail.Messages)-1].Feedback, rating)
		}
	}
	if rec := doAuthed(t, h, token, http.MethodDelete, path, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}
	if got := getConversationDetail(t, h, token, conv.ID).Messages[len(msgs.rows)-1].Feedback; got != "" {
		t.Fatalf("rating after clear = %q", got)
	}
	if rec := doAuthed(t, h, token, http.MethodPut, path, map[string]string{"rating": "sideways"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid rating = %d", rec.Code)
	}

	userPath := "/api/conversations/" + conv.ID + "/messages/" + userID + "/feedback"
	if rec := doAuthed(t, h, token, http.MethodPut, userPath, map[string]string{"rating": "up"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("user row rating = %d", rec.Code)
	}

	other := convs.mustCreate("u-1", "other")
	otherMsg, _ := msgs.Add(ctx, storage.Message{ConversationID: other.ID, UserID: "u-1", Role: "assistant", Content: "a"})
	if rec := doAuthed(t, h, token, http.MethodPut,
		"/api/conversations/"+conv.ID+"/messages/"+otherMsg+"/feedback", map[string]string{"rating": "up"}); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-conversation rating = %d", rec.Code)
	}

	foreign := convs.mustCreate("u-2", "foreign")
	foreignMsg, _ := msgs.Add(ctx, storage.Message{ConversationID: foreign.ID, UserID: "u-2", Role: "assistant", Content: "a"})
	if rec := doAuthed(t, h, token, http.MethodPut,
		"/api/conversations/"+foreign.ID+"/messages/"+foreignMsg+"/feedback", map[string]string{"rating": "up"}); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign rating = %d", rec.Code)
	}
}

func TestFeedbackOmittedFromSharedTranscript(t *testing.T) {
	h, token, convs, msgs := newShareTestServer(t)
	ctx := context.Background()
	conv := convs.mustCreate("u-1", "Shared")
	userID, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "q"})
	answerID, _ := msgs.Add(ctx, storage.Message{ConversationID: conv.ID, UserID: "u-1", Role: "assistant", ParentID: userID, Content: "a"})
	_ = convs.SetActiveLeaf(ctx, conv.ID, "u-1", answerID)

	if rec := doAuthed(t, h, token, http.MethodPut,
		"/api/conversations/"+conv.ID+"/messages/"+answerID+"/feedback", map[string]string{"rating": "up"}); rec.Code != http.StatusNoContent {
		t.Fatalf("set feedback: %d %s", rec.Code, rec.Body.String())
	}

	shareRec := httptest.NewRecorder()
	h.ServeHTTP(shareRec, authedReq(http.MethodPost, "/api/conversations/"+conv.ID+"/shares", token))
	if shareRec.Code != http.StatusCreated {
		t.Fatalf("share: %d %s", shareRec.Code, shareRec.Body.String())
	}
	var created struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(shareRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	pub := httptest.NewRecorder()
	h.ServeHTTP(pub, httptest.NewRequest(http.MethodGet, "/api/shared/"+created.Token, nil))
	if pub.Code != http.StatusOK {
		t.Fatalf("public read: %d %s", pub.Code, pub.Body.String())
	}
	var thread struct {
		Messages []msgDTO `json:"messages"`
	}
	if err := json.Unmarshal(pub.Body.Bytes(), &thread); err != nil {
		t.Fatal(err)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("shared transcript messages = %d", len(thread.Messages))
	}
	if thread.Messages[1].Feedback != "" {
		t.Fatalf("shared transcript leaked feedback %q", thread.Messages[1].Feedback)
	}
}
