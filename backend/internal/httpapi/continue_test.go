package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

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
	if _, err := msgs.Add(context.Background(), storage.Message{
		ConversationID: conv.ID, UserID: "u-1", Role: "assistant", ParentID: userID,
		Content: "partial", Truncated: true,
		Data: mustJSON(model.Message{Role: model.RoleAssistant, Content: "partial"}),
	}); err != nil {
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
		if row.Role == "assistant" && row.Content == "continued text" && row.ParentID == userID {
			found = true
		}
	}
	if !found {
		t.Fatalf("continuation did not branch under the user row: %+v", msgs.rows)
	}
}
