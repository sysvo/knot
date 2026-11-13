package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sysvo/knot/internal/db"
)

// The benchmarks below compare the queries this change replaced with the ones it
// introduced, on the same data and the same machine. The old statements live here
// so the comparison stays reproducible: they are the "before" numbers in
// docs/ARCHITECTURE.md.
//
//	go test ./internal/store -bench . -benchtime 5x -run XXX

// oldBoardSelect is the previous board query, which counted posts through a join
// of every post in the board.
const oldBoardSelect = `
	SELECT b.id, b.slug, b.name, b.description, b.position, b.created_at,
	       (SELECT count(*) FROM threads t
	         WHERE t.board_id = b.id AND t.deleted_at IS NULL AND t.approved_at IS NOT NULL),
	       (SELECT count(*) FROM posts p JOIN threads t2 ON t2.id = p.thread_id
	         WHERE t2.board_id = b.id AND p.deleted_at IS NULL
	           AND p.approved_at IS NOT NULL AND t2.approved_at IS NOT NULL),
	       COALESCE((SELECT max(t3.last_post_at) FROM threads t3
	         WHERE t3.board_id = b.id AND t3.deleted_at IS NULL
	           AND t3.approved_at IS NOT NULL), 0)
	FROM boards b`

// oldThreadList is the previous thread listing: hydrate first, order afterwards.
func oldThreadList(q ThreadQuery) string {
	where := ` WHERE 1 = 1`
	if q.BoardID != nil {
		where += ` AND t.board_id = ?`
	}
	where += ` AND t.deleted_at IS NULL AND t.approved_at IS NOT NULL`
	return threadColumns() + ` FROM threads t` + threadJoins + where + q.Sort.orderBy() + ` LIMIT ? OFFSET ?`
}

// benchStore seeds a large database once for the whole process. Test cleanup
// cannot be used here: it runs at the end of the first benchmark, which closes the
// shared handle the later ones are still using.
func benchStore(b *testing.B) *Store {
	b.Helper()
	benchOnce.Do(func() {
		path := filepath.Join(os.TempDir(), fmt.Sprintf("knot-bench-%d.db", os.Getpid()))
		for _, suffix := range []string{"", "-wal", "-shm"} {
			os.Remove(path + suffix)
		}
		conn, err := db.Open(path)
		if err != nil {
			benchErr = err
			return
		}
		if err := db.Migrate(conn); err != nil {
			benchErr = err
			return
		}
		benchCached = New(conn)
		seedBulk(b, benchCached, 10000, 4)
	})
	if benchErr != nil {
		b.Fatalf("benchmark fixture: %v", benchErr)
	}
	return benchCached
}

var (
	benchOnce   sync.Once
	benchCached *Store
	benchErr    error
)

func BenchmarkListThreadsNew(b *testing.B) {
	s := benchStore(b)
	ctx := context.Background()
	q := ThreadQuery{Sort: SortRecent, Limit: 20}
	sqlText, args := q.listSQL(q.Limit, q.Offset)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := s.db.QueryContext(ctx, sqlText, args...)
		if err != nil {
			b.Fatal(err)
		}
		for rows.Next() {
		}
		rows.Close()
	}
}

func BenchmarkListThreadsOld(b *testing.B) {
	s := benchStore(b)
	ctx := context.Background()
	q := ThreadQuery{Sort: SortRecent, Limit: 20}
	sqlText := oldThreadList(q)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := s.db.QueryContext(ctx, sqlText, q.Limit, q.Offset)
		if err != nil {
			b.Fatal(err)
		}
		for rows.Next() {
		}
		rows.Close()
	}
}

func BenchmarkListBoardsNew(b *testing.B) {
	s := benchStore(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ListBoards(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkListBoardsOld(b *testing.B) {
	s := benchStore(b)
	ctx := context.Background()
	sqlText := oldBoardSelect + ` ORDER BY b.position, b.name`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := s.db.QueryContext(ctx, sqlText)
		if err != nil {
			b.Fatal(err)
		}
		for rows.Next() {
		}
		rows.Close()
	}
}

func BenchmarkAdminStats(b *testing.B) {
	s := benchStore(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.AdminStats(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearchCommonTerm(b *testing.B) {
	s := benchStore(b)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Search(ctx, "body", "", 30); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSearchBM25Order is the previous ranking, kept so the change to the
// hidden rank column can be checked rather than taken on faith.
func BenchmarkSearchBM25Order(b *testing.B) {
	s := benchStore(b)
	ctx := context.Background()
	sqlText, args := searchSQL(ftsQuery("body"), "", "bm25(search_index)", 30)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := s.db.QueryContext(ctx, sqlText, args...)
		if err != nil {
			b.Fatal(err)
		}
		for rows.Next() {
		}
		rows.Close()
	}
}
