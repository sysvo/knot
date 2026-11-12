package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/sysvo/knot/internal/db"
)

// openTestDB migrates a fresh database in a temporary directory.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return conn
}

// TestMigrateIsIdempotent proves that running migrations twice is a no-op,
// which is what happens on every restart of a deployed instance.
func TestMigrateIsIdempotent(t *testing.T) {
	conn := openTestDB(t)
	if err := db.Migrate(conn); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

// TestSeedBoards checks the seed migration landed.
func TestSeedBoards(t *testing.T) {
	conn := openTestDB(t)
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM boards`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("expected seeded boards, got %d", n)
	}
}

// TestFullTextSearch proves FTS5 is compiled into the driver and that the
// triggers keep the index in sync. Without this, search would silently return
// nothing while every other test still passed.
func TestFullTextSearch(t *testing.T) {
	conn := openTestDB(t)
	const ts = int64(1_700_000_000)

	if _, err := conn.Exec(`INSERT INTO users (username, created_at) VALUES ('alice', ?)`, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(
		`INSERT INTO threads (board_id, author_id, title, created_at, last_post_at)
		 VALUES (1, 1, 'Deploying knot behind Caddy', ?, ?)`, ts, ts,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(
		`INSERT INTO posts (thread_id, author_id, body_md, created_at)
		 VALUES (1, 1, 'Reverse proxies need the forwarded headers set.', ?)`, ts,
	); err != nil {
		t.Fatal(err)
	}

	var threadID, postID int64
	if err := conn.QueryRow(
		`SELECT thread_id, post_id FROM search_index WHERE search_index MATCH 'proxies'`,
	).Scan(&threadID, &postID); err != nil {
		t.Fatalf("search for 'proxies': %v", err)
	}
	if threadID != 1 {
		t.Fatalf("thread_id = %d, want 1", threadID)
	}

	var hits int
	if err := conn.QueryRow(
		`SELECT count(*) FROM search_index WHERE search_index MATCH 'deploy*'`,
	).Scan(&hits); err != nil {
		t.Fatalf("prefix search: %v", err)
	}
	if hits == 0 {
		t.Fatal("prefix search found nothing")
	}

	// The retitle trigger must update the indexed title, otherwise search
	// results show the old heading forever.
	if _, err := conn.Exec(`UPDATE threads SET title = 'Unrelated heading' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(
		`SELECT count(*) FROM search_index WHERE search_index MATCH 'caddy'`,
	).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatalf("stale title still indexed, %d hits", hits)
	}
}
