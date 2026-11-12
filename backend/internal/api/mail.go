package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sysvo/knot/internal/auth"
	"github.com/sysvo/knot/internal/mail"
)

// Settings keys that describe the relay.
const (
	settingSMTPHost = "smtp_host"
	settingSMTPPort = "smtp_port"
	settingSMTPUser = "smtp_user"
	settingSMTPPass = "smtp_pass"
	settingSMTPFrom = "smtp_from"
)

// smtpConfig resolves the relay to use right now.
//
// The admin panel's settings win over the environment, so a self-hoster can fix
// a typo without a shell. Environment values still apply when the database has
// nothing, which keeps container-only deployments working.
func (s *Server) smtpConfig(ctx context.Context) mail.Config {
	cfg := s.cfg.SMTP

	all, err := s.store.Settings(ctx)
	if err != nil {
		s.log.Error("read mail settings", "err", err)
		return cfg
	}

	if v := strings.TrimSpace(all[settingSMTPHost]); v != "" {
		cfg.Host = v
	}
	if v := strings.TrimSpace(all[settingSMTPPort]); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < 65536 {
			cfg.Port = n
		}
	}
	if v := strings.TrimSpace(all[settingSMTPUser]); v != "" {
		cfg.User = v
	}
	if v := strings.TrimSpace(all[settingSMTPPass]); v != "" {
		cfg.Pass = v
	}
	if v := strings.TrimSpace(all[settingSMTPFrom]); v != "" {
		cfg.From = v
	}
	return cfg
}

// queueMailTo puts a message in the outbox and wakes the worker.
//
// Nothing is sent during a request any more: an unreachable relay used to make
// sign-up hang for the length of a TCP timeout, and a refused message was lost.
func (s *Server) queueMailTo(ctx context.Context, to, subject, body string) {
	if _, err := s.store.EnqueueMail(ctx, to, subject, body, time.Now().Unix()); err != nil {
		s.log.Error("queue mail", "err", err, "to", to)
		return
	}
	s.kickMail()
}

// kickMail nudges the worker without blocking the caller; a full channel means a
// wake-up is already pending, which is all that is needed.
func (s *Server) kickMail() {
	select {
	case s.mailKick <- struct{}{}:
	default:
	}
}

// processOutbox attempts delivery of the messages that are due.
func (s *Server) processOutbox(ctx context.Context, limit int) {
	due, err := s.store.DueMail(ctx, time.Now().Unix(), limit)
	if err != nil {
		s.log.Error("read mail queue", "err", err)
		return
	}
	if len(due) == 0 {
		return
	}

	cfg := s.smtpConfig(ctx)
	if !cfg.Enabled() {
		// Leave the queue exactly as it is: the messages are still there when
		// mail is configured, and the admin panel shows them waiting. Burning
		// attempts here would mark them failed for a problem the operator is
		// already being told about.
		s.log.Warn("mail waiting: no SMTP server configured", "queued", len(due))
		return
	}

	for _, m := range due {
		sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.mail.Send(sendCtx, cfg, m.To, m.Subject, m.Body)
		cancel()

		if err == nil {
			if err := s.store.MarkMailSent(ctx, m.ID); err != nil {
				s.log.Error("mark mail sent", "err", err, "id", m.ID)
			}
			continue
		}

		attempts := m.Attempts + 1
		next := time.Now().Add(mail.Backoff(attempts)).Unix()
		if err := s.store.MarkMailAttempt(ctx, m.ID, err.Error(), next); err != nil {
			s.log.Error("mark mail attempt", "err", err, "id", m.ID)
		}
		s.log.Warn("mail delivery failed",
			"id", m.ID, "attempt", attempts, "max", mail.MaxAttempts, "err", err)
	}
}

// mailWorker delivers queued mail until the context is cancelled.
func (s *Server) mailWorker(ctx context.Context) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.mailKick:
		}
		s.processOutbox(ctx, 10)
	}
}

// ---- admin handlers ----

type testMailRequest struct {
	To string `json:"to"`
}

// handleAdminTestMail sends one message synchronously and reports the real
// error, because the whole point is to find out what is wrong.
func (s *Server) handleAdminTestMail(w http.ResponseWriter, r *http.Request) {
	admin := s.mustAdmin(w, r)
	if admin == nil {
		return
	}
	if !s.allow(w, r, "mail-test", 10, 5) {
		return
	}

	var req testMailRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	to := strings.TrimSpace(req.To)
	if to == "" || auth.ValidateEmail(to) != nil {
		writeError(w, http.StatusBadRequest, "invalid_email", "enter an address to send the test to")
		return
	}

	cfg := s.smtpConfig(r.Context())
	if !cfg.Enabled() {
		writeError(w, http.StatusBadRequest, "mail_not_configured",
			"set an SMTP host first, then send a test")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	site := s.getSetting(r.Context(), settingSiteName, "knot")
	err := s.mail.Send(ctx, cfg, to, "Test message from "+site,
		"This is a test message from "+site+".\n\nIf you are reading it, the mail settings work.\n")
	if err != nil {
		s.log.Warn("test mail failed", "to", to, "err", err)
		writeError(w, http.StatusBadGateway, "smtp_error", err.Error())
		return
	}

	_ = s.store.Log(r.Context(), admin.ID, "admin.mail.test", "settings", 0, to)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleAdminMail lists what is waiting and what has given up.
func (s *Server) handleAdminMail(w http.ResponseWriter, r *http.Request) {
	if s.mustAdmin(w, r) == nil {
		return
	}

	pending, failed, err := s.store.MailStats(r.Context(), mail.MaxAttempts)
	if err != nil {
		writeStoreError(s, w, r, err, "could not read the mail queue")
		return
	}
	items, err := s.store.UnsentMail(r.Context(), 50)
	if err != nil {
		writeStoreError(s, w, r, err, "could not read the mail queue")
		return
	}

	cfg := s.smtpConfig(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":   cfg.Enabled(),
		"pending":      pending,
		"failed":       failed,
		"max_attempts": mail.MaxAttempts,
		"items":        items,
	})
}

func (s *Server) handleAdminRetryMail(w http.ResponseWriter, r *http.Request) {
	admin := s.mustAdmin(w, r)
	if admin == nil {
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid message id")
		return
	}
	if err := s.store.RetryMail(r.Context(), id); err != nil {
		writeStoreError(s, w, r, err, "no such message")
		return
	}
	_ = s.store.Log(r.Context(), admin.ID, "admin.mail.retry", "mail", id, "")
	s.kickMail()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminDeleteMail(w http.ResponseWriter, r *http.Request) {
	admin := s.mustAdmin(w, r)
	if admin == nil {
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid message id")
		return
	}
	if err := s.store.DeleteMail(r.Context(), id); err != nil {
		writeStoreError(s, w, r, err, "no such message")
		return
	}
	_ = s.store.Log(r.Context(), admin.ID, "admin.mail.delete", "mail", id, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
