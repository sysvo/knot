package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/sysvo/knot/internal/model"
)

// ---- Notifications ----

// Notify inserts a notification unless an identical one already exists, so a
// user is never told twice about the same post.
func (s *Store) Notify(ctx context.Context, userID, actorID int64, kind string, threadID, postID int64) error {
	if userID == actorID {
		return nil // do not notify people about their own actions
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (user_id, actor_id, kind, thread_id, post_id, created_at)
		 SELECT ?, ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (
		   SELECT 1 FROM notifications WHERE user_id = ? AND post_id = ? AND kind = ?
		 )`,
		userID, actorID, kind, threadID, postID, time.Now().Unix(), userID, postID, kind)
	return err
}

// ListNotifications returns the newest notifications for a user.
func (s *Store) ListNotifications(ctx context.Context, userID int64, limit int) ([]model.Notification, int64, error) {
	var unread int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = ? AND read_at IS NULL`, userID).Scan(&unread); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT n.id, n.kind, n.thread_id, n.post_id, n.created_at, n.read_at IS NOT NULL,
		        COALESCE(t.title, ''), COALESCE(substr(p.body_md, 1, 200), ''),
		        `+userCols("a")+`
		 FROM notifications n
		 LEFT JOIN threads t ON t.id = n.thread_id
		 LEFT JOIN posts p ON p.id = n.post_id
		 LEFT JOIN users a ON a.id = n.actor_id
		 WHERE n.user_id = ?
		 ORDER BY n.created_at DESC, n.id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []model.Notification{}
	for rows.Next() {
		var (
			n     model.Notification
			read  int
			actor userRow
		)
		dest := []any{&n.ID, &n.Kind, &n.ThreadID, &n.PostID, &n.CreatedAt, &read, &n.ThreadTitle, &n.Excerpt}
		dest = append(dest, actor.dest()...)
		if err := rows.Scan(dest...); err != nil {
			return nil, 0, err
		}
		n.Read = read != 0
		n.Actor = actor.user()
		out = append(out, n)
	}
	return out, unread, rows.Err()
}

// MarkNotificationsRead clears the unread flag, or deletes everything.
func (s *Store) MarkNotificationsRead(ctx context.Context, userID int64, deleteAll bool) error {
	if deleteAll {
		_, err := s.db.ExecContext(ctx, `DELETE FROM notifications WHERE user_id = ?`, userID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`,
		time.Now().Unix(), userID)
	return err
}

// ---- Reports ----

// CreateReport files a report about a post (threadID may be the same thread).
func (s *Store) CreateReport(ctx context.Context, reporterID int64, postID *int64, threadID *int64, reason, details string) (int64, error) {
	var pid, tid any
	if postID != nil {
		pid = *postID
	}
	if threadID != nil {
		tid = *threadID
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reports (reporter_id, post_id, thread_id, reason, details, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		reporterID, pid, tid, reason, details, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListReports returns reports, optionally only the open ones.
func (s *Store) ListReports(ctx context.Context, onlyOpen bool, limit, offset int) ([]model.Report, int64, error) {
	where := ""
	args := []any{}
	if onlyOpen {
		where = ` WHERE r.status = 'open'`
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM reports r`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.post_id, r.thread_id, r.reason, r.details, r.status, r.created_at,
		        COALESCE(t.title, ''), COALESCE(substr(p.body_md, 1, 240), ''),
		        `+userCols("ru")+`
		 FROM reports r
		 LEFT JOIN posts p ON p.id = r.post_id
		 LEFT JOIN threads t ON t.id = COALESCE(r.thread_id, p.thread_id)
		 LEFT JOIN users ru ON ru.id = r.reporter_id`+where+`
		 ORDER BY r.created_at DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []model.Report{}
	for rows.Next() {
		var (
			r        model.Report
			postID   sql.NullInt64
			threadID sql.NullInt64
			reporter userRow
		)
		dest := []any{&r.ID, &postID, &threadID, &r.Reason, &r.Details, &r.Status, &r.CreatedAt, &r.ThreadTitle, &r.Excerpt}
		dest = append(dest, reporter.dest()...)
		if err := rows.Scan(dest...); err != nil {
			return nil, 0, err
		}
		if postID.Valid {
			v := postID.Int64
			r.PostID = &v
		}
		if threadID.Valid {
			v := threadID.Int64
			r.ThreadID = &v
		}
		r.Reporter = reporter.user()
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// ResolveReport closes a report as resolved or dismissed.
func (s *Store) ResolveReport(ctx context.Context, id, by int64, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE reports SET status = ?, resolved_by = ?, resolved_at = ? WHERE id = ?`,
		status, by, time.Now().Unix(), id)
	return err
}

// ---- Uploads ----

// CreateUpload records a stored attachment.
func (s *Store) CreateUpload(ctx context.Context, userID int64, storedName, filename, contentType string, size int64) (*model.Upload, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO uploads (user_id, stored_name, filename, content_type, size, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, storedName, filename, contentType, size, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &model.Upload{ID: id, Filename: filename, ContentType: contentType, Size: size, URL: "/uploads/" + storedName}, nil
}

// ---- Settings ----

// Settings returns every site setting as a map.
func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetSettings writes settings inside one transaction.
func (s *Store) SetSettings(ctx context.Context, kv map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for k, v := range kv {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings (key, value) VALUES (?, ?)
			 ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- Audit ----

// AuditEntry is one recorded moderation action.
type AuditEntry struct {
	ID         int64       `json:"id"`
	Action     string      `json:"action"`
	TargetType string      `json:"target_type"`
	TargetID   int64       `json:"target_id"`
	Details    string      `json:"details"`
	CreatedAt  int64       `json:"created_at"`
	Actor      *model.User `json:"actor"`
}

// Log records a moderation action.
func (s *Store) Log(ctx context.Context, actorID int64, action, targetType string, targetID int64, details string) error {
	var actor any
	if actorID != 0 {
		actor = actorID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (actor_id, action, target_type, target_id, details, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		actor, action, targetType, targetID, details, time.Now().Unix())
	return err
}

// ListAudit returns the newest audit entries.
func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.action, a.target_type, a.target_id, a.details, a.created_at,
		        `+userCols("au")+`
		 FROM audit_log a
		 LEFT JOIN users au ON au.id = a.actor_id
		 ORDER BY a.created_at DESC, a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AuditEntry{}
	for rows.Next() {
		var (
			e     AuditEntry
			actor userRow
		)
		dest := []any{&e.ID, &e.Action, &e.TargetType, &e.TargetID, &e.Details, &e.CreatedAt}
		dest = append(dest, actor.dest()...)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		e.Actor = actor.user()
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- Admin statistics ----

// AdminStats is the summary shown at the top of the admin dashboard.
type AdminStats struct {
	Users        int64 `json:"users"`
	Threads      int64 `json:"threads"`
	Posts        int64 `json:"posts"`
	OpenReports  int64 `json:"open_reports"`
	PendingItems int64 `json:"pending_items"`
	BannedUsers  int64 `json:"banned_users"`
	PublishedAt  int64 `json:"published_at"`
}

// AdminStats gathers the dashboard counters in one round trip.
func (s *Store) AdminStats(ctx context.Context) (*AdminStats, error) {
	var st AdminStats
	counts := []struct {
		query string
		dst   *int64
	}{
		{`SELECT count(*) FROM users`, &st.Users},
		{`SELECT count(*) FROM threads WHERE deleted_at IS NULL AND approved_at IS NOT NULL`, &st.Threads},
		{`SELECT count(*) FROM posts WHERE deleted_at IS NULL AND approved_at IS NOT NULL`, &st.Posts},
		{`SELECT count(*) FROM reports WHERE status = 'open'`, &st.OpenReports},
		{`SELECT (SELECT count(*) FROM threads
		      WHERE approved_at IS NULL AND deleted_at IS NULL)
		      + (SELECT count(*) FROM posts p JOIN threads t ON t.id = p.thread_id
		         WHERE p.approved_at IS NULL AND p.deleted_at IS NULL
		           AND t.approved_at IS NOT NULL)`, &st.PendingItems},
		{`SELECT count(*) FROM users WHERE status = 'banned'`, &st.BannedUsers},
	}
	for _, c := range counts {
		if err := s.db.QueryRowContext(ctx, c.query).Scan(c.dst); err != nil {
			return nil, err
		}
	}
	st.PublishedAt = time.Now().Unix()
	return &st, nil
}

// CountAdmins is used to refuse demoting the last administrator.
func (s *Store) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role = 'admin'`).Scan(&n)
	return n, err
}

// ---- Search ----

// SitemapThread is one thread URL, without the joins a full thread listing needs.
type SitemapThread struct {
	ID         int64 `json:"id"`
	LastPostAt int64 `json:"last_post_at"`
}

// SitemapBoard is one board URL.
type SitemapBoard struct {
	Slug       string `json:"slug"`
	LastPostAt int64  `json:"last_post_at"`
}

// SitemapEntries returns the public URLs worth indexing.
func (s *Store) SitemapEntries(ctx context.Context, limit int) ([]SitemapThread, []SitemapBoard, error) {
	threadRows, err := s.db.QueryContext(ctx, `
		SELECT id, last_post_at FROM threads
		WHERE deleted_at IS NULL AND approved_at IS NOT NULL
		ORDER BY last_post_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, nil, err
	}
	defer threadRows.Close()

	threads := []SitemapThread{}
	for threadRows.Next() {
		var t SitemapThread
		if err := threadRows.Scan(&t.ID, &t.LastPostAt); err != nil {
			return nil, nil, err
		}
		threads = append(threads, t)
	}
	if err := threadRows.Err(); err != nil {
		return nil, nil, err
	}

	boardRows, err := s.db.QueryContext(ctx, `
		SELECT b.slug, COALESCE(max(t.last_post_at), b.created_at)
		FROM boards b
		LEFT JOIN threads t ON t.board_id = b.id
		     AND t.deleted_at IS NULL AND t.approved_at IS NOT NULL
		GROUP BY b.id
		ORDER BY b.position, b.name`)
	if err != nil {
		return nil, nil, err
	}
	defer boardRows.Close()

	boards := []SitemapBoard{}
	for boardRows.Next() {
		var b SitemapBoard
		if err := boardRows.Scan(&b.Slug, &b.LastPostAt); err != nil {
			return nil, nil, err
		}
		boards = append(boards, b)
	}
	return threads, boards, boardRows.Err()
}

// Search runs a full-text query. boardSlug restricts results to one board.
//
// The snippet marks the matched terms with control characters rather than <mark>
// tags. The indexed text is whatever a member typed, and FTS5 inserts the
// delimiters into it verbatim without escaping anything, so a snippet containing
// markup is a stored-XSS vector the moment a caller treats it as HTML. Markers are
// turned into tags by the API after the text has been escaped.
func (s *Store) Search(ctx context.Context, query, boardSlug string, limit int) ([]model.SearchHit, error) {
	match := ftsQuery(query)
	if match == "" {
		return []model.SearchHit{}, nil
	}

	sqlText := `
		SELECT search_index.thread_id, search_index.post_id, search_index.title, b.slug, b.name,
		       snippet(search_index, 1, char(2), char(3), '…', 24),
		       COALESCE(u.username, ''), t.last_post_at, t.created_at
		FROM search_index
		JOIN threads t ON t.id = search_index.thread_id
		JOIN boards b ON b.id = t.board_id
		LEFT JOIN posts p ON p.id = search_index.post_id
		LEFT JOIN users u ON u.id = p.author_id
		WHERE search_index MATCH ?
		  AND t.deleted_at IS NULL
		  AND t.approved_at IS NOT NULL
		  AND (p.id IS NULL OR (p.deleted_at IS NULL AND p.approved_at IS NOT NULL))`
	args := []any{match}
	if boardSlug != "" {
		sqlText += ` AND b.slug = ? COLLATE NOCASE`
		args = append(args, boardSlug)
	}
	sqlText += ` ORDER BY bm25(search_index) LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.SearchHit{}
	for rows.Next() {
		var h model.SearchHit
		if err := rows.Scan(&h.ThreadID, &h.PostID, &h.Title, &h.BoardSlug, &h.BoardName,
			&h.Snippet, &h.Author, &h.LastPostAt, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ftsQuery turns free text into a safe FTS5 MATCH expression.
//
// Everything that is not a letter or a digit acts as a separator rather than
// being deleted: the index tokenises "self-hosted" as two words, so joining it
// back into "selfhosted" would search for a token that cannot exist. Each term
// gets a prefix wildcard so results appear while the reader is still typing, and
// quoting means no character in the input can ever be read as an FTS operator.
func ftsQuery(q string) string {
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		terms = append(terms, `"`+f+`"*`)
	}
	return strings.Join(terms, " AND ")
}

// HealthCheck runs a trivial query, used by the container health check.
func (s *Store) HealthCheck(ctx context.Context) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&n); err != nil {
		return fmt.Errorf("database unavailable: %w", err)
	}
	return nil
}
