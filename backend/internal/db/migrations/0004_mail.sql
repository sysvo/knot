-- Outgoing mail is queued rather than sent inside a request.
--
-- A slow or unreachable SMTP server used to make registration and magic-link
-- requests hang until the dial timed out, and a transient failure was logged and
-- lost. Every message now goes into this table and a worker delivers it, with
-- backoff and a visible record of what is stuck.

CREATE TABLE mail_outbox (
    id              INTEGER PRIMARY KEY,
    to_addr         TEXT NOT NULL,
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at INTEGER NOT NULL,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    sent_at         INTEGER
);

-- The worker only ever asks for unsent rows whose time has come.
CREATE INDEX mail_outbox_due ON mail_outbox (next_attempt_at) WHERE sent_at IS NULL;

INSERT OR IGNORE INTO settings (key, value) VALUES
    ('smtp_host', ''),
    ('smtp_port', '587'),
    ('smtp_user', ''),
    ('smtp_pass', ''),
    ('smtp_from', '');
