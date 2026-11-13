package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// seedBulk fills the database without going through the store, so the fixture is
// one transaction instead of thousands.
func seedBulk(t testing.TB, s *Store, threads, postsPerThread int) {
	t.Helper()
	ctx := context.Background()

	board, err := s.BoardBySlug(ctx, "general")
	if err != nil {
		t.Fatalf("load seeded board: %v", err)
	}
	acct, err := s.CreateAccount(ctx, "planuser", "plan@example.com", "x", "member", true)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	for i := 0; i < threads; i++ {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO threads (board_id, author_id, title, excerpt, created_at, last_post_at, approved_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			board.ID, acct.User.ID, "thread", "excerpt", now-int64(i), now-int64(i), now)
		if err != nil {
			t.Fatalf("insert thread: %v", err)
		}
		threadID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("thread id: %v", err)
		}
		for j := 0; j < postsPerThread; j++ {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO posts (thread_id, author_id, body_md, body_html, search_text, created_at, approved_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				threadID, acct.User.ID, "body", "<p>body</p>", "body", now-int64(i), now); err != nil {
				t.Fatalf("insert post: %v", err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE threads SET post_count = (SELECT count(*) FROM posts WHERE thread_id = ?) WHERE id = ?`,
			threadID, threadID); err != nil {
			t.Fatalf("set post count: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `ANALYZE`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

// queryPlan returns SQLite's plan as one string.
func queryPlan(t *testing.T, s *Store, sqlText string, args ...any) string {
	t.Helper()
	rows, err := s.db.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+sqlText, args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var plan []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("plan rows: %v", err)
	}
	return strings.Join(plan, "\n")
}

// TestThreadListPlanUsesTheIndex is the regression guard for the slow thread list:
// it asserts the page of ids is picked by walking a visibility index rather than
// scanning every thread and sorting the result.
func TestThreadListPlanUsesTheIndex(t *testing.T) {
	s := newTestStore(t)
	seedBulk(t, s, 400, 3)

	cases := []struct {
		sort  ThreadSort
		index string
	}{
		{SortRecent, "threads_visible_recent"},
		{SortNewest, "threads_visible_newest"},
		{SortPopular, "threads_visible_popular"},
	}

	for _, tc := range cases {
		q := ThreadQuery{Sort: tc.sort, Limit: 20}
		sqlText, args := q.listSQL(q.Limit, q.Offset)
		plan := queryPlan(t, s, sqlText, args...)

		if scansThreads(plan) {
			t.Errorf("%s: plan scans the threads table instead of using an index:\n%s", tc.sort, plan)
		}
		if !strings.Contains(plan, tc.index) {
			t.Errorf("%s: plan does not use %s:\n%s", tc.sort, tc.index, plan)
		}
		// The hydration join does not promise to keep the subquery's order, so one
		// sort of the twenty rows already selected is expected — what must not happen
		// is sorting the whole table first.
		if !sortOnlyAfterPage(plan) {
			t.Errorf("%s: plan sorts rows while still selecting the page:\n%s", tc.sort, plan)
		}
	}
}

// scansThreads reports a plan line that reads the threads table without an index.
// `SCAN t USING INDEX …` is an index scan and is exactly what we want; a bare
// `SCAN t` is the full table scan this change removed.
func scansThreads(plan string) bool {
	for _, line := range strings.Split(plan, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SCAN t") && !strings.Contains(line, "USING") {
			return true
		}
	}
	return false
}

// sortOnlyAfterPage reports that any ORDER BY sort happens after the page has been
// limited, so it sorts one page rather than every row.
func sortOnlyAfterPage(plan string) bool {
	sortAt := strings.Index(plan, "USE TEMP B-TREE")
	if sortAt < 0 {
		return true
	}
	if strings.Count(plan, "USE TEMP B-TREE") > 1 {
		return false
	}
	pageAt := strings.Index(plan, "SCAN page")
	return pageAt >= 0 && sortAt > pageAt
}

// TestThreadListByBoardPlanUsesTheIndex covers the board page, which filters as
// well as orders.
func TestThreadListByBoardPlanUsesTheIndex(t *testing.T) {
	s := newTestStore(t)
	seedBulk(t, s, 400, 2)

	board, err := s.BoardBySlug(context.Background(), "general")
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	q := ThreadQuery{BoardID: &board.ID, Sort: SortRecent, Limit: 20}
	sqlText, args := q.listSQL(q.Limit, q.Offset)
	plan := queryPlan(t, s, sqlText, args...)

	if strings.Contains(plan, "SCAN t") {
		t.Errorf("board listing scans the threads table:\n%s", plan)
	}
	if !strings.Contains(plan, "threads_visible_board") && !strings.Contains(plan, "threads_visible_recent") {
		t.Errorf("board listing uses no visibility index:\n%s", plan)
	}
}

// TestBoardCountersDoNotTouchPosts is the guard for the home page: the board post
// count comes from threads.post_count and the counters are one grouped pass, so the
// plan must not mention the posts table and must not run a subquery per board.
func TestBoardCountersDoNotTouchPosts(t *testing.T) {
	s := newTestStore(t)
	seedBulk(t, s, 100, 3)

	plan := queryPlan(t, s, listBoardsSelect+` ORDER BY b.position, b.name`)
	if strings.Contains(strings.ToLower(plan), "posts") {
		t.Errorf("the board query still reads the posts table:\n%s", plan)
	}
	if strings.Contains(plan, "CORRELATED SCALAR SUBQUERY") {
		t.Errorf("the board counters still run a subquery per board:\n%s", plan)
	}
}

// TestBoardCountersMatchThePosts makes sure the denormalised sum agrees with the
// rows it stands for, which is the assumption the query change depends on.
func TestBoardCountersMatchThePosts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedBulk(t, s, 5, 4)

	boards, err := s.ListBoards(ctx)
	if err != nil {
		t.Fatalf("list boards: %v", err)
	}
	if len(boards) == 0 {
		t.Fatal("no boards")
	}

	var threads, posts int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM threads WHERE deleted_at IS NULL AND approved_at IS NOT NULL`).
		Scan(&threads); err != nil {
		t.Fatalf("count threads: %v", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM posts p JOIN threads t ON t.id = p.thread_id
		 WHERE p.deleted_at IS NULL AND p.approved_at IS NOT NULL
		   AND t.deleted_at IS NULL AND t.approved_at IS NOT NULL`).
		Scan(&posts); err != nil {
		t.Fatalf("count posts: %v", err)
	}

	var gotThreads, gotPosts int64
	for _, b := range boards {
		gotThreads += b.ThreadCount
		gotPosts += b.PostCount
	}
	if gotThreads != threads || gotPosts != posts {
		t.Errorf("boards report %d threads / %d posts, rows say %d / %d",
			gotThreads, gotPosts, threads, posts)
	}
}
