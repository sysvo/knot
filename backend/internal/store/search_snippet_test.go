package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sysvo/knot/internal/db"
	"github.com/sysvo/knot/internal/model"
)

// newTestStore opens a real database with the embedded migrations, so the FTS5
// triggers and the snippet() call are exercised as they are in production.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(conn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(conn)
}

// TestSearchSnippetMarksMatchesWithoutTags inserts a post whose indexed text
// contains HTML and checks that the store adds no markup of its own. The snippet
// used to be built with <mark> delimiters, which handed the API a string that
// already looked like HTML and contained whatever the member typed — the
// escaping step downstream could not tell the two apart.
//
// The text itself is returned verbatim, including the angle brackets: this layer
// deals in plain text, and escaping happens once, in the API.
func TestSearchSnippetMarksMatchesWithoutTags(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	board, err := s.BoardBySlug(ctx, "general")
	if err != nil {
		t.Fatalf("load seeded board: %v", err)
	}
	acct, err := s.CreateAccount(ctx, "alice", "alice@example.com", "x", model.RoleMember, true)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	const payload = `<img src=x onerror=alert(1)> written here`
	thread, err := s.CreateThread(ctx, board.ID, acct.User.ID, "A thread", "",
		payload, "", payload, true)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	hits, err := s.Search(ctx, "written", "", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(hits))
	}
	if hits[0].ThreadID != thread.ID {
		t.Errorf("hit is for thread %d, want %d", hits[0].ThreadID, thread.ID)
	}

	snippet := hits[0].Snippet
	if strings.Contains(snippet, "<mark>") || strings.Contains(snippet, "</mark>") {
		t.Fatalf("store generated HTML in the snippet: %q", snippet)
	}
	if !strings.Contains(snippet, "\x02written\x03") {
		t.Errorf("snippet has no match markers: %q", snippet)
	}
	if !strings.HasPrefix(snippet, `<img src=x onerror=alert(1)> `) || !strings.HasSuffix(snippet, " here") {
		t.Errorf("snippet changed the text it is supposed to quote: %q", snippet)
	}
}

// TestSearchSnippetHidesHeldPosts guards the other half of the queue: a post
// waiting for review must not show up in search at all.
func TestSearchSnippetHidesHeldPosts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	board, err := s.BoardBySlug(ctx, "general")
	if err != nil {
		t.Fatalf("load seeded board: %v", err)
	}
	acct, err := s.CreateAccount(ctx, "bob", "bob@example.com", "x", model.RoleMember, true)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	const text = "nothing to see here"
	if _, err := s.CreateThread(ctx, board.ID, acct.User.ID, "Held thread", "",
		text, "", text, false); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	hits, err := s.Search(ctx, "nothing", "", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("held thread appeared in search: %+v", hits)
	}
}
