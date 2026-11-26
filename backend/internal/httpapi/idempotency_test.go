package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

func TestSendMessageIdempotencyAndRunStatus(t *testing.T) {
	m := testmodel.New().Respond(model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "answer"},
		Usage:   model.Usage{InputTokens: 2, OutputTokens: 1},
	})
	agent := newTestAgent(t, m)
	usage := newFakeUsage()
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, usage)
	conv := convs.mustCreate("u-1", "")

	body := map[string]any{"content": "hello", "clientMessageId": "key-1"}
	rec := doJSON(h, http.MethodPost, "/api/conversations/"+conv.ID+"/messages", token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("first send: %d %s", rec.Code, rec.Body.String())
	}
	before := msgs.count()

	rec = doJSON(h, http.MethodPost, "/api/conversations/"+conv.ID+"/messages", token, body)
	if rec.Code != http.StatusConflict || authErrorCode(t, rec) != "duplicate_send" {
		t.Fatalf("duplicate: %d %s", rec.Code, rec.Body.String())
	}
	if msgs.count() != before {
		t.Fatalf("duplicate persisted %d new rows", msgs.count()-before)
	}

	rec = doJSON(h, http.MethodGet, "/api/conversations/"+conv.ID+"/run", token, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"active":false}` {
		t.Fatalf("run status: %d %s", rec.Code, rec.Body.String())
	}
}
