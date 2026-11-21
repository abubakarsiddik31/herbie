package httpapi

import (
	"context"
	"strings"
	"time"

	"github.com/abubakarsiddik31/golem-chatbot/internal/storage"
	"github.com/abubakarsiddik31/golem/model"
)

const (
	autoTitleTimeout    = 15 * time.Second
	autoTitleInputRunes = 4000
)

// TitleGenerator is the auto-titling seam; *chat.TitleGenerator is the
// production wiring and tests inject fakes.
type TitleGenerator interface {
	ModelName() string
	Generate(ctx context.Context, input string) (string, model.Usage, error)
}

// maybeAutotitle schedules one asynchronous title attempt. It runs only from
// finishRun/finishResumed, so failed and paused runs never spend title tokens,
// and it never blocks the response stream.
func (s *Server) maybeAutotitle(userID, convID string) {
	if s.deps.Titles == nil {
		return
	}
	go s.autotitle(userID, convID)
}

// autotitle re-reads the conversation so it cannot race a manual rename: the
// generated title only applies while the source is still heuristic. It uses
// the run's own timeout and leaves the heuristic title in place on any miss.
func (s *Server) autotitle(userID, convID string) {
	ctx, cancel := context.WithTimeout(context.Background(), autoTitleTimeout)
	defer cancel()

	conv, err := s.deps.Convos.ByID(ctx, convID, userID)
	if err != nil {
		return
	}
	// Bots and forks are stamped manual at creation; the source check covers
	// them and a user rename.
	if conv.BotID != nil || conv.TitleSource != "heuristic" {
		return
	}
	rows, err := s.deps.Msgs.ForConversation(ctx, convID, userID)
	if err != nil {
		return
	}
	leaf := ""
	if conv.ActiveLeafID != nil {
		leaf = *conv.ActiveLeafID
	}
	if leaf == "" {
		leaf = latestRowID(rows)
	}
	path, err := s.pathToLeaf(ctx, convID, userID, leaf)
	if err != nil {
		return
	}

	// Only the conversation's first user turn: exactly one user row on the
	// active path. The auto source stamp makes a successful attempt final,
	// so retries after an edit/regenerate find a non-heuristic source.
	input := ""
	userRows := 0
	for _, m := range path {
		if m.Role != string(model.RoleUser) {
			continue
		}
		userRows++
		if userRows == 1 {
			input = m.Content
		}
	}
	if userRows != 1 {
		return
	}
	input = autoTitleInput(input)
	if input == "" {
		return
	}

	title, usage, err := s.deps.Titles.Generate(ctx, input)
	if err != nil {
		s.deps.Log.Warn("auto title failed", "err", err, "conv_id", convID)
		return
	}
	if err := s.deps.Convos.SetAutoTitle(ctx, convID, userID, title); err != nil {
		s.deps.Log.Warn("could not store auto title", "err", err, "conv_id", convID)
		return
	}
	cid := convID
	_ = s.deps.Usage.Add(ctx, storage.UsageEvent{
		UserID: userID, Kind: "title", Model: s.deps.Titles.ModelName(),
		ConversationID: &cid, InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		Requests: 1, CostMicros: s.deps.Rates.ChatCostMicrosFor(s.deps.Titles.ModelName(), usage.InputTokens, usage.OutputTokens),
	})
}

// autoTitleInput removes attached-file blocks (their full contents are not a
// title signal) and caps the prompt. It never includes memories, global
// instructions, or conversation settings.
func autoTitleInput(content string) string {
	content = fileBlockRe.ReplaceAllString(content, "")
	content = strings.TrimSpace(content)
	if r := []rune(content); len(r) > autoTitleInputRunes {
		content = strings.TrimSpace(string(r[:autoTitleInputRunes]))
	}
	return content
}
