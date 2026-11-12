-- Core schema for knot.
--
-- Conventions:
--   * all timestamps are INTEGER unix seconds, UTC
--   * deletes that must preserve thread structure are soft (deleted_at)

CREATE TABLE users (
    id             INTEGER PRIMARY KEY,
    username       TEXT NOT NULL,
    email          TEXT,
    password_hash  TEXT,
    display_name   TEXT NOT NULL DEFAULT '',
    bio            TEXT NOT NULL DEFAULT '',
    avatar_url     TEXT NOT NULL DEFAULT '',
    role           TEXT NOT NULL DEFAULT 'member', -- member | moderator | admin
    status         TEXT NOT NULL DEFAULT 'active', -- active | banned
    email_verified INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL
);

CREATE UNIQUE INDEX users_username ON users (username COLLATE NOCASE);
CREATE UNIQUE INDEX users_email ON users (email COLLATE NOCASE) WHERE email IS NOT NULL;

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY, -- sha256 of the cookie value, never the value itself
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX sessions_user ON sessions (user_id);

-- Single-use tokens: magic links and e-mail verification.
CREATE TABLE login_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER REFERENCES users (id) ON DELETE CASCADE,
    email      TEXT NOT NULL DEFAULT '',
    purpose    TEXT NOT NULL, -- magic | verify
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER
);

CREATE TABLE oauth_accounts (
    provider         TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    user_id          INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at       INTEGER NOT NULL,
    PRIMARY KEY (provider, provider_user_id)
);

CREATE UNIQUE INDEX oauth_accounts_user ON oauth_accounts (user_id, provider);

CREATE TABLE oauth_states (
    state       TEXT PRIMARY KEY,
    provider    TEXT NOT NULL,
    verifier    TEXT NOT NULL DEFAULT '',
    redirect_to TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    expires_at  INTEGER NOT NULL
);

CREATE TABLE boards (
    id          INTEGER PRIMARY KEY,
    slug        TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL
);

CREATE UNIQUE INDEX boards_slug ON boards (slug COLLATE NOCASE);

CREATE TABLE threads (
    id           INTEGER PRIMARY KEY,
    board_id     INTEGER NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    author_id    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    title        TEXT NOT NULL,
    -- plain-text preview of the first post, denormalised for list queries
    excerpt      TEXT NOT NULL DEFAULT '',
    pinned       INTEGER NOT NULL DEFAULT 0,
    locked       INTEGER NOT NULL DEFAULT 0,
    deleted_at   INTEGER,
    deleted_by   INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at   INTEGER NOT NULL,
    last_post_at INTEGER NOT NULL,
    last_post_id INTEGER REFERENCES posts (id) ON DELETE SET NULL,
    post_count   INTEGER NOT NULL DEFAULT 0,
    view_count   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX threads_board ON threads (board_id, pinned DESC, last_post_at DESC);
CREATE INDEX threads_recent ON threads (last_post_at DESC);

CREATE TABLE posts (
    id         INTEGER PRIMARY KEY,
    thread_id  INTEGER NOT NULL REFERENCES threads (id) ON DELETE CASCADE,
    author_id  INTEGER REFERENCES users (id) ON DELETE SET NULL,
    body_md    TEXT NOT NULL,
    body_html  TEXT NOT NULL DEFAULT '',
    score      INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    edited_at  INTEGER,
    deleted_at INTEGER,
    deleted_by INTEGER REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX posts_thread ON posts (thread_id, created_at);
CREATE INDEX posts_author ON posts (author_id, created_at DESC);

CREATE TABLE post_votes (
    post_id    INTEGER NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    value      INTEGER NOT NULL, -- 1 or -1
    created_at INTEGER NOT NULL,
    PRIMARY KEY (post_id, user_id)
);

CREATE TABLE reports (
    id          INTEGER PRIMARY KEY,
    reporter_id INTEGER REFERENCES users (id) ON DELETE SET NULL,
    post_id     INTEGER REFERENCES posts (id) ON DELETE CASCADE,
    thread_id   INTEGER REFERENCES threads (id) ON DELETE CASCADE,
    reason      TEXT NOT NULL,
    details     TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open', -- open | resolved | dismissed
    resolved_by INTEGER REFERENCES users (id) ON DELETE SET NULL,
    resolved_at INTEGER,
    created_at  INTEGER NOT NULL
);

CREATE INDEX reports_status ON reports (status, created_at DESC);

CREATE TABLE notifications (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    actor_id   INTEGER REFERENCES users (id) ON DELETE SET NULL,
    kind       TEXT NOT NULL, -- reply | mention
    thread_id  INTEGER REFERENCES threads (id) ON DELETE CASCADE,
    post_id    INTEGER REFERENCES posts (id) ON DELETE CASCADE,
    read_at    INTEGER,
    created_at INTEGER NOT NULL
);

CREATE INDEX notifications_user ON notifications (user_id, created_at DESC);

CREATE TABLE uploads (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER REFERENCES users (id) ON DELETE SET NULL,
    stored_name  TEXT NOT NULL UNIQUE,
    filename     TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size         INTEGER NOT NULL,
    created_at   INTEGER NOT NULL
);

CREATE TABLE audit_log (
    id          INTEGER PRIMARY KEY,
    actor_id    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    action      TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT '',
    target_id   INTEGER,
    details     TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE INDEX audit_log_recent ON audit_log (created_at DESC);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Full-text search. Kept as a standalone FTS5 table (not external content) so
-- that triggers can maintain it from posts and thread titles.
CREATE VIRTUAL TABLE search_index USING fts5 (
    title,
    body,
    thread_id UNINDEXED,
    post_id UNINDEXED,
    tokenize = 'porter unicode61'
);

CREATE TRIGGER posts_search_insert AFTER INSERT ON posts BEGIN
    INSERT INTO search_index (title, body, thread_id, post_id)
    VALUES (
        (SELECT title FROM threads WHERE id = new.thread_id),
        new.body_md,
        new.thread_id,
        new.id
    );
END;

CREATE TRIGGER posts_search_update AFTER UPDATE OF body_md ON posts BEGIN
    UPDATE search_index SET body = new.body_md WHERE post_id = new.id;
END;

CREATE TRIGGER posts_search_delete AFTER DELETE ON posts BEGIN
    DELETE FROM search_index WHERE post_id = old.id;
END;

CREATE TRIGGER threads_search_retitle AFTER UPDATE OF title ON threads BEGIN
    UPDATE search_index SET title = new.title WHERE thread_id = new.id;
END;
