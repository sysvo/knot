// Package mail sends transactional e-mail.
//
// Nothing here talks to the database or the HTTP layer: the caller supplies the
// SMTP configuration for each send, so changing the relay in the admin panel
// takes effect on the next message without a restart.
package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// ErrNotConfigured is returned when no SMTP host is set.
var ErrNotConfigured = errors.New("no SMTP server configured")

// Config describes an SMTP relay.
type Config struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

// Enabled reports whether this configuration can send anything.
func (c Config) Enabled() bool { return strings.TrimSpace(c.Host) != "" }

func (c Config) port() int {
	if c.Port <= 0 || c.Port > 65535 {
		return 587
	}
	return c.Port
}

func (c Config) from() string {
	if strings.TrimSpace(c.From) != "" {
		return c.From
	}
	return "knot@localhost"
}

// Mailer sends plain-text messages.
type Mailer struct {
	log *slog.Logger
}

// New builds a Mailer.
func New(log *slog.Logger) *Mailer { return &Mailer{log: log} }

// Send delivers one message using cfg.
//
// Errors are returned with enough detail to be useful in the admin panel: "smtp
// auth: 535 authentication failed" is the sentence that fixes the problem, and
// swallowing it is how a self-hoster spends an evening guessing.
func (m *Mailer) Send(ctx context.Context, cfg Config, to, subject, body string) error {
	if !cfg.Enabled() {
		return ErrNotConfigured
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.port()))
	msg := buildMessage(cfg.from(), to, subject, body)

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer conn.Close()

	// Give up if the far end stalls mid-conversation.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if cfg.User != "" {
		auth := smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(cfg.from()); err != nil {
		return fmt.Errorf("smtp sender rejected: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp recipient rejected: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp finish: %w", err)
	}
	return client.Quit()
}

// buildMessage assembles an RFC 5322 message with a plain-text body.
func buildMessage(from, to, subject, body string) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + encodeHeader(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return b.String()
}

// encodeHeader keeps the subject ASCII-safe; non-ASCII is encoded per RFC 2047.
func encodeHeader(s string) string {
	for _, r := range s {
		if r > 127 {
			return "=?utf-8?b?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
		}
	}
	return s
}
