-- Indexes for the visibility filter that every listing applies.
--
-- The predicates here match the WHERE clauses in the store word for word: a
-- partial index is only used when the planner can prove the query's conditions
-- imply the index's, so `deleted_at IS NULL AND approved_at IS NOT NULL` must
-- appear in the same form in both places.

-- Thread lists. One per sort order, because an index cannot satisfy a different
-- ORDER BY than the one it was built for.
CREATE INDEX IF NOT EXISTS threads_visible_recent
    ON threads (pinned DESC, last_post_at DESC, id DESC)
    WHERE deleted_at IS NULL AND approved_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS threads_visible_newest
    ON threads (pinned DESC, created_at DESC, id DESC)
    WHERE deleted_at IS NULL AND approved_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS threads_visible_popular
    ON threads (pinned DESC, post_count DESC, last_post_at DESC)
    WHERE deleted_at IS NULL AND approved_at IS NOT NULL;

-- Board pages and the per-board counters: one board's threads, in listing order.
CREATE INDEX IF NOT EXISTS threads_visible_board
    ON threads (board_id, pinned DESC, last_post_at DESC, id DESC)
    WHERE deleted_at IS NULL AND approved_at IS NOT NULL;

-- Counting published posts without reading the post bodies. These counts run on
-- the home page, every board page and the admin dashboard.
CREATE INDEX IF NOT EXISTS posts_visible
    ON posts (thread_id)
    WHERE deleted_at IS NULL AND approved_at IS NOT NULL;

-- The review queue and the "awaiting review" counter.
CREATE INDEX IF NOT EXISTS threads_pending
    ON threads (created_at DESC)
    WHERE approved_at IS NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS posts_pending
    ON posts (created_at DESC)
    WHERE approved_at IS NULL AND deleted_at IS NULL;

-- Profile pages list a member's posts newest first.
CREATE INDEX IF NOT EXISTS posts_visible_author
    ON posts (author_id, created_at DESC)
    WHERE deleted_at IS NULL AND approved_at IS NOT NULL;
