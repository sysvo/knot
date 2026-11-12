package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sysvo/knot/internal/model"
)

// Account is a user plus the private fields needed for authentication and for
// rendering the signed-in user's own settings page.
type Account struct {
	User          model.User
	Email         string
	EmailVerified bool
	PasswordHash  string
	Providers     []string
}

// Self converts the account to the private JSON view returned by /auth/me.
func (a *Account) Self() *model.Self {
	providers := a.Providers
	if providers == nil {
		providers = []string{}
	}
	return &model.Self{
		User:          a.User,
		Email:         a.Email,
		EmailVerified: a.EmailVerified,
		HasPassword:   a.PasswordHash != "",
		Providers:     providers,
	}
}

const accountQuery = `
	SELECT id, username, COALESCE(email, ''), COALESCE(password_hash, ''),
	       display_name, bio, avatar_url, role, status, email_verified, created_at
	FROM users`

func (s *Store) scanAccount(row interface{ Scan(...any) error }) (*Account, error) {
	var a Account
	err := row.Scan(
		&a.User.ID, &a.User.Username, &a.Email, &a.PasswordHash,
		&a.User.DisplayName, &a.User.Bio, &a.User.AvatarURL, &a.User.Role,
		&a.User.Status, &a.EmailVerified, &a.User.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AccountByID loads an account by primary key.
func (s *Store) AccountByID(ctx context.Context, id int64) (*Account, error) {
	a, err := s.scanAccount(s.db.QueryRowContext(ctx, accountQuery+` WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := s.loadProviders(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// AccountByUsername loads an account by username, case-insensitively.
func (s *Store) AccountByUsername(ctx context.Context, username string) (*Account, error) {
	a, err := s.scanAccount(s.db.QueryRowContext(ctx, accountQuery+` WHERE username = ? COLLATE NOCASE`, username))
	if err != nil {
		return nil, err
	}
	if err := s.loadProviders(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// AccountByEmail loads an account by e-mail address, case-insensitively.
func (s *Store) AccountByEmail(ctx context.Context, email string) (*Account, error) {
	a, err := s.scanAccount(s.db.QueryRowContext(ctx, accountQuery+` WHERE email = ? COLLATE NOCASE`, email))
	if err != nil {
		return nil, err
	}
	if err := s.loadProviders(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) loadProviders(ctx context.Context, a *Account) error {
	rows, err := s.db.QueryContext(ctx, `SELECT provider FROM oauth_accounts WHERE user_id = ? ORDER BY provider`, a.User.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return err
		}
		a.Providers = append(a.Providers, p)
	}
	return rows.Err()
}

// CreateAccount inserts a user. Email and passwordHash may be empty, in which
// case NULL is stored so the unique indexes do not collide.
func (s *Store) CreateAccount(ctx context.Context, username, email, passwordHash, role string, verified bool) (*Account, error) {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, email, password_hash, role, status, email_verified, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		username, nullableString(email), nullableString(passwordHash),
		role, model.StatusActive, boolInt(verified), now,
	)
	if err != nil {
		if isUniqueViolation(err, "username") {
			return nil, fmt.Errorf("%w: username %q", ErrConflict, username)
		}
		if isUniqueViolation(err, "email") {
			return nil, fmt.Errorf("%w: email %q", ErrConflict, email)
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.AccountByID(ctx, id)
}

// UpdateProfile saves the fields a user may edit on their own account.
func (s *Store) UpdateProfile(ctx context.Context, id int64, displayName, bio, avatarURL string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET display_name = ?, bio = ?, avatar_url = ? WHERE id = ?`,
		displayName, bio, avatarURL, id)
	return err
}

// UpdateAccount is used by the admin user editor.
func (s *Store) UpdateAccount(ctx context.Context, id int64, email, role, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET email = ?, role = ?, status = ? WHERE id = ?`,
		nullableString(email), role, status, id)
	if isUniqueViolation(err, "email") {
		return fmt.Errorf("%w: email %q", ErrConflict, email)
	}
	return err
}

// SetPassword replaces the stored password hash.
func (s *Store) SetPassword(ctx context.Context, id int64, hash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	return err
}

// SetEmailVerified records that the address was proven to belong to the user.
func (s *Store) SetEmailVerified(ctx context.Context, id int64, verified bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET email_verified = ? WHERE id = ?`, boolInt(verified), id)
	return err
}

// SetEmail updates the e-mail address and resets verification.
func (s *Store) SetEmail(ctx context.Context, id int64, email string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET email = ?, email_verified = 0 WHERE id = ?`, nullableString(email), id)
	if isUniqueViolation(err, "email") {
		return fmt.Errorf("%w: email %q", ErrConflict, email)
	}
	return err
}

// CountUsers returns the number of accounts, used to seed the first admin.
func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

// ListUsers returns accounts matching q (username or e-mail), newest first.
func (s *Store) ListUsers(ctx context.Context, q string, limit, offset int) ([]model.User, int64, error) {
	where := ""
	args := []any{}
	if q != "" {
		where = ` WHERE (username LIKE ? OR email LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like)
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, display_name, bio, avatar_url, role, status, created_at
		 FROM users`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.Role, &u.Status, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
