package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/testmodel"
)

func TestCompareCreatesSiblingBranches(t *testing.T) {
	m := testmodel.New().
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "answer A"}, Usage: model.Usage{InputTokens: 5, OutputTokens: 2}}).
		Respond(model.Response{Message: model.Message{Role: model.RoleAssistant, Content: "answer B"}, Usage: model.Usage{InputTokens: 6, OutputTokens: 3}})
	agent := newTestAgent(t, m)
	usage := newFakeUsage()
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, usage)
	conv := convs.mustCreate("u-1", "")

	rec := doJSON(h, http.MethodPost, "/api/conversations/"+conv.ID+"/compare", token, map[string]any{
		"content": "compare this",
		"models":  []string{"gemini-3.5-flash", "gemini-3.5-pro"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("compare: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		PromptMessageID string `json:"promptMessageId"`
		Results         []struct {
			Model     string `json:"model"`
			MessageID string `json:"messageId"`
			Content   string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Results) != 2 || body.Results[0].MessageID == body.Results[1].MessageID {
		t.Fatalf("results: %+v", body.Results)
	}
	if body.Results[0].Content != "answer A" || body.Results[1].Content != "answer B" {
		t.Fatalf("contents: %+v", body.Results)
	}
	var assistants int
	for _, row := range msgs.rows {
		if row.Role == "assistant" && row.ParentID == body.PromptMessageID {
			assistants++
		}
	}
	if assistants != 2 {
		t.Fatalf("assistant siblings = %d, want 2: %+v", assistants, msgs.rows)
	}
	if len(usage.events) != 2 {
		t.Fatalf("usage events = %d", len(usage.events))
	}
	convAfter := convs.convs[conv.ID]
	if convAfter.ActiveLeafID == nil || *convAfter.ActiveLeafID != body.PromptMessageID {
		t.Fatalf("active leaf should stay on the prompt until a winner is picked: %+v", convAfter.ActiveLeafID)
	}
}

func TestCompareValidatesModels(t *testing.T) {
	agent := newTestAgent(t, testmodel.New())
	msgs := newFakeMsgs()
	convs := newFakeConvos(msgs)
	h, token := newHandlerServer(t, agent, convs, msgs, newFakeUsage())
	conv := convs.mustCreate("u-1", "")

	for _, models := range [][]string{{"gemini-3.5-flash"}, {"gemini-3.5-flash", "gemini-3.5-flash"}, {"auto", "gemini-3.5-pro"}} {
		rec := doJSON(h, http.MethodPost, "/api/conversations/"+conv.ID+"/compare", token, map[string]any{
			"content": "hi", "models": models,
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("models %v -> %d", models, rec.Code)
		}
	}
}
