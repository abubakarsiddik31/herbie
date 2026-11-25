// Package mailer sends transactional auth email. SMTP is optional: when it
// is not configured the log mailer records the attempt instead, and links
// are only printed when explicitly enabled for local development.
package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

type Config struct {
	Host           string
	Port           int
	User           string
	Pass           string
	From           string
	ImplicitTLS    bool
	AllowPlaintext bool
}

// New builds the configured mailer: SMTP when a host is set, otherwise the
// log mailer.
func New(cfg Config, log *slog.Logger, logLinks bool) Mailer {
	if strings.TrimSpace(cfg.Host) == "" {
		return &LogMailer{Log: log, LogLinks: logLinks}
	}
	return &SMTPMailer{cfg: cfg}
}

// LogMailer is the no-SMTP fallback. It never prints the body unless
// LogLinks is on, because bodies carry single-use tokens.
type LogMailer struct {
	Log      *slog.Logger
	LogLinks bool
}

func (m *LogMailer) Send(_ context.Context, to, subject, body string) error {
	if m.LogLinks {
		m.Log.Info("email (log mailer)", "to", to, "subject", subject, "body", body)
		return nil
	}
	m.Log.Info("email suppressed: SMTP not configured (set SMTP_HOST or EMAIL_LOG_LINKS=true for dev)", "to", to, "subject", subject)
	return nil
}

// SMTPMailer sends plain-text mail over implicit TLS (465) or STARTTLS
// (587/25). Plaintext auth is refused unless explicitly allowed.
type SMTPMailer struct {
	cfg Config
}

func rejectHeaderInjection(fields ...string) error {
	for _, f := range fields {
		if strings.ContainsAny(f, "\r\n") {
			return errors.New("mailer: header injection rejected")
		}
	}
	return nil
}

func (m *SMTPMailer) Send(_ context.Context, to, subject, body string) error {
	if err := rejectHeaderInjection(to, subject, m.cfg.From); err != nil {
		return err
	}
	toAddr, err := mail.ParseAddress(to)
	if err != nil {
		return fmt.Errorf("mailer: invalid recipient: %w", err)
	}
	fromAddr := m.cfg.From
	fromParsed, err := mail.ParseAddress(fromAddr)
	if err != nil {
		return fmt.Errorf("mailer: invalid sender %q: %w", fromAddr, err)
	}
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))
	tlsCfg := &tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}

	var client *smtp.Client
	if m.cfg.ImplicitTLS {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("mailer: dial tls: %w", err)
		}
		client, err = smtp.NewClient(conn, m.cfg.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("mailer: smtp client: %w", err)
		}
	} else {
		conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			return fmt.Errorf("mailer: dial: %w", err)
		}
		client, err = smtp.NewClient(conn, m.cfg.Host)
		if err != nil {
			conn.Close()
			return fmt.Errorf("mailer: smtp client: %w", err)
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsCfg); err != nil {
				client.Close()
				return fmt.Errorf("mailer: starttls: %w", err)
			}
		} else if !m.cfg.AllowPlaintext {
			client.Close()
			return errors.New("mailer: server does not offer STARTTLS and SMTP_ALLOW_PLAINTEXT is false")
		}
	}
	defer client.Close()

	if m.cfg.User != "" {
		auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("mailer: auth: %w", err)
		}
	}
	if err := client.Mail(fromParsed.Address); err != nil {
		return fmt.Errorf("mailer: mail from: %w", err)
	}
	if err := client.Rcpt(toAddr.Address); err != nil {
		return fmt.Errorf("mailer: rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mailer: data: %w", err)
	}
	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n",
		fromAddr, toAddr.Address, subject)
	if _, err := w.Write([]byte(headers + body)); err != nil {
		return fmt.Errorf("mailer: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: close body: %w", err)
	}
	return client.Quit()
}

// Async wraps a mailer with a bounded worker queue so a slow SMTP hop does
// not make account-recovery responses time-differ by whether the address
// exists. A full queue drops the message and reports the error to the caller.
type Async struct {
	inner  Mailer
	ch     chan asyncMsg
	done   chan struct{}
	mu     sync.RWMutex
	closed bool
}

type asyncMsg struct {
	to, subject, body string
}

func NewAsync(inner Mailer, size int) *Async {
	if size <= 0 {
		size = 128
	}
	a := &Async{inner: inner, ch: make(chan asyncMsg, size), done: make(chan struct{})}
	go func() {
		a.worker()
		close(a.done)
	}()
	return a
}

func (a *Async) worker() {
	for msg := range a.ch {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = a.inner.Send(ctx, msg.to, msg.subject, msg.body)
		cancel()
	}
}

func (a *Async) Send(_ context.Context, to, subject, body string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return errors.New("mailer: closed")
	}
	select {
	case a.ch <- asyncMsg{to: to, subject: subject, body: body}:
		return nil
	default:
		return errors.New("mailer: queue full")
	}
}

// Close stops accepting mail, drains the queue, then waits for the worker.
func (a *Async) Close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	close(a.ch)
	a.mu.Unlock()
	<-a.done
}
