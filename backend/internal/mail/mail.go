// Package mail sends transactional e-mail. When no SMTP host is configured the
// message is written to the log instead, so development needs no mail server.
package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/sysvo/knot/internal/config"
)

// Mailer sends plain-text messages.
type Mailer struct {
	cfg config.SMTP
	log *slog.Logger
}

// New builds a Mailer from configuration.
func New(cfg config.SMTP, log *slog.Logger) *Mailer {
	return &Mailer{cfg: cfg, log: log}
}

// Functional reports whether real mail will be sent.
func (m *Mailer) Functional() bool { return m.cfg.Host != "" }

// Send delivers one message. Errors are returned to the caller so that, for
// example, a failed verification mail can be reported to the user.
func (m *Mailer) Send(ctx context.Context, to, subject, body string) error {
	if m.cfg.Host == "" {
		m.log.Info("mail (not sent, no SMTP configured)", "to", to, "subject", subject, "body", body)
		return nil
	}

	addr := net.JoinHostPort(m.cfg.Host, fmt.Sprint(m.cfg.Port))
	msg := buildMessage(m.cfg.From, to, subject, body)

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if m.cfg.User != "" {
		auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("smtp from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
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
