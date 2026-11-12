-- Spam defence.
--
-- Two ideas share this migration:
--
--   1. Content can be held for review before it is published. A NULL
--      approved_at means "waiting for a moderator". Every existing row is
--      backfilled to its creation time, because a migration that made the whole
--      forum invisible would be unforgettable and unforgivable.
--
--   2. New accounts are limited until they have proven themselves. The
--      thresholds live in settings so they can be tuned without a restart.

ALTER TABLE threads ADD COLUMN approved_at INTEGER;
ALTER TABLE posts ADD COLUMN approved_at INTEGER;

UPDATE threads SET approved_at = created_at WHERE approved_at IS NULL;
UPDATE posts SET approved_at = created_at WHERE approved_at IS NULL;

-- Partial indexes: the review queue only ever asks for the unpublished rows.
CREATE INDEX threads_pending ON threads (approved_at, created_at) WHERE approved_at IS NULL;
CREATE INDEX posts_pending ON posts (approved_at, created_at) WHERE approved_at IS NULL;

INSERT OR IGNORE INTO settings (key, value) VALUES
    ('spam_links_require_posts', '3'),
    ('spam_links_require_days', '2'),
    ('spam_new_posts_per_day', '20'),
    ('spam_approve_first_posts', '0'),
    ('spam_blocked_email_domains', '');
