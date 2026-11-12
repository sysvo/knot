package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sysvo/knot/internal/model"
)

func threadSelect() string {
	return `SELECT t.id, t.board_id, b.slug, b.name, t.title, t.pinned, t.locked,
	       (t.deleted_at IS NOT NULL), (t.approved_at IS NULL), t.excerpt,
	       t.post_count, t.view_count, t.created_at, t.last_post_at,
	       ` + userCols("u") + `,
	       ` + userCols("lu") + `
	FROM threads t
	JOIN boards b ON b.id = t.board_id
	LEFT JOIN users u ON u.id = t.author_id
	LEFT JOIN posts lp ON lp.id = t.last_post_id
	LEFT JOIN users lu ON lu.id = lp.author_id`
}

func scanThread(row interface{ Scan(...any) error }) (*model.Thread, error) {
	var (
		t                   model.Thread
		pinned, locked, del int
		pending             int
		author, lastAuthor  userRow
	)
	dest := []any{&t.ID, &t.BoardID, &t.BoardSlug, &t.BoardName, &t.Title,
		&pinned, &locked, &del, &pending, &t.Excerpt, &t.PostCount, &t.ViewCount,
		&t.CreatedAt, &t.LastPostAt}
	dest = append(dest, author.dest()...)
	dest = append(dest, lastAuthor.dest()...)

	err := row.Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	t.Pinned = pinned != 0
	t.Locked = locked != 0
	t.Deleted = del != 0
	t.Pending = pending != 0
	t.Author = author.user()
	t.LastAuthor = lastAuthor.user()
	return &t, nil
}

// ThreadSort is the order of a thread list.
type ThreadSort string

const (
	SortRecent  ThreadSort = "recent"
	SortNewest  ThreadSort = "newest"
	SortPopular ThreadSort = "popular"
)

func (s ThreadSort) orderBy() string {
	switch s {
	case SortNewest:
		return ` ORDER BY t.pinned DESC, t.created_at DESC, t.id DESC`
	case SortPopular:
		return ` ORDER BY t.pinned DESC, t.post_count DESC, t.last_post_at DESC`
	default:
		return ` ORDER BY t.pinned DESC, t.last_post_at DESC, t.id DESC`
	}
}

// ThreadQuery selects a page of threads. The zero value lists published,
// undeleted threads across every board, but Limit must still be set.
type ThreadQuery struct {
	BoardID        *int64
	Sort           ThreadSort
	Limit          int
	Offset         int
	IncludeDeleted bool
	IncludePending bool
}

// ListThreads pages through threads.
func (s *Store) ListThreads(ctx context.Context, q ThreadQuery) ([]model.Thread, int64, error) {
	where := ` WHERE 1 = 1`
	args := []any{}
	if q.BoardID != nil {
		where += ` AND t.board_id = ?`
		args = append(args, *q.BoardID)
	}
	if !q.IncludeDeleted {
		where += ` AND t.deleted_at IS NULL`
	}
	if !q.IncludePending {
		where += ` AND t.approved_at IS NOT NULL`
	}

	var total int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM threads t`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// threadSelect already joins boards and users, so the same WHERE applies.
	sqlText := threadSelect() + where + q.Sort.orderBy() + ` LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, sqlText, append(args, q.Limit, q.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []model.Thread{}
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
	}
	return out, total, rows.Err()
}

// ThreadByID loads a single thread with its board and participants.
func (s *Store) ThreadByID(ctx context.Context, id int64) (*model.Thread, error) {
	return scanThread(s.db.QueryRowContext(ctx, threadSelect()+` WHERE t.id = ?`, id))
}

// CreateThread inserts a thread and its first post in one transaction.
// searchText is the plain-text rendering used by the full-text index. When
// approved is false the thread and its first post are held for review.
func (s *Store) CreateThread(ctx context.Context, boardID, authorID int64, title, excerpt, bodyMD, bodyHTML, searchText string, approved bool) (*model.Thread, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	var approvedAt any
	if approved {
		approvedAt = now
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO threads (board_id, author_id, title, excerpt, created_at, last_post_at, approved_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		boardID, authorID, title, excerpt, now, now, approvedAt)
	if err != nil {
		return nil, err
	}
	threadID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO posts (thread_id, author_id, body_md, body_html, search_text, created_at, approved_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		threadID, authorID, bodyMD, bodyHTML, searchText, now, approvedAt); err != nil {
		return nil, err
	}

	if err := recountThread(ctx, tx, threadID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ThreadByID(ctx, threadID)
}

// ApproveThread publishes a thread held for review, together with the posts
// that were waiting with it.
func (s *Store) ApproveThread(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx,
		`UPDATE threads SET approved_at = ? WHERE id = ? AND approved_at IS NULL`, now, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE posts SET approved_at = ? WHERE thread_id = ? AND approved_at IS NULL`, now, id); err != nil {
		return err
	}
	if err := recountThread(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateThreadTitle changes a title; the search index follows via trigger.
func (s *Store) UpdateThreadTitle(ctx context.Context, id int64, title string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE threads SET title = ? WHERE id = ?`, title, id)
	return err
}

// SetThreadFlags pins or locks a thread.
func (s *Store) SetThreadFlags(ctx context.Context, id int64, pinned, locked bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE threads SET pinned = ?, locked = ? WHERE id = ?`, boolInt(pinned), boolInt(locked), id)
	return err
}

// SoftDeleteThread hides a thread but keeps its posts for moderation history.
func (s *Store) SoftDeleteThread(ctx context.Context, id int64, by int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE threads SET deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now().Unix(), by, id)
	return err
}

// IncrementThreadViews counts a read. It is fire-and-forget.
func (s *Store) IncrementThreadViews(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE threads SET view_count = view_count + 1 WHERE id = ?`, id)
	return err
}

// RecentThreads is the home page list: newest activity across all boards.
func (s *Store) RecentThreads(ctx context.Context, limit int) ([]model.Thread, error) {
	threads, _, err := s.ListThreads(ctx, ThreadQuery{Sort: SortRecent, Limit: limit})
	return threads, err
}

// execer is the subset of *sql.DB and *sql.Tx that recountThread needs, so the
// recount can run inside the transaction that changed the posts.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// recountThread recomputes a thread's counters from its posts.
//
// Publishing, approving and deleting all change what should be counted, and
// incremental arithmetic in three places eventually disagrees with the data.
// One query that derives the answer cannot drift.
func recountThread(ctx context.Context, db execer, threadID int64) error {
	_, err := db.ExecContext(ctx, `
		UPDATE threads SET
		    post_count = (
		        SELECT count(*) FROM posts
		        WHERE thread_id = ? AND deleted_at IS NULL AND approved_at IS NOT NULL
		    ),
		    last_post_id = (
		        SELECT id FROM posts
		        WHERE thread_id = ? AND deleted_at IS NULL AND approved_at IS NOT NULL
		        ORDER BY created_at DESC, id DESC LIMIT 1
		    ),
		    last_post_at = COALESCE(
		        (SELECT created_at FROM posts
		         WHERE thread_id = ? AND deleted_at IS NULL AND approved_at IS NOT NULL
		         ORDER BY created_at DESC, id DESC LIMIT 1),
		        created_at
		    )
		WHERE id = ?`,
		threadID, threadID, threadID, threadID)
	return err
}
