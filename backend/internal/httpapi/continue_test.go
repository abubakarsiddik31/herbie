package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem"
	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

func TestContinueInterruptedAnswer(t *testing.T) {
	m := testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "continued text"},
		Usage:   model.Usage{InputTokens: 3, OutputTokens: 2},
	})
	agent := newTestAgent(t, m)
	usage := newFakeUsage()
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, usage)
	conv := convs.mustCreate("u-1", "")

	userID, err := msgs.Add(context.Background(), storage.Message{
		ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "question",
		Data: mustJSON(model.Message{Role: model.RoleUser, Content: "question"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	partialID, err := msgs.Add(context.Background(), storage.Message{
		ConversationID: conv.ID, UserID: "u-1", Role: "assistant", ParentID: userID,
		Content: "partial", Truncated: true,
		Data: mustJSON(model.Message{Role: model.RoleAssistant, Content: "partial"}),
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := doJSON(h, http.MethodPost, "/api/conversations/"+conv.ID+"/continue", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("continue: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "continued text") {
		t.Fatalf("missing continuation: %s", rec.Body.String())
	}
	var found bool
	for _, row := range msgs.rows {
		if row.Role == "assistant" && row.Content == "continued text" && row.ParentID == partialID {
			found = true
		}
	}
	if !found {
		t.Fatalf("continuation did not branch under the user row: %+v", msgs.rows)
	}
}

func TestPersistFailureSurvivesCancelledContext(t *testing.T) {
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	usage := newFakeUsage()
	srv := &Server{deps: ServerDeps{
		Msgs: msgs, Convos: convs, Usage: usage,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}
	conv := convs.mustCreate("u-1", "")
	promptID, err := msgs.Add(context.Background(), storage.Message{
		ConversationID: conv.ID, UserID: "u-1", Role: "user", Content: "question",
		Data: mustJSON(model.Message{Role: model.RoleUser, Content: "question"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate the client aborting the stream
	runErr := &golem.RunError{
		Stage: golem.StageModel,
		Err:   context.Canceled,
		Partial: &golem.PartialResult{
			Messages: []model.Message{
				{Role: model.RoleUser, Content: "question"},
				{Role: model.RoleAssistant, Content: "partial answer"},
			},
			Usage: model.Usage{InputTokens: 1, OutputTokens: 1},
		},
	}
	rec := httptest.NewRecorder()
	sink, _ := newSSESink(rec)
	srv.persistFailure(ctx, "u-1", conv.ID, chat.RunSpec{Model: "gemini-3.5-flash"}, promptID, nil, -1, runErr, sink)
	var found bool
	for _, row := range msgs.rows {
		if row.Role == "assistant" && row.Content == "partial answer" && row.Truncated {
			found = true
		}
	}
	if !found {
		t.Fatalf("cancelled run did not persist its partial: %+v", msgs.rows)
	}
}
