package httpapi

import "strings"

// maxQuoteRunes caps the server-side quote snapshot. The client never sends
// quote text: the server reads the target row, so an edited or deleted
// source cannot rewrite what a past turn saw.
const maxQuoteRunes = 2000

// composeUserContent is the single place that turns a stored raw user
// message plus its quote snapshot into the text the model receives. Send,
// edit, and regenerate all call it, and the result is what gets persisted in
// messages.data for history replay; messages.content stays the raw text for
// the transcript and search.
func composeUserContent(content, quoteText string) string {
	if quoteText == "" {
		return content
	}
	var b strings.Builder
	b.WriteString("[Replying to an earlier message]\n")
	for _, line := range strings.Split(strings.ReplaceAll(quoteText, "\r\n", "\n"), "\n") {
		b.WriteString("> ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(content)
	return b.String()
}

// snapshotQuote trims and caps the quoted row's raw content for storage.
func snapshotQuote(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if r := []rune(content); len(r) > maxQuoteRunes {
		return strings.TrimSpace(string(r[:maxQuoteRunes])) + "…"
	}
	return content
}
