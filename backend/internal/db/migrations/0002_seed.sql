-- Seed data: a starter set of boards and default settings.
-- INSERT OR IGNORE keeps this safe to re-run and safe for admins to edit.

INSERT OR IGNORE INTO boards (slug, name, description, position, created_at) VALUES
    ('general',   'General',   'Anything and everything.',                 10, unixepoch()),
    ('help',      'Help',      'Ask questions and get unstuck.',           20, unixepoch()),
    ('showcase',  'Showcase',  'Share what you have built with knot.',     30, unixepoch()),
    ('meta',      'Meta',      'Discussion about running and improving this forum.', 40, unixepoch());

INSERT OR IGNORE INTO settings (key, value) VALUES
    ('site_name',         'knot'),
    ('site_description',  'A small, self-hosted forum.'),
    ('allow_registration', 'true'),
    ('require_email_verification', 'false'),
    ('registration_note', '');
