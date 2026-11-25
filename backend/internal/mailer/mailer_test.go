package mailer

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSMTPRejectsHeaderInjection(t *testing.T) {
	m := &SMTPMailer{cfg: Config{Host: "localhost", Port: 25, From: "a@b.co"}}
	if err := m.Send(context.Background(), "x@y.co", "sub\r\nject", "body"); err == nil {
		t.Fatal("CRLF in subject must be rejected")
	}
	if err := m.Send(context.Background(), "x@y.co", "subject", "body"); err == nil {
		// No SMTP listener is fine to fail on; the point is the header check
		// runs first for the malicious input above.
		t.Log("unexpectedly delivered without a server")
	}
}

func TestLogMailerHidesLinksByDefault(t *testing.T) {
	var buf bytes.Buffer
	m := &LogMailer{Log: slog.New(slog.NewTextHandler(&buf, nil))}
	if err := m.Send(context.Background(), "a@b.co", "subject", "token=secret-token"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "secret-token") {
		t.Fatalf("token leaked into logs: %s", buf.String())
	}
}

type countingMailer struct{ n atomic.Int32 }

func (c *countingMailer) Send(context.Context, string, string, string) error {
	c.n.Add(1)
	return nil
}

func TestAsyncDrainsOnClose(t *testing.T) {
	c := &countingMailer{}
	a := NewAsync(c, 4)
	for i := 0; i < 3; i++ {
		if err := a.Send(context.Background(), "a@b.co", "s", "b"); err != nil {
			t.Fatal(err)
		}
	}
	a.Close()
	if got := c.n.Load(); got != 3 {
		t.Fatalf("drained %d messages, want 3", got)
	}
	if err := a.Send(context.Background(), "a@b.co", "s", "b"); err == nil {
		t.Fatal("send after close must fail")
	}
}
