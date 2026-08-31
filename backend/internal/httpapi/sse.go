package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// sseSink writes the chat SSE protocol and implements chat.Sink.
type sseSink struct {
	w     http.ResponseWriter
	flush http.Flusher
}

func newSSESink(w http.ResponseWriter) (*sseSink, bool) {
	flush, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flush.Flush()
	return &sseSink{w: w, flush: flush}, true
}

func (s *sseSink) event(name string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, b); err != nil {
		return err
	}
	s.flush.Flush()
	return nil
}

func (s *sseSink) Delta(text string) error { return s.event("delta", map[string]string{"text": text}) }
func (s *sseSink) ModelStart()             { _ = s.event("meta", map[string]string{"type": "model_start"}) }
func (s *sseSink) ModelEnd(in, out int) {
	_ = s.event("meta", map[string]any{"type": "model_end", "inputTokens": in, "outputTokens": out})
}
func (s *sseSink) ToolStart(name string) {
	_ = s.event("meta", map[string]any{"type": "tool_start", "name": name})
}
func (s *sseSink) ToolEnd(name string, ok bool) {
	_ = s.event("meta", map[string]any{"type": "tool_end", "name": name, "ok": ok})
}
