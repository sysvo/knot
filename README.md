# knot

A small forum you can host yourself.

Two processes, one database file, no external services: a Go API over SQLite and
a Next.js frontend. Put it on a VPS with Docker Compose and it is running in
about a minute.

- **Boards, threads and replies**, with pinning, locking and soft deletion
- **Markdown** written by readers, rendered and sanitised on the server once, at
  write time
- **Accounts** with argon2id passwords, opaque database-backed sessions, magic
  links, password reset by e-mail, and optional GitHub or Discord sign-in
- **E-mail** configured in the admin panel with a test button, delivered through
  a queue that retries and shows you what failed
- **Full-text search** over every post, using SQLite's FTS5
- **Votes, reports and a moderation queue**, with an audit log of who did what
- **Spam defence** that does not get in the way: new accounts cannot post links,
  have a daily limit, and can be held for review until a moderator publishes them
- **Notifications** for replies and `@mentions`
- **Profiles, avatars and image uploads**
- **Light and dark themes** that follow the reader's system preference
- **Administration**: boards, users, roles, bans and site settings, in the app
- **Backups**: one file holding the database and every upload, taken daily with a
  retention limit, or with one button, and downloadable from the admin panel
- **Shareable and crawlable**: OpenGraph tags on threads and boards, an XML
  sitemap and a `robots.txt`, so a shared link renders a card instead of a bare URL

knot is licensed under the [AGPL-3.0](LICENSE). If you host a modified version,
you have to publish the changes — that is the point of the licence, and it is why
improvements to a hosted forum tend to come back upstream.

## Run it

### Docker Compose (what you want on a server)

```bash
cp .env.example .env      # then edit KNOT_PUBLIC_URL and KNOT_DOMAIN
docker compose up -d --build
```

Caddy obtains a TLS certificate for `KNOT_DOMAIN` automatically and routes
`/api/*` and `/uploads/*` to the Go process and everything else to Next.js. Then
open the site and finish setup in the browser: the first page asks for the forum's
name, the administrator account and, optionally, a mail relay. See
[docs/SELF-HOSTING.md](docs/SELF-HOSTING.md) for backups, upgrades, mail and OAuth
setup.

### Local development

Two terminals:

```bash
cd backend  && go run ./cmd/knot serve     # http://localhost:8080
cd frontend && npm install && npm run dev  # http://localhost:3000
```

The frontend proxies `/api` and `/uploads` to the API in development, so the
browser only ever talks to `localhost:3000` and the session cookie is
first-party. No environment variables are required: the database is created at
`backend/data/knot.db` with a few starter boards.

The first visit to `http://localhost:3000` offers a setup page that creates the
administrator. Setup is open only while the instance has no accounts at all, so it
closes as soon as it is done and cannot be re-used by a later visitor.

## Configuration

Every value has a working default. Nothing is required to run locally.

| Variable | Default | Purpose |
| --- | --- | --- |
| `KNOT_PUBLIC_URL` | `http://localhost:3000` | Public address. OAuth callbacks and magic links are built from it. |
| `KNOT_ADDR` | `:8080` | Listen address of the API. |
| `KNOT_DB` | `data/knot.db` | SQLite database file. |
| `KNOT_UPLOAD_DIR` | `data/uploads` | Where uploaded images are stored. |
| `KNOT_BACKUP_DIR` | beside the database | Where backup archives are written. |
| `KNOT_BACKUP_KEEP` | `7` | How many daily backups to keep; `0` turns them off. |
| `KNOT_SECURE_COOKIES` | derived from `KNOT_PUBLIC_URL` | Set `true` when served over HTTPS. |
| `KNOT_TRUST_PROXY` | `true` | Read `X-Forwarded-For` for rate limiting and logs. |
| `KNOT_SESSION_TTL` | `720h` | How long a sign-in lasts. |
| `KNOT_MAX_UPLOAD_MB` | `8` | Largest accepted image. |
| `KNOT_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `KNOT_LOG_JSON` | `false` | Structured logs, for shipping to somewhere. |
| `KNOT_SMTP_HOST` / `_PORT` / `_USER` / `_PASS` / `_FROM` | unset | Seeds the relay; Administration → Settings overrides it. |
| `KNOT_GITHUB_CLIENT_ID` / `_SECRET` | unset | Enables GitHub sign-in. |
| `KNOT_DISCORD_CLIENT_ID` / `_SECRET` | unset | Enables Discord sign-in. |
| `KNOT_ADMIN_USERNAME` / `_EMAIL` / `_PASSWORD` | unset | Creates the first administrator on an empty database, for unattended installs. |

`KNOT_ADMIN_PASSWORD` is only used on a brand-new install; it never changes an
existing account. If you leave these unset, the setup page asks for an
administrator instead. To recover a locked-out account:

```bash
docker compose exec api knot passwd alice     # prompts for a new password
docker compose exec api knot role alice admin
```

## Tests

```bash
cd backend && go test ./...
cd frontend && npm run typecheck && npm run build
```

`scripts/smoke.ps1` drives 79 behaviours against a running API — setup, sign-up,
sessions, posting, voting, notifications, moderation, search, backups, the
sitemap, and CSRF rejection. It needs a **fresh database**, because setup only
succeeds while no account exists.

## How it is put together

```
frontend/          Next.js App Router, server components read from the API
backend/           Go: HTTP API, store, auth, markdown, mail
  cmd/knot/        the binary: serve, migrate, passwd, role
  internal/api/    handlers and middleware
  internal/store/  every SQL statement in the project
  internal/db/     SQLite connection and embedded migrations
deploy/Caddyfile   reverse proxy and TLS
docs/              architecture and self-hosting notes
```

[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) explains why it is built this way,
including what was deliberately left out.

## Contributing

Issues and pull requests are welcome. Read
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) first — it covers the conventions
that matter, and the tests that have to stay green.
