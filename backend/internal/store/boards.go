package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sysvo/knot/internal/model"
)

const boardSelect = `
	SELECT b.id, b.slug, b.name, b.description, b.position, b.created_at,
	       (SELECT count(*) FROM threads t
	         WHERE t.board_id = b.id AND t.deleted_at IS NULL),
	       (SELECT count(*) FROM posts p JOIN threads t2 ON t2.id = p.thread_id
	         WHERE t2.board_id = b.id AND p.deleted_at IS NULL AND t2.deleted_at IS NULL),
	       COALESCE((SELECT max(t3.last_post_at) FROM threads t3
	         WHERE t3.board_id = b.id AND t3.deleted_at IS NULL), 0)
	FROM boards b`

func scanBoard(row interface{ Scan(...any) error }) (*model.Board, error) {
	var b model.Board
	err := row.Scan(&b.ID, &b.Slug, &b.Name, &b.Description, &b.Position, &b.CreatedAt,
		&b.ThreadCount, &b.PostCount, &b.LastPostAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBoards returns every board with its thread and post counts.
func (s *Store) ListBoards(ctx context.Context) ([]model.Board, error) {
	rows, err := s.db.QueryContext(ctx, boardSelect+` ORDER BY b.position, b.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Board{}
	for rows.Next() {
		b, err := scanBoard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// BoardBySlug loads one board by its URL slug.
func (s *Store) BoardBySlug(ctx context.Context, slug string) (*model.Board, error) {
	return scanBoard(s.db.QueryRowContext(ctx, boardSelect+` WHERE b.slug = ? COLLATE NOCASE`, slug))
}

// BoardByID loads one board by id.
func (s *Store) BoardByID(ctx context.Context, id int64) (*model.Board, error) {
	return scanBoard(s.db.QueryRowContext(ctx, boardSelect+` WHERE b.id = ?`, id))
}

// CreateBoard adds a board. Slug and name are validated by the caller.
func (s *Store) CreateBoard(ctx context.Context, slug, name, description string, position int) (*model.Board, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO boards (slug, name, description, position, created_at) VALUES (?, ?, ?, ?, ?)`,
		slug, name, description, position, time.Now().Unix())
	if isUniqueViolation(err, "slug") {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.BoardByID(ctx, id)
}

// UpdateBoard changes a board's presentation.
func (s *Store) UpdateBoard(ctx context.Context, id int64, slug, name, description string, position int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE boards SET slug = ?, name = ?, description = ?, position = ? WHERE id = ?`,
		slug, name, description, position, id)
	if isUniqueViolation(err, "slug") {
		return ErrConflict
	}
	return err
}

// DeleteBoard removes a board and, by cascade, its threads and posts.
func (s *Store) DeleteBoard(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM boards WHERE id = ?`, id)
	return err
}
