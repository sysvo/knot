package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sysvo/knot/internal/model"
)

func postSelect() string {
	return `SELECT p.id, p.thread_id, p.body_md, p.body_html, p.score,
	       p.created_at, p.edited_at, (p.deleted_at IS NOT NULL), (p.approved_at IS NULL),
	       COALESCE((SELECT v.value FROM post_votes v WHERE v.post_id = p.id AND v.user_id = ?), 0),
	       ` + userCols("u") + `
	FROM posts p
	LEFT JOIN users u ON u.id = p.author_id`
}

func scanPost(row interface{ Scan(...any) error }) (*model.Post, error) {
	var (
		p                model.Post
		deleted, pending int
		edited           sql.NullInt64
		author           userRow
	)
	dest := []any{&p.ID, &p.ThreadID, &p.BodyMD, &p.BodyHTML, &p.Score,
		&p.CreatedAt, &edited, &deleted, &pending, &p.MyVote}
	dest = append(dest, author.dest()...)

	err := row.Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	p.Deleted = deleted != 0
	p.Pending = pending != 0
	if edited.Valid {
		v := edited.Int64
		p.EditedAt = &v
	}
	p.Author = author.user()
	if p.Deleted {
		// Never leak the contents of a removed post, but keep its place in the
		// thread so replies still make sense.
		p.BodyMD, p.BodyHTML = "", ""
	}
	return &p, nil
}

// ListPosts pages through a thread in posting order. viewerID is 0 for
// anonymous readers and controls the per-post MyVote value. Posts held for
// review are counted but only returned when includePending is set.
func (s *Store) ListPosts(ctx context.Context, threadID, viewerID int64, limit, offset int, includePending bool) ([]model.Post, int64, error) {
	where := ` WHERE p.thread_id = ?`
	if !includePending {
		where += ` AND p.approved_at IS NOT NULL`
	}

	var total int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM posts p`+where, threadID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		postSelect()+where+` ORDER BY p.created_at, p.id LIMIT ? OFFSET ?`,
		viewerID, threadID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []model.Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	return out, total, rows.Err()
}

// PostByID loads one post.
func (s *Store) PostByID(ctx context.Context, id, viewerID int64) (*model.Post, error) {
	return scanPost(s.db.QueryRowContext(ctx, postSelect()+` WHERE p.id = ?`, viewerID, id))
}

// CreatePost appends a reply. When approved is false the reply is held for
// review and does not touch the thread's counters until it is approved.
func (s *Store) CreatePost(ctx context.Context, threadID, authorID int64, bodyMD, bodyHTML, searchText string, approved bool) (*model.Post, error) {
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
		`INSERT INTO posts (thread_id, author_id, body_md, body_html, search_text, created_at, approved_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		threadID, authorID, bodyMD, bodyHTML, searchText, now, approvedAt)
	if err != nil {
		return nil, err
	}
	postID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	if err := recountThread(ctx, tx, threadID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.PostByID(ctx, postID, authorID)
}

// ApprovePost publishes a reply that was held for review.
func (s *Store) ApprovePost(ctx context.Context, id int64) (*model.Post, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var threadID int64
	if err := tx.QueryRowContext(ctx, `SELECT thread_id FROM posts WHERE id = ?`, id).Scan(&threadID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE posts SET approved_at = ? WHERE id = ? AND approved_at IS NULL`, time.Now().Unix(), id); err != nil {
		return nil, err
	}
	if err := recountThread(ctx, tx, threadID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.PostByID(ctx, id, 0)
}

// UpdatePost rewrites a post's body and records the edit time.
func (s *Store) UpdatePost(ctx context.Context, id int64, bodyMD, bodyHTML, searchText string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE posts SET body_md = ?, body_html = ?, search_text = ?, edited_at = ? WHERE id = ?`,
		bodyMD, bodyHTML, searchText, time.Now().Unix(), id)
	return err
}

// SoftDeletePost blanks a post and fixes up the thread counters.
func (s *Store) SoftDeletePost(ctx context.Context, id, by int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var threadID int64
	var alreadyDeleted sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT thread_id, deleted_at FROM posts WHERE id = ?`, id).Scan(&threadID, &alreadyDeleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if alreadyDeleted.Valid {
		return nil
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE posts SET deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now().Unix(), by, id); err != nil {
		return err
	}
	// Recount so a deleted pending post does not decrement anything it never added.
	if err := recountThread(ctx, tx, threadID); err != nil {
		return err
	}
	return tx.Commit()
}

// VotePost records or clears a vote and returns the post's new score.
// value must be 1, -1, or 0 to remove the vote.
func (s *Store) VotePost(ctx context.Context, postID, userID int64, value int) (int64, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM posts WHERE id = ?`, postID).Scan(&exists)
	if err != nil {
		return 0, 0, err
	}
	if exists == 0 {
		return 0, 0, ErrNotFound
	}

	if value == 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM post_votes WHERE post_id = ? AND user_id = ?`, postID, userID); err != nil {
			return 0, 0, err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO post_votes (post_id, user_id, value, created_at) VALUES (?, ?, ?, ?)
			 ON CONFLICT (post_id, user_id) DO UPDATE SET value = excluded.value`,
			postID, userID, value, time.Now().Unix()); err != nil {
			return 0, 0, err
		}
	}

	// Recomputing from the vote table is cheap and cannot drift.
	if _, err := tx.ExecContext(ctx,
		`UPDATE posts SET score = (SELECT COALESCE(sum(value), 0) FROM post_votes WHERE post_id = ?)
		 WHERE id = ?`, postID, postID); err != nil {
		return 0, 0, err
	}

	var score int64
	if err := tx.QueryRowContext(ctx, `SELECT score FROM posts WHERE id = ?`, postID).Scan(&score); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return score, value, nil
}

// UserPostCount returns how many published posts a user has written. This is
// also the number the trust rules use, so held posts do not count.
func (s *Store) UserPostCount(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM posts
		 WHERE author_id = ? AND deleted_at IS NULL AND approved_at IS NOT NULL`, userID).Scan(&n)
	return n, err
}

// PostsInLastDay counts a user's recent posts, including held ones, so that
// posting into the review queue still consumes the daily quota.
func (s *Store) PostsInLastDay(ctx context.Context, userID int64, since int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM posts
		 WHERE author_id = ? AND deleted_at IS NULL AND created_at >= ?`, userID, since).Scan(&n)
	return n, err
}

// RecentPostsByUser feeds the profile page. Held posts stay private.
func (s *Store) RecentPostsByUser(ctx context.Context, userID int64, limit int) ([]model.Post, error) {
	rows, err := s.db.QueryContext(ctx,
		postSelect()+` WHERE p.author_id = ? AND p.deleted_at IS NULL AND p.approved_at IS NOT NULL
		 ORDER BY p.created_at DESC LIMIT ?`, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}
