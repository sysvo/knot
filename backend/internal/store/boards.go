package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sysvo/knot/internal/model"
)

// boardColumns is the shared projection.
const boardColumns = `b.id, b.slug, b.name, b.description, b.position, b.created_at`

// boardSelect reads one board and its counters.
//
// The post count is summed from threads.post_count rather than counted through a
// join of every post in the board. That column is maintained by recountThread in
// the same transaction as every post mutation, so the sum is the same number the
// join produced — for the visible threads, which is what the board count means
// everywhere else — without reading the posts table at all.
const boardSelect = `
	SELECT ` + boardColumns + `,
	       (SELECT count(*) FROM threads t
	         WHERE t.board_id = b.id AND t.deleted_at IS NULL AND t.approved_at IS NOT NULL),
	       COALESCE((SELECT sum(t2.post_count) FROM threads t2
	         WHERE t2.board_id = b.id AND t2.deleted_at IS NULL AND t2.approved_at IS NOT NULL), 0),
	       COALESCE((SELECT max(t3.last_post_at) FROM threads t3
	         WHERE t3.board_id = b.id AND t3.deleted_at IS NULL
	           AND t3.approved_at IS NOT NULL), 0)
	FROM boards b`

// listBoardsSelect reads every board.
//
// The counters are one grouped pass over the visible threads, joined back to the
// boards, rather than three correlated subqueries per board: with a dozen boards
// the correlated form walked the same threads twelve times.
const listBoardsSelect = `
	SELECT ` + boardColumns + `,
	       COALESCE(agg.threads, 0), COALESCE(agg.posts, 0), COALESCE(agg.last_post_at, 0)
	FROM boards b
	LEFT JOIN (
	    SELECT board_id,
	           count(*)                AS threads,
	           sum(post_count)         AS posts,
	           max(last_post_at)       AS last_post_at
	    FROM threads
	    WHERE deleted_at IS NULL AND approved_at IS NOT NULL
	    GROUP BY board_id
	) agg ON agg.board_id = b.id`

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
	rows, err := s.db.QueryContext(ctx, listBoardsSelect+` ORDER BY b.position, b.name`)
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
