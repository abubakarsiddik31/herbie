package chat

import (
	"strings"
	"testing"
)

func TestBotSystemPrompt(t *testing.T) {
	got := BotSystemPrompt("Scout", "Track competitor pricing weekly.", []string{"prefers bullet lists", "timezone is Asia/Dhaka"})
	for _, want := range []string{
		"You are Scout", "persistent AI teammate",
		"Track competitor pricing weekly.",
		"1. prefers bullet lists", "2. timezone is Asia/Dhaka",
		"resumed much later",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q:\n%s", want, got)
		}
	}
	if empty := BotSystemPrompt("Bo", "", nil); strings.Contains(empty, "Things you remember") {
		t.Error("empty memories must not render the memories section")
	}
}
