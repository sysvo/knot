// Package store contains every SQL query in the application. Handlers never
// write SQL directly, which keeps the schema knowledge in one place.
package store

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/sysvo/knot/internal/model"
)

// Sentinel errors returned by the store. The HTTP layer maps them to status
// codes; nothing else needs to inspect driver errors.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("already exists")
)

// Store is a thin, typed wrapper around the database.
type Store struct {
	db *sql.DB
}

// New returns a Store backed by db.
func New(db *sql.DB) *Store { return &Store{db: db} }

// DB exposes the underlying handle for migrations and tests.
func (s *Store) DB() *sql.DB { return s.db }

// isUniqueViolation reports whether err is a UNIQUE constraint failure. The
// driver returns plain errors, so the constraint name is read from the text.
func isUniqueViolation(err error, column string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE constraint failed") {
		return false
	}
	return column == "" || strings.Contains(msg, column)
}

// nullableString turns "" into NULL so that unique indexes on optional columns
// allow more than one empty value.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// userCols is the projection used everywhere a user is embedded in a query.
// Joined users come from a LEFT JOIN, so every column can be NULL.
func userCols(alias string) string {
	return alias + ".id, " + alias + ".username, " + alias + ".display_name, " +
		alias + ".bio, " + alias + ".avatar_url, " + alias + ".role, " +
		alias + ".status, " + alias + ".created_at"
}

// userRow is a scan target for userCols that tolerates the NULLs produced by a
// LEFT JOIN when the author account no longer exists.
type userRow struct {
	id        sql.NullInt64
	username  sql.NullString
	display   sql.NullString
	bio       sql.NullString
	avatar    sql.NullString
	role      sql.NullString
	status    sql.NullString
	createdAt sql.NullInt64
}

// dest returns the scan destinations for the eight user columns.
func (r *userRow) dest() []any {
	return []any{&r.id, &r.username, &r.display, &r.bio, &r.avatar, &r.role, &r.status, &r.createdAt}
}

// user converts the scanned row, returning nil when there was no matching user.
func (r *userRow) user() *model.User {
	if !r.id.Valid {
		return nil
	}
	return &model.User{
		ID:          r.id.Int64,
		Username:    r.username.String,
		DisplayName: r.display.String,
		Bio:         r.bio.String,
		AvatarURL:   r.avatar.String,
		Role:        r.role.String,
		Status:      r.status.String,
		CreatedAt:   r.createdAt.Int64,
	}
}
