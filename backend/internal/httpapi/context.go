package httpapi

import (
	"context"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/chat"
	"github.com/abubakarsiddik31/golem/model"
	"github.com/abubakarsiddik31/golem/tool"
)

// contextBreakdown is the token accounting shown in the context meter. The
// total is exact when the provider offers a counting endpoint, otherwise it
// is the sum of the estimates.
type contextBreakdown struct {
	SystemTokens       int    `json:"systemTokens"`
	MemoryTokens       int    `json:"memoryTokens"`
	InstructionsTokens int    `json:"instructionsTokens"`
	HistoryTokens      int    `json:"historyTokens"`
	PromptTokens       int    `json:"promptTokens"`
	AttachmentTokens   int    `json:"attachmentTokens"`
	TotalTokens        int    `json:"totalTokens"`
	Exact              bool   `json:"exact"`
	Model              string `json:"model"`
}

const estimatePerAttachment = 1000

func countAttachments(history []model.Message, parts []model.Part) int {
	n := len(parts)
	for _, m := range history {
		n += len(m.Parts)
	}
	return n
}

// contextBreakdown prices the request runTurn is about to send: the assembled
// system prompt (including memory and user preferences), history, the new
// prompt, and attachments. Exact provider counting is attempted with a short
// timeout; any failure falls back to the 4-chars/token estimate.
func (s *Server) contextBreakdown(ctx context.Context, userID string, spec chat.RunSpec, history []model.Message, prompt string, parts []model.Part, tools []tool.Tool[chat.Deps]) contextBreakdown {
	b := contextBreakdown{
		SystemTokens:     chat.EstimateTextTokens(spec.SystemPrompt),
		HistoryTokens:    chat.EstimateHistoryTokens(history),
		PromptTokens:     chat.EstimateTextTokens(prompt),
		AttachmentTokens: countAttachments(history, parts) * estimatePerAttachment,
		Model:            spec.Model,
	}
	if s.deps.Memories != nil {
		if mems, err := s.deps.Memories.List(ctx, userID); err == nil {
			var sb strings.Builder
			for _, m := range mems {
				sb.WriteString("- ")
				sb.WriteString(m.Content)
				sb.WriteString("\n")
			}
			b.MemoryTokens = chat.EstimateTextTokens(sb.String())
		}
	}
	if s.deps.Profiles != nil {
		if instructions, err := s.deps.Profiles.Instructions(ctx, userID); err == nil {
			b.InstructionsTokens = chat.EstimateTextTokens(instructions)
		}
	}
	b.TotalTokens = b.SystemTokens + b.HistoryTokens + b.PromptTokens + b.AttachmentTokens

	agent := s.agentFor(ctx, userID)
	if agent == nil {
		return b
	}
	messages := make([]model.Message, 0, len(history)+2)
	messages = append(messages, model.Message{Role: model.RoleSystem, Content: chat.PromptFor(spec, tools)})
	messages = append(messages, history...)
	messages = append(messages, model.Message{Role: model.RoleUser, Content: prompt, Parts: parts})
	countCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if n, err := agent.CountInputTokens(countCtx, spec, messages, chat.ToolSpecs(tools)); err == nil && n > 0 {
		b.TotalTokens = n
		b.Exact = true
	}
	return b
}
