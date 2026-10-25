package chat

import (
	"fmt"
	"strings"
)

// BotSystemPrompt builds a bot's system prompt: identity, job, and compounding
// memories (most-recent-first). Background runs may resume long after the task
// was given, so the closing instruction tells the model the thread persists.
func BotSystemPrompt(name, instructions string, memories []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are %s, a persistent AI teammate the user keeps around.\n\nJob:\n%s\n", name, instructions)
	if len(memories) > 0 {
		b.WriteString("\nThings you remember (learned over time, most recent first):\n")
		for i, m := range memories {
			fmt.Fprintf(&b, "%d. %s\n", i+1, m)
		}
	}
	b.WriteString("\nYou may be resumed much later; continue the conversation where it left off and finish the task.")
	return b.String()
}
