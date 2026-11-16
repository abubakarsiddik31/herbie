package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

func addTreeRow(t *testing.T, msgs *fakeMsgs, convID, userID, parent, role, content string, at time.Time) string {
	t.Helper()
	id, err := msgs.Add(context.Background(), storage.Message{
		ConversationID: convID, UserID: userID, ParentID: parent, Role: role,
		Content: content, CreatedAt: at, Data: mustJSON(model.Message{Role: model.Role(role), Content: content}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func addToolCallRow(t *testing.T, msgs *fakeMsgs, convID, userID, parent, callID string, at time.Time) string {
	t.Helper()
	msg := model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: callID, Name: "get_weather", Args: json.RawMessage(`{}`)}}}
	id, err := msgs.Add(context.Background(), storage.Message{
		ConversationID: convID, UserID: userID, ParentID: parent, Role: string(model.RoleAssistant),
		Data: mustJSON(msg), CreatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func branchReq(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := reqJSON(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEditAndRegenerateSiblingsActivateBothWays(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newEndlessAgent(t, "answer"), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")

	for _, content := range []string{"first", "second"} {
		if rec := postMessage(t, h, token, conv.ID, content); rec.Code != http.StatusOK {
			t.Fatalf("send %q: %d %s", content, rec.Code, rec.Body.String())
		}
	}
	rows := msgs.forConv(conv.ID, "u-1")
	if len(rows) != 4 {
		t.Fatalf("seed rows = %d, want 4", len(rows))
	}

	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+rows[0].ID+"/edit",
		map[string]string{"content": "rewritten"}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body.String())
	}
	rows = msgs.forConv(conv.ID, "u-1")
	if len(rows) != 6 || rows[4].ParentID != "" || rows[5].ParentID != rows[4].ID {
		t.Fatalf("edit must create a sibling root branch: %+v", rows)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != rows[5].ID {
		t.Fatalf("edit did not move the leaf: %v", leaf)
	}

	// Activating the original root picks the deepest leaf of its subtree.
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+rows[0].ID+"/activate", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("activate original: %d %s", rec.Code, rec.Body.String())
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != rows[3].ID {
		t.Fatalf("activate original root = %v, want deepest old leaf %s", leaf, rows[3].ID)
	}

	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+rows[4].ID+"/activate", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("activate edited root: %d %s", rec.Code, rec.Body.String())
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != rows[5].ID {
		t.Fatalf("activate edited root = %v, want %s", leaf, rows[5].ID)
	}

	// Back on the old branch, regenerate the last answer: a sibling of the
	// old answer, which activation can switch between.
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+rows[0].ID+"/activate", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("reactivate original: %d", rec.Code)
	}
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/regenerate", map[string]any{}); rec.Code != http.StatusOK {
		t.Fatalf("regenerate: %d %s", rec.Code, rec.Body.String())
	}
	rows = msgs.forConv(conv.ID, "u-1")
	if len(rows) != 7 {
		t.Fatalf("rows after regenerate = %d, want 7", len(rows))
	}
	oldAnswer, newAnswer := rows[3], rows[6]
	if newAnswer.ParentID != oldAnswer.ParentID {
		t.Fatalf("regenerated answer must be a sibling: old parent %q new %q", oldAnswer.ParentID, newAnswer.ParentID)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != newAnswer.ID {
		t.Fatalf("regenerate did not move the leaf: %v", leaf)
	}
	for _, target := range []string{oldAnswer.ID, newAnswer.ID} {
		if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+target+"/activate", nil); rec.Code != http.StatusNoContent {
			t.Fatalf("activate answer %s: %d", target, rec.Code)
		}
		if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != target {
			t.Fatalf("activate answer %s -> %v", target, leaf)
		}
	}

	// A second edit branches off the old branch's second user row.
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+rows[0].ID+"/activate", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("activate old root: %d", rec.Code)
	}
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+rows[2].ID+"/edit",
		map[string]string{"content": "second edited"}); rec.Code != http.StatusOK {
		t.Fatalf("second edit: %d %s", rec.Code, rec.Body.String())
	}
	rows = msgs.forConv(conv.ID, "u-1")
	if len(rows) != 9 {
		t.Fatalf("rows after second edit = %d, want 9", len(rows))
	}
	editedUser, editedAnswer := rows[7], rows[8]
	if editedUser.ParentID != rows[1].ID || editedAnswer.ParentID != editedUser.ID {
		t.Fatalf("second edit must fork under the first assistant: %+v", rows[7:])
	}

	// A second regenerate branches the edited turn's answer again.
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/regenerate", map[string]any{}); rec.Code != http.StatusOK {
		t.Fatalf("second regenerate: %d %s", rec.Code, rec.Body.String())
	}
	rows = msgs.forConv(conv.ID, "u-1")
	if len(rows) != 10 || rows[9].ParentID != editedUser.ID {
		t.Fatalf("second regenerate did not create a sibling answer: %+v", rows)
	}
	editedVariant := rows[9]
	if editedVariant.ID == editedAnswer.ID {
		t.Fatal("regenerate reused the old answer row")
	}

	// Both second-level branches switch deterministically.
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+rows[2].ID+"/activate", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("activate old turn: %d", rec.Code)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != newAnswer.ID {
		t.Fatalf("old turn activate -> %v, want newest answer %s", leaf, newAnswer.ID)
	}
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+editedUser.ID+"/activate", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("activate edited turn: %d", rec.Code)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != editedVariant.ID {
		t.Fatalf("edited turn activate -> %v, want %s", leaf, editedVariant.ID)
	}
}

func TestTranscriptBranchMetadataActivePathOnly(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	base := time.Now().Add(-time.Hour).Truncate(time.Second)

	rootUser := addTreeRow(t, msgs, conv.ID, "u-1", "", "user", "root", base)
	otherRoot := addTreeRow(t, msgs, conv.ID, "u-1", "", "user", "other root", base.Add(time.Second))
	_ = otherRoot
	callA := addToolCallRow(t, msgs, conv.ID, "u-1", rootUser, "call-a", base.Add(2*time.Second))
	addTreeRow(t, msgs, conv.ID, "u-1", callA, "tool", "tool-a", base.Add(3*time.Second))
	addTreeRow(t, msgs, conv.ID, "u-1", callA, "assistant", "answer-a", base.Add(4*time.Second))
	callB := addToolCallRow(t, msgs, conv.ID, "u-1", rootUser, "call-b", base.Add(5*time.Second))
	addTreeRow(t, msgs, conv.ID, "u-1", callB, "tool", "tool-b", base.Add(6*time.Second))
	answerB := addTreeRow(t, msgs, conv.ID, "u-1", callB, "assistant", "answer-b", base.Add(7*time.Second))
	if err := convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", answerB); err != nil {
		t.Fatal(err)
	}

	rec := branchReq(t, h, token, http.MethodGet, "/api/conversations/"+conv.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Conversation conversationDTO `json:"conversation"`
		Messages     []msgDTO        `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Conversation.ActiveLeafID == nil || *got.Conversation.ActiveLeafID != answerB {
		t.Fatalf("activeLeafId = %v, want %s", got.Conversation.ActiveLeafID, answerB)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("transcript must contain only the active path's visible rows: %+v", got.Messages)
	}
	user, assistant := got.Messages[0], got.Messages[1]
	if user.ID != rootUser || assistant.ID != answerB {
		t.Fatalf("unexpected transcript rows: %+v", got.Messages)
	}
	if user.Branch == nil || user.Branch.Count != 2 || user.Branch.Index != 0 ||
		len(user.Branch.IDs) != 2 || user.Branch.IDs[0] != rootUser {
		t.Fatalf("user branches = %+v", user.Branch)
	}
	// Assistant variants are chain roots (the hidden tool-call rows), and the
	// metadata rides the last visible assistant row of the active chain.
	if assistant.Branch == nil || assistant.Branch.Count != 2 || assistant.Branch.Index != 1 ||
		len(assistant.Branch.IDs) != 2 || assistant.Branch.IDs[0] != callA || assistant.Branch.IDs[1] != callB {
		t.Fatalf("assistant branches = %+v, want chain roots [%s %s]", assistant.Branch, callA, callB)
	}
}

func TestActivatePicksDeterministicDeepestLeaf(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	base := time.Now().Add(-time.Hour).Truncate(time.Second)

	root := addTreeRow(t, msgs, conv.ID, "u-1", "", "user", "q", base)
	a1 := addTreeRow(t, msgs, conv.ID, "u-1", root, "assistant", "a1", base.Add(time.Second))
	g1 := addTreeRow(t, msgs, conv.ID, "u-1", a1, "assistant", "g1", base.Add(2*time.Second))
	a2 := addTreeRow(t, msgs, conv.ID, "u-1", root, "assistant", "a2", base.Add(3*time.Second))
	h2 := addTreeRow(t, msgs, conv.ID, "u-1", a2, "assistant", "h2", base.Add(4*time.Second))

	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+root+"/activate", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("activate: %d", rec.Code)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != h2 {
		t.Fatalf("tie-break must pick the newest deepest leaf %s, got %v", h2, leaf)
	}
	_ = g1

	deeper := addTreeRow(t, msgs, conv.ID, "u-1", g1, "assistant", "g2", base.Add(5*time.Second))
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+root+"/activate", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("activate deeper: %d", rec.Code)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != deeper {
		t.Fatalf("deepest leaf = %v, want %s", leaf, deeper)
	}
}

func TestDeleteActiveAndInactiveBranches(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newEndlessAgent(t, "answer"), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	base := time.Now().Add(-time.Hour).Truncate(time.Second)

	root := addTreeRow(t, msgs, conv.ID, "u-1", "", "user", "q", base)
	active := addTreeRow(t, msgs, conv.ID, "u-1", root, "assistant", "active", base.Add(time.Second))
	inactive := addTreeRow(t, msgs, conv.ID, "u-1", root, "assistant", "inactive", base.Add(2*time.Second))
	if err := convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", active); err != nil {
		t.Fatal(err)
	}

	if rec := branchReq(t, h, token, http.MethodDelete, "/api/conversations/"+conv.ID+"/messages/"+inactive, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete inactive: %d", rec.Code)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != active {
		t.Fatalf("deleting an inactive branch moved the leaf: %v", leaf)
	}

	if rec := branchReq(t, h, token, http.MethodDelete, "/api/conversations/"+conv.ID+"/messages/"+active, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete active: %d", rec.Code)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != root {
		t.Fatalf("active subtree delete must fall back to the parent: %v", leaf)
	}

	if rec := branchReq(t, h, token, http.MethodDelete, "/api/conversations/"+conv.ID+"/messages/"+root, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete root: %d", rec.Code)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf != nil {
		t.Fatalf("root delete must clear the leaf: %v", leaf)
	}
	if len(msgs.forConv(conv.ID, "u-1")) != 0 {
		t.Fatalf("root delete left rows: %+v", msgs.forConv(conv.ID, "u-1"))
	}

	// The next send starts a fresh single root, never a second parentless row.
	if rec := postMessage(t, h, token, conv.ID, "fresh"); rec.Code != http.StatusOK {
		t.Fatalf("send after delete: %d %s", rec.Code, rec.Body.String())
	}
	fresh := msgs.forConv(conv.ID, "u-1")
	if len(fresh) != 2 || fresh[0].ParentID != "" || fresh[1].ParentID != fresh[0].ID {
		t.Fatalf("fresh send did not start a new root: %+v", fresh)
	}
}

func TestDeleteAndMutationsBlockedWhilePending(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	pending := &fakePending{}
	h, token := newHandlerServerFull(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage(), newFakeToolStore(), pending)
	conv := convs.mustCreate("u-1", "")
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	root := addTreeRow(t, msgs, conv.ID, "u-1", "", "user", "q", base)
	if err := pending.Add(context.Background(), []storage.PendingToolCall{{
		CallID: "c-1-call-x", ConversationID: conv.ID, UserID: "u-1", ToolName: "t", Status: "pending",
	}}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/conversations/" + conv.ID + "/messages", map[string]string{"content": "new"}},
		{http.MethodDelete, "/api/conversations/" + conv.ID + "/messages/" + root, nil},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/messages/" + root + "/edit", map[string]string{"content": "x"}},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/messages/" + root + "/activate", nil},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/regenerate", map[string]any{}},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/fork", map[string]any{}},
	}
	for _, tc := range cases {
		rec := branchReq(t, h, token, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusConflict || errorCode(t, rec) != "approval_pending" {
			t.Errorf("%s %s during pending = %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestMutationsBlockedDuringInFlightRun(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	m := testmodel.StreamFunc(func(ctx context.Context, _ model.Request, onDelta func(model.Delta) error) (model.Response, error) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
			return model.Response{}, ctx.Err()
		}
		_ = testmodel.Emit(onDelta, model.Delta{Content: "done"})
		return model.Response{
			Message: model.Message{Role: model.RoleAssistant, Content: "done"},
			Usage:   model.Usage{InputTokens: 1, OutputTokens: 1},
		}, nil
	})
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, m), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	root := addTreeRow(t, msgs, conv.ID, "u-1", "", "user", "q", base)
	if err := convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", root); err != nil {
		t.Fatal(err)
	}

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- postMessage(t, h, token, conv.ID, "blocking")
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("run never started")
	}

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/conversations/" + conv.ID + "/messages", map[string]string{"content": "racing"}},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/messages/" + root + "/edit", map[string]string{"content": "x"}},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/messages/" + root + "/activate", nil},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/regenerate", map[string]any{}},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/fork", map[string]any{}},
		{http.MethodDelete, "/api/conversations/" + conv.ID + "/messages/" + root, nil},
	}
	for _, tc := range cases {
		rec := branchReq(t, h, token, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusConflict || errorCode(t, rec) != "run_active" {
			t.Errorf("%s %s during run = %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	close(release)
	rec := <-done
	if rec.Code != http.StatusOK {
		t.Fatalf("blocking run finished with %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBotConversationBranchEndpointsRejected(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, newFakeUsage())
	botID := "b-1"
	conv, err := convs.Create(context.Background(), "u-1", "bot chat", storage.ConversationPatch{BotID: &botID})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	root := addTreeRow(t, msgs, conv.ID, "u-1", "", "user", "q", base)
	answer := addTreeRow(t, msgs, conv.ID, "u-1", root, "assistant", "a", base.Add(time.Second))
	if err := convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", answer); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/conversations/" + conv.ID + "/messages/" + root + "/edit", map[string]string{"content": "x"}},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/regenerate", map[string]any{}},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/messages/" + root + "/activate", nil},
		{http.MethodPost, "/api/conversations/" + conv.ID + "/fork", map[string]any{}},
	}
	for _, tc := range cases {
		rec := branchReq(t, h, token, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusConflict || errorCode(t, rec) != "bot_conversation" {
			t.Errorf("%s %s on bot conversation = %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	// Deleting a branch is allowed: it is history pruning, not branching.
	if rec := branchReq(t, h, token, http.MethodDelete, "/api/conversations/"+conv.ID+"/messages/"+answer, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete on bot conversation: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRegenerateByteIdenticalKeepsBothVariants(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newEndlessAgent(t, "same"), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	if rec := postMessage(t, h, token, conv.ID, "hi"); rec.Code != http.StatusOK {
		t.Fatalf("send: %d", rec.Code)
	}
	if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/regenerate", map[string]any{}); rec.Code != http.StatusOK {
		t.Fatalf("regenerate: %d %s", rec.Code, rec.Body.String())
	}
	rows := msgs.forConv(conv.ID, "u-1")
	if len(rows) != 3 {
		t.Fatalf("byte-identical regenerate must still persist a variant: %d rows", len(rows))
	}
	first, second := rows[1], rows[2]
	if first.Content != "same" || second.Content != "same" || first.ParentID != second.ParentID {
		t.Fatalf("variants not preserved: %+v", rows)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != second.ID {
		t.Fatalf("active leaf = %v, want %s", leaf, second.ID)
	}
	for _, target := range []string{first.ID, second.ID} {
		if rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+conv.ID+"/messages/"+target+"/activate", nil); rec.Code != http.StatusNoContent {
			t.Fatalf("activate %s: %d", target, rec.Code)
		}
		if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != target {
			t.Fatalf("activate %s -> %v", target, leaf)
		}
	}
}

func TestForkCopiesPrefixAndSettings(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	h, token := newHandlerServer(t, newTestAgent(t, scriptedPongModel()), convs, msgs, usage)
	modelID := "gemini-2.5-pro"
	temp := 0.7
	sys := "be terse"
	rag := false
	project := "p-1"
	src, err := convs.Create(context.Background(), "u-1", "Source", storage.ConversationPatch{
		Model: &modelID, Temperature: &temp, SystemPrompt: &sys, RagEnabled: &rag, ProjectID: &project,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	u1 := addTreeRow(t, msgs, src.ID, "u-1", "", "user", "q1", base)
	a1 := addTreeRow(t, msgs, src.ID, "u-1", u1, "assistant", "a1", base.Add(time.Second))
	u2 := addTreeRow(t, msgs, src.ID, "u-1", a1, "user", "q2", base.Add(2*time.Second))
	a2 := addTreeRow(t, msgs, src.ID, "u-1", u2, "assistant", "a2", base.Add(3*time.Second))
	if err := convs.SetActiveLeaf(context.Background(), src.ID, "u-1", a2); err != nil {
		t.Fatal(err)
	}

	rec := branchReq(t, h, token, http.MethodPost, "/api/conversations/"+src.ID+"/fork", map[string]string{"messageId": a1})
	if rec.Code != http.StatusCreated {
		t.Fatalf("fork prefix: %d %s", rec.Code, rec.Body.String())
	}
	var forked conversationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &forked); err != nil {
		t.Fatal(err)
	}
	if forked.Title != "Fork of Source" || forked.Model != modelID || forked.Temperature == nil || *forked.Temperature != temp ||
		forked.SystemPrompt != sys || forked.RagEnabled != rag || forked.BotID != nil {
		t.Fatalf("fork settings not copied: %+v", forked)
	}
	copied := msgs.forConv(forked.ID, "u-1")
	if len(copied) != 2 || copied[0].Content != "q1" || copied[1].Content != "a1" {
		t.Fatalf("prefix fork rows = %+v", copied)
	}
	if copied[0].ID == u1 || copied[0].ParentID != "" || copied[1].ParentID != copied[0].ID {
		t.Fatalf("fork must mint fresh IDs and remap parents: %+v", copied)
	}
	if forked.ActiveLeafID == nil || *forked.ActiveLeafID != copied[1].ID {
		t.Fatalf("fork active leaf = %v, want %s", forked.ActiveLeafID, copied[1].ID)
	}
	if len(usage.events) != 0 {
		t.Fatalf("fork must not write ledger events: %+v", usage.events)
	}

	rec = branchReq(t, h, token, http.MethodPost, "/api/conversations/"+src.ID+"/fork", map[string]any{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("fork default: %d %s", rec.Code, rec.Body.String())
	}
	var full conversationDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &full)
	if got := msgs.forConv(full.ID, "u-1"); len(got) != 4 {
		t.Fatalf("default fork must copy the active path: %d rows", len(got))
	}
}

func TestSendRecoversNullLeafWithoutSecondRoot(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, newEndlessAgent(t, "answer"), convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	root := addTreeRow(t, msgs, conv.ID, "u-1", "", "user", "q", base)
	answer := addTreeRow(t, msgs, conv.ID, "u-1", root, "assistant", "a", base.Add(time.Second))

	if rec := postMessage(t, h, token, conv.ID, "next"); rec.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	rows := msgs.forConv(conv.ID, "u-1")
	var roots int
	var prompt storage.Message
	for _, m := range rows {
		if m.ParentID == "" {
			roots++
		}
		if m.Content == "next" {
			prompt = m
		}
	}
	if roots != 1 {
		t.Fatalf("NULL-leaf recovery created %d roots: %+v", roots, rows)
	}
	if prompt.ParentID != answer {
		t.Fatalf("recovered prompt parent = %q, want %s", prompt.ParentID, answer)
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != rows[len(rows)-1].ID {
		t.Fatalf("active leaf not recovered/persisted: %v", leaf)
	}
}

func TestResumeFailureDoesNotDuplicatePausedTail(t *testing.T) {
	// The resume model fails after echoing the paused history; positional
	// dedupe must not re-persist that tail.
	var calls int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"temp":"21C"}`))
	}))
	t.Cleanup(srv.Close)
	m := testmodel.StreamFunc(func(ctx context.Context, req model.Request, onDelta func(model.Delta) error) (model.Response, error) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			return model.Response{
				Message: model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{
					{ID: "call-1", Name: "get_weather", Args: json.RawMessage(`{}`)},
				}},
				Usage: model.Usage{InputTokens: 3, OutputTokens: 2},
			}, nil
		}
		_ = testmodel.Emit(onDelta, model.Delta{Content: "partial "})
		return model.Response{}, fmt.Errorf("boom")
	})
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	tools := newFakeToolStore()
	_, _ = tools.Create(context.Background(), storedWeatherTool(t, "u-1", srv.URL+"/wx/{{city}}", true))
	pending := &fakePending{}
	h, token := newHandlerServerFull(t, agent, convs, msgs, newFakeUsage(), tools, pending)

	conv := convs.mustCreate("u-1", "")
	rec := postMessage(t, h, token, conv.ID, "go")
	if !strings.Contains(rec.Body.String(), "event: approval_request") {
		t.Fatalf("pause leg failed:\n%s", rec.Body.String())
	}
	callID := announcedCallID(t, rec.Body.String())
	before := msgs.forConv(conv.ID, "u-1")

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", map[string]any{
		"decisions": []map[string]any{{"callId": callID, "approved": true}},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "event: error") {
		t.Fatalf("resume should fail:\n%s", rec.Body.String())
	}
	after := msgs.forConv(conv.ID, "u-1")
	users, callsRows := 0, 0
	for _, row := range after {
		switch row.Role {
		case string(model.RoleUser):
			users++
		case string(model.RoleAssistant):
			if strings.Contains(string(row.Data), "toolCalls") {
				callsRows++
			}
		}
	}
	if users != 1 || callsRows != 1 {
		t.Fatalf("resume failure duplicated the paused tail: users=%d toolcall rows=%d before=%d after=%d",
			users, callsRows, len(before), len(after))
	}
}

func TestBotRunnerAppendsToActiveLeafAndResumesFromPath(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	runs := newFakeBotRuns()
	pending := &fakePending{}
	agent := &fakeBotAgent{steps: []fakeAgentStep{{
		outcome: chat.Outcome{
			Messages: []model.Message{
				{Role: model.RoleUser, Content: "task"},
				{Role: model.RoleAssistant, Content: "answer"},
			},
			Usage:    model.Usage{InputTokens: 2, OutputTokens: 1},
			Requests: 1,
		},
	}}}
	s := newBotRunnerTestServer(agent, runs, convs, msgs, usage, pending)
	conv := convs.mustCreate("u-1", "seed")
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	root := addTreeRow(t, msgs, conv.ID, "u-1", "", "user", "old q", base)
	leaf := addTreeRow(t, msgs, conv.ID, "u-1", root, "assistant", "old a", base.Add(time.Second))
	if err := convs.SetActiveLeaf(context.Background(), conv.ID, "u-1", leaf); err != nil {
		t.Fatal(err)
	}
	run, err := runs.Create(context.Background(), storage.BotRun{ConversationID: conv.ID, UserID: "u-1", Task: "task"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartBotRunner(ctx)
	s.WakeBotRunner()
	waitForBotRun(t, runs, run.ID, "succeeded")

	rows := msgs.forConv(conv.ID, "u-1")
	var taskRow, answerRow storage.Message
	for _, m := range rows {
		if m.Content == "task" {
			taskRow = m
		}
		if m.Content == "answer" {
			answerRow = m
		}
	}
	if taskRow.ParentID != leaf || answerRow.ParentID != taskRow.ID {
		t.Fatalf("bot run did not append to the active leaf: task=%q answer=%q", taskRow.ParentID, answerRow.ParentID)
	}
	agent.mu.Lock()
	history := append([]model.Message(nil), agent.lastHistory...)
	agent.mu.Unlock()
	if len(history) != 2 || history[0].Content != "old q" || history[1].Content != "old a" {
		t.Fatalf("bot history must be the active path: %+v", history)
	}
}

func TestBotResumeFailureDoesNotDuplicatePausedTail(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	runs := newFakeBotRuns()
	pending := &fakePending{}
	paused := []model.Message{
		{Role: model.RoleUser, Content: "check the weather"},
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call-1", Name: "get_weather", Args: json.RawMessage(`{}`)}}},
	}
	agent := &fakeBotAgent{steps: []fakeAgentStep{
		{outcome: chat.Outcome{
			Messages: paused,
			Usage:    model.Usage{InputTokens: 3, OutputTokens: 2},
			Requests: 1,
			Pending:  []chat.PendingApproval{{CallID: "call-1", ToolName: "get_weather"}},
		}},
		{echoPartial: true},
	}}
	s := newBotRunnerTestServer(agent, runs, convs, msgs, usage, pending)
	conv := convs.mustCreate("u-1", "Scout chat")
	run, err := runs.Create(context.Background(), storage.BotRun{ConversationID: conv.ID, UserID: "u-1", Task: "check the weather"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartBotRunner(ctx)
	s.WakeBotRunner()
	waitForBotRun(t, runs, run.ID, "waiting_approval")

	parked := pending.snapshot()
	if len(parked) != 1 {
		t.Fatalf("parked calls = %d", len(parked))
	}
	if parked[0].MessageID == "" {
		t.Fatal("parked call missing its message row reference")
	}
	if err := pending.SetStatus(context.Background(), "u-1", parked[0].CallID, "approved"); err != nil {
		t.Fatal(err)
	}
	s.NotifyBotRunResume(run.ID)
	waitForBotRun(t, runs, run.ID, "failed")

	users, callRows := 0, 0
	for _, row := range msgs.forConv(conv.ID, "u-1") {
		if row.Role == string(model.RoleUser) {
			users++
		}
		if row.Role == string(model.RoleAssistant) && strings.Contains(string(row.Data), "toolCalls") {
			callRows++
		}
	}
	if users != 1 || callRows != 1 {
		t.Fatalf("bot resume failure duplicated the paused tail: users=%d toolcall rows=%d", users, callRows)
	}
}

// TestResumeMixedBatchKeepsExecutedResults covers the deferred-batch hazard:
// when one call executes and another defers, the paused run persists the
// executed result after the assistant row. The parked anchor must be that
// tail (not the call row) and the resume must replay the executed result
// while persisting only the resolution and the new answer.
func TestResumeMixedBatchKeepsExecutedResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"time":"12:00"}`))
	}))
	t.Cleanup(srv.Close)

	var mu sync.Mutex
	var calls int
	var resumeHistory []model.Message
	m := testmodel.StreamFunc(func(_ context.Context, req model.Request, onDelta func(model.Delta) error) (model.Response, error) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			return model.Response{
				Message: model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{
					{ID: "call-1", Name: "get_time", Args: json.RawMessage(`{}`)},
					{ID: "call-2", Name: "get_weather", Args: json.RawMessage(`{"city":"x"}`)},
				}},
				Usage: model.Usage{InputTokens: 10, OutputTokens: 5},
			}, nil
		}
		mu.Lock()
		resumeHistory = append([]model.Message(nil), req.Messages...)
		mu.Unlock()
		_ = testmodel.Emit(onDelta, model.Delta{Content: "done"})
		return model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "done"},
			Usage: model.Usage{InputTokens: 10, OutputTokens: 5}}, nil
	})
	agent := newTestAgent(t, m)
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	tools := newFakeToolStore()
	if _, err := tools.Create(context.Background(), storedWeatherTool(t, "u-1", srv.URL+"/wx/{{city}}", true)); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.Create(context.Background(), storage.UserTool{
		UserID: "u-1", Name: "get_time", Description: "time", Method: "GET",
		URLTemplate: srv.URL + "/time", Params: json.RawMessage(`[]`), Headers: json.RawMessage(`{}`), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	pending := &fakePending{}
	h, token := newHandlerServerFull(t, agent, convs, msgs, newFakeUsage(), tools, pending)
	conv := convs.mustCreate("u-1", "")

	rec := postMessage(t, h, token, conv.ID, "go")
	if !strings.Contains(rec.Body.String(), "event: approval_request") {
		t.Fatalf("pause leg failed:\n%s", rec.Body.String())
	}
	callID := announcedCallID(t, rec.Body.String())

	rows := msgs.forConv(conv.ID, "u-1")
	var executedID string
	for _, row := range rows {
		if row.Role == string(model.RoleTool) && strings.Contains(row.Content, "12:00") {
			executedID = row.ID
		}
	}
	if executedID == "" {
		t.Fatalf("eager call result not persisted: %+v", rows)
	}
	parked := pending.snapshot()
	if len(parked) != 1 || parked[0].MessageID != executedID {
		t.Fatalf("parked anchor = %+v, want executed tail %s", parked, executedID)
	}

	req := reqJSON(http.MethodPost, "/api/conversations/"+conv.ID+"/approvals", map[string]any{
		"decisions": []map[string]any{{"callId": callID, "approved": true}},
	})
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("resume leg failed:\n%s", rec.Body.String())
	}

	mu.Lock()
	history := append([]model.Message(nil), resumeHistory...)
	mu.Unlock()
	var replayed bool
	for _, msg := range history {
		if msg.Role == model.RoleTool && strings.Contains(msg.Content, "12:00") {
			replayed = true
		}
	}
	if !replayed {
		t.Fatalf("resume history dropped the executed tool result: %+v", history)
	}

	after := msgs.forConv(conv.ID, "u-1")
	toolResults := 0
	for _, row := range after {
		if row.Role == string(model.RoleTool) {
			toolResults++
		}
	}
	if toolResults != 2 {
		t.Fatalf("resume persisted %d tool results, want 2 (executed + resolution): %+v", toolResults, after)
	}
	if len(after) != 5 {
		t.Fatalf("resumed rows = %d, want 5: %+v", len(after), after)
	}
	for i := 1; i < len(after); i++ {
		if after[i].ParentID != after[i-1].ID {
			t.Fatalf("resumed rows are not one chain: row %d parent %q, want %q", i, after[i].ParentID, after[i-1].ID)
		}
	}
	if leaf := convs.convs[conv.ID].ActiveLeafID; leaf == nil || *leaf != after[len(after)-1].ID {
		t.Fatalf("active leaf = %v, want %s", leaf, after[len(after)-1].ID)
	}
}
