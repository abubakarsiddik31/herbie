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
// is the sum of the estimates. Memory and instruction rows are "of which"
// views of the assembled system prompt, not additional tokens.
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

func estimateAttachment(part model.Part) int {
	switch part.Kind {
	case model.PartImage:
		return 1000
	case model.PartAudio:
		return 1500
	case model.PartVideo, model.PartDocument:
		return 2000
	default:
		return 1000
	}
}

func estimateAttachments(history []model.Message, parts []model.Part) int {
	total := 0
	for _, m := range history {
		for _, p := range m.Parts {
			total += estimateAttachment(p)
		}
	}
	for _, p := range parts {
		total += estimateAttachment(p)
	}
	return total
}

// contextBreakdown prices the request runTurn is about to send: the assembled
// instructions (including memory and user preferences), history, the new
// prompt, and attachments. Exact provider counting is attempted with a short
// timeout; any failure falls back to the 4-chars/token estimate.
func (s *Server) contextBreakdown(ctx context.Context, userID string, spec chat.RunSpec, history []model.Message, prompt string, parts []model.Part, tools []tool.Tool[chat.Deps]) contextBreakdown {
	instructions := chat.PromptFor(spec, tools)
	b := contextBreakdown{
		SystemTokens:     chat.EstimateTextTokens(instructions),
		HistoryTokens:    chat.EstimateHistoryTextTokens(history),
		PromptTokens:     chat.EstimateTextTokens(prompt),
		AttachmentTokens: estimateAttachments(history, parts),
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
	messages, err := chat.RequestMessages(ctx, instructions, history, prompt, parts)
	if err != nil {
		if s.deps.Log != nil {
			s.deps.Log.Debug("exact token count unavailable", "model", spec.Model, "err", err)
		}
		return b
	}
	countCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	if n, err := agent.CountInputTokens(countCtx, spec, messages, chat.ToolSpecs(tools)); err == nil && n > 0 {
		b.TotalTokens = n
		b.Exact = true
	} else if err != nil && s.deps.Log != nil {
		s.deps.Log.Debug("exact token count failed; using estimate", "model", spec.Model, "err", err)
	}
	return b
}
