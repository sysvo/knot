package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sysvo/knot/internal/model"
)

// CreateSession stores a session row. Only the hash of the token is kept, so a
// database leak does not hand out live sessions.
func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt int64, userAgent, ip string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at, user_agent, ip)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tokenHash, userID, time.Now().Unix(), expiresAt, userAgent, ip)
	return err
}

// SessionUser resolves a session token hash to its user, or (nil, nil) when the
// session is unknown or expired. Banned users lose access immediately even if
// their cookie is still valid.
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (*model.User, error) {
	var u model.User
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.display_name, u.bio, u.avatar_url, u.role, u.status, u.created_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = ? AND s.expires_at > ? AND u.status = ?`,
		tokenHash, time.Now().Unix(), model.StatusActive,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.Role, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// DeleteSession removes one session by token hash ("sign out").
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteOtherSessions signs out every session except the current one.
func (s *Store) DeleteOtherSessions(ctx context.Context, userID int64, keepHash string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, userID, keepHash)
	return err
}

// PurgeExpiredSessions removes stale session and login-token rows. It is called
// periodically so the tables do not grow without bound.
func (s *Store) PurgeExpiredSessions(ctx context.Context) error {
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM login_tokens WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_states WHERE expires_at <= ?`, now)
	return err
}

// CreateLoginToken stores a single-use token (magic link or verification).
func (s *Store) CreateLoginToken(ctx context.Context, tokenHash, email string, userID *int64, purpose string, expiresAt int64) error {
	var uid any
	if userID != nil {
		uid = *userID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO login_tokens (token_hash, user_id, email, purpose, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tokenHash, uid, email, purpose, time.Now().Unix(), expiresAt)
	return err
}

// ConsumeLoginToken atomically marks a token as used and returns its payload.
// Consuming twice fails, which is what makes a magic link single-use.
func (s *Store) ConsumeLoginToken(ctx context.Context, tokenHash, purpose string) (userID sql.NullInt64, email string, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return userID, "", err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE login_tokens SET used_at = ?
		 WHERE token_hash = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?`,
		time.Now().Unix(), tokenHash, purpose, time.Now().Unix())
	if err != nil {
		return userID, "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return userID, "", err
	}
	if n == 0 {
		return userID, "", ErrNotFound
	}

	if err := tx.QueryRowContext(ctx,
		`SELECT user_id, email FROM login_tokens WHERE token_hash = ?`, tokenHash,
	).Scan(&userID, &email); err != nil {
		return userID, "", err
	}
	return userID, email, tx.Commit()
}

// ---- OAuth ----

// OAuthUser returns the account linked to a provider identity, if any.
func (s *Store) OAuthUser(ctx context.Context, provider, providerUserID string) (*Account, error) {
	var userID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM oauth_accounts WHERE provider = ? AND provider_user_id = ?`,
		provider, providerUserID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.AccountByID(ctx, userID)
}

// LinkOAuth attaches a provider identity to an existing account.
func (s *Store) LinkOAuth(ctx context.Context, provider, providerUserID string, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO oauth_accounts (provider, provider_user_id, user_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		provider, providerUserID, userID, time.Now().Unix())
	if isUniqueViolation(err, "") {
		return ErrConflict
	}
	return err
}

// CreateOAuthState stores the state parameter of an OAuth flow together with
// its PKCE verifier.
func (s *Store) CreateOAuthState(ctx context.Context, state, provider, verifier, redirectTo string, expiresAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO oauth_states (state, provider, verifier, redirect_to, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		state, provider, verifier, redirectTo, time.Now().Unix(), expiresAt)
	return err
}

// ConsumeOAuthState validates and deletes a state parameter in one transaction,
// so a replayed callback cannot be used twice.
func (s *Store) ConsumeOAuthState(ctx context.Context, state string) (provider, verifier, redirectTo string, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", "", err
	}
	defer tx.Rollback()

	var expiresAt int64
	err = tx.QueryRowContext(ctx,
		`SELECT provider, verifier, redirect_to, expires_at FROM oauth_states WHERE state = ?`, state,
	).Scan(&provider, &verifier, &redirectTo, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM oauth_states WHERE state = ?`, state); err != nil {
		return "", "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", "", err
	}
	if expiresAt <= time.Now().Unix() {
		return "", "", "", ErrNotFound
	}
	return provider, verifier, redirectTo, nil
}
