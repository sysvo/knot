package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// QueuedMail is one row of the outbox.
type QueuedMail struct {
	ID        int64  `json:"id"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
	CreatedAt int64  `json:"created_at"`
	NextTryAt int64  `json:"next_attempt_at"`
	Sent      bool   `json:"sent"`
}

// EnqueueMail stores a message for the worker. dueAt is when the first attempt
// may happen, which is normally now.
func (s *Store) EnqueueMail(ctx context.Context, to, subject, body string, dueAt int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO mail_outbox (to_addr, subject, body, next_attempt_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		to, subject, body, dueAt, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MailByID loads one message, body included.
//
// The queue listing redacts links; this is the deliberate, audited read behind
// "show me the message" for an operator debugging a delivery.
func (s *Store) MailByID(ctx context.Context, id int64) (*QueuedMail, error) {
	var m QueuedMail
	err := s.db.QueryRowContext(ctx,
		`SELECT id, to_addr, subject, body, attempts, last_error, created_at, next_attempt_at
		 FROM mail_outbox WHERE id = ?`, id).
		Scan(&m.ID, &m.To, &m.Subject, &m.Body, &m.Attempts, &m.LastError, &m.CreatedAt, &m.NextTryAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DueMail returns unsent messages whose next attempt has arrived.
func (s *Store) DueMail(ctx context.Context, now int64, limit int) ([]QueuedMail, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, to_addr, subject, body, attempts, last_error, created_at, next_attempt_at
		 FROM mail_outbox
		 WHERE sent_at IS NULL AND next_attempt_at <= ?
		 ORDER BY next_attempt_at, id
		 LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []QueuedMail{}
	for rows.Next() {
		var m QueuedMail
		if err := rows.Scan(&m.ID, &m.To, &m.Subject, &m.Body, &m.Attempts,
			&m.LastError, &m.CreatedAt, &m.NextTryAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkMailSent records delivery.
func (s *Store) MarkMailSent(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE mail_outbox SET sent_at = ?, last_error = '' WHERE id = ?`,
		time.Now().Unix(), id)
	return err
}

// MarkMailAttempt records a failed attempt and when to try again.
func (s *Store) MarkMailAttempt(ctx context.Context, id int64, errText string, nextAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE mail_outbox SET attempts = attempts + 1, last_error = ?, next_attempt_at = ?
		 WHERE id = ?`, errText, nextAt, id)
	return err
}

// UnsentMail lists messages still in the outbox, newest first. It includes the
// body: an administrator debugging "no mail arrives" needs to see exactly what
// was going to be sent, and the outbox is the only place that exists.
func (s *Store) UnsentMail(ctx context.Context, limit int) ([]QueuedMail, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, to_addr, subject, body, attempts, last_error, created_at, next_attempt_at
		 FROM mail_outbox WHERE sent_at IS NULL
		 ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []QueuedMail{}
	for rows.Next() {
		var m QueuedMail
		if err := rows.Scan(&m.ID, &m.To, &m.Subject, &m.Body, &m.Attempts,
			&m.LastError, &m.CreatedAt, &m.NextTryAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MailStats counts what is waiting and what has given up.
func (s *Store) MailStats(ctx context.Context, maxAttempts int) (pending, failed int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT
		  (SELECT count(*) FROM mail_outbox WHERE sent_at IS NULL AND attempts < ?),
		  (SELECT count(*) FROM mail_outbox WHERE sent_at IS NULL AND attempts >= ?)`,
		maxAttempts, maxAttempts).Scan(&pending, &failed)
	return pending, failed, err
}

// RetryMail puts a failed message back at the front of the queue.
func (s *Store) RetryMail(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE mail_outbox SET attempts = 0, next_attempt_at = ?, last_error = ''
		 WHERE id = ? AND sent_at IS NULL`, time.Now().Unix(), id)
	return err
}

// DeleteMail removes a queued message, for one that should never be sent.
func (s *Store) DeleteMail(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM mail_outbox WHERE id = ?`, id)
	return err
}

// PruneMail deletes delivered messages older than the given cutoff, so the
// outbox does not grow without bound.
func (s *Store) PruneMail(ctx context.Context, before int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM mail_outbox WHERE sent_at IS NOT NULL AND sent_at < ?`, before)
	return err
}
