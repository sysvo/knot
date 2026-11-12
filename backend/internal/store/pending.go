package store

import (
	"context"
	"database/sql"

	"github.com/sysvo/knot/internal/model"
)

// PendingPost is a reply held for review, together with where it belongs so a
// moderator can judge it without opening three pages.
type PendingPost struct {
	Post        model.Post `json:"post"`
	ThreadTitle string     `json:"thread_title"`
	BoardSlug   string     `json:"board_slug"`
}

// PendingThreads lists threads waiting for review, oldest first so nothing
// starves at the back of a busy queue. A thread's first post travels with it:
// approving the thread publishes both.
func (s *Store) PendingThreads(ctx context.Context, limit int) ([]model.Thread, error) {
	rows, err := s.db.QueryContext(ctx,
		threadSelect()+` WHERE t.approved_at IS NULL AND t.deleted_at IS NULL
		 ORDER BY t.created_at, t.id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Thread{}
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// PendingPosts lists replies waiting for review inside threads that are already
// published. Replies to a thread that is itself unpublished surface through
// PendingThreads instead, so the queue never shows the same item twice.
func (s *Store) PendingPosts(ctx context.Context, limit int) ([]PendingPost, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.thread_id, p.body_md, p.body_html, p.score,
		       p.created_at, p.edited_at, (p.deleted_at IS NOT NULL), (p.approved_at IS NULL),
		       0, `+userCols("u")+`, t.title, b.slug
		FROM posts p
		LEFT JOIN users u ON u.id = p.author_id
		JOIN threads t ON t.id = p.thread_id
		JOIN boards b ON b.id = t.board_id
		WHERE p.approved_at IS NULL AND p.deleted_at IS NULL AND t.approved_at IS NOT NULL
		ORDER BY p.created_at, p.id
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PendingPost{}
	for rows.Next() {
		var (
			item             PendingPost
			deleted, pending int
			edited           sql.NullInt64
			score            int
			author           userRow
		)
		dest := []any{&item.Post.ID, &item.Post.ThreadID, &item.Post.BodyMD,
			&item.Post.BodyHTML, &item.Post.Score, &item.Post.CreatedAt,
			&edited, &deleted, &pending, &score}
		dest = append(dest, author.dest()...)
		dest = append(dest, &item.ThreadTitle, &item.BoardSlug)

		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		item.Post.Deleted = deleted != 0
		item.Post.Pending = pending != 0
		if edited.Valid {
			v := edited.Int64
			item.Post.EditedAt = &v
		}
		item.Post.Author = author.user()
		out = append(out, item)
	}
	return out, rows.Err()
}

// PendingCounts reports how many items are waiting for review, using the same
// definition as the queue itself.
func (s *Store) PendingCounts(ctx context.Context) (threads, posts int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM threads
		         WHERE approved_at IS NULL AND deleted_at IS NULL),
		       (SELECT count(*) FROM posts p JOIN threads t ON t.id = p.thread_id
		         WHERE p.approved_at IS NULL AND p.deleted_at IS NULL
		           AND t.approved_at IS NOT NULL)`,
	).Scan(&threads, &posts)
	return threads, posts, err
}
