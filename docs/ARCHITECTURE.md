# Architecture

The short version: a Go process owns the data, a Next.js process renders pages,
and a reverse proxy puts them on one origin.

```
browser ──► Caddy ──┬── /api/*, /uploads/* ──► Go API ──► SQLite + files
                    └── everything else     ──► Next.js ──► (reads from the API)
```

Both halves are developed and deployed separately, so each can be replaced
without rewriting the other. That is the main reason for the split: the API is a
complete, documented JSON interface rather than something the frontend reaches
into.

## Why Go for the API

- **One binary, no runtime.** `CGO_ENABLED=0` and a pure-Go SQLite driver mean
  the container is a static binary on an Alpine base. There is no interpreter
  version to match and no dependency tree to audit at deploy time.
- **SQLite is the whole database.** For a forum, one file plus WAL is faster and
  simpler than a separate database server, and a backup is a file copy.
- **The Go standard library covers HTTP routing** since 1.22 (`GET /api/v1/threads/{id}`),
  so the router is not a dependency.

## Why Next.js for the frontend

Server components render the pages on the server with the reader's cookie
forwarded to the API, so the HTML arrives complete — good for search engines and
for the first paint. Client components are used only where something actually
responds to input: voting, composing, moderation controls, admin forms.

The browser never talks to the API directly. It requests `/api/...` from the
page's own origin, which the Next rewrite (in development) or Caddy (in
production) forwards to the Go process. Consequences worth knowing:

- the session cookie is httpOnly, `SameSite=Lax`, and first-party
- there is no CORS configuration anywhere, and nothing to get wrong
- uploads can be served with a long cache lifetime from the same origin

## Data model

`users`, `boards`, `threads`, `posts` are the core. Around them:

- `sessions` — the hash of a cookie token, never the token itself
- `login_tokens` — single-use magic links and e-mail verification
- `oauth_accounts` — provider identity → local account
- `post_votes` — one row per voter per post; the score is recomputed from it
- `reports`, `notifications`, `audit_log`, `uploads`, `settings`

Threads and posts are **soft deleted** (`deleted_at`) so replies do not lose
their context and moderators can still see what happened. Deleting a board is
hard: it cascades. A NULL `approved_at` means the row is held for review; see
[Spam defence](#spam-defence) below.

`threads` keeps denormalised `post_count`, `last_post_at`, `last_post_id` and
`excerpt` so that listing a board is a single indexed query instead of an
aggregate over every post.

### SQLite specifics

- **One connection.** The pool is capped at one connection. SQLite serialises
  writers anyway; a single connection also removes any chance of `SQLITE_BUSY`
  or of a deferred transaction failing to upgrade to a write lock. Statements
  take tens of microseconds, so this comfortably serves a forum. To scale past a
  single box, the store is the only thing that has to change.
- **WAL, `synchronous=NORMAL`, `foreign_keys=ON`** are set in the DSN.
- **Migrations are embedded** in the binary and applied on start, each in its own
  transaction, tracked in `schema_migrations`.

## Markdown, and what is trusted

Post bodies are Markdown. They are converted to HTML with goldmark (GitHub
flavoured: tables, strikethrough, task lists) and sanitised with bluemonday
before being stored. The stored `body_html` is what gets rendered, so:

- readers never run a Markdown parser
- there is exactly one sanitiser, and it runs once
- what is in the database is already safe to serve

`body_md` is kept as well, so a post can be edited in its original form.
A plain-text rendering is stored in `search_text`; the search index reads that,
so snippets read as prose rather than showing `###` and asterisks.

## Search

FTS5 with the `porter unicode61` tokenizer, maintained by triggers on `posts` and
`threads`. Free-text input is turned into a quoted, prefix-matched `AND` query in
which every non-alphanumeric character is a separator — so `self-hosted` searches
for both words and no input can be read as an FTS operator.

## Authentication

- **Passwords**: argon2id (64 MiB, t=3, p=2), PHC string format.
- **Sessions**: 32 random bytes in an httpOnly cookie; the database stores only
  its SHA-256. Signing out deletes the row, so a leaked database does not hand
  out live sessions. Banning a user deletes their sessions.
- **Magic links**: 30 minutes, single use, consumed inside a transaction.
- **OAuth**: authorisation-code flow with PKCE and a stored `state`. Identities
  are matched to an existing account when the provider reports a verified e-mail
  address; otherwise a new account is created.
- **Timing**: a missing account still performs a password hash comparison, so
  "no such user" and "wrong password" take the same time.

## Spam defence

An open registration form on a public forum is an invitation. The rules are
deliberately few, and each one is narrow enough to explain to a real person:

- **Links are earned.** A new account cannot post links to other sites until it
  has N published posts *and* has been a member for M days. The check runs on the
  rendered HTML, not the Markdown, so bare URLs the renderer turns into links and
  external images are caught by the same rule that catches `[text](url)`. It is
  re-checked on edit, because otherwise the trick is to post clean text and edit
  the link in.
- **A daily limit** applies to accounts that have not qualified yet. It counts
  held posts too, so filling the queue is not a way around it.
- **First posts can be held.** `approved_at IS NULL` means "waiting for a
  moderator". Held content is invisible everywhere: out of board listings, out of
  search, out of the counters, and it never triggers a notification, because a
  notification pointing at something the recipient cannot open is worse than no
  notification. Approving publishes it and *then* announces it.
- **Registration has a honeypot and a timing floor.** Both reject with the same
  vague failure as any other bad sign-up.
- **Optional e-mail domain blocklist**, matching subdomains.

Two design choices are worth calling out.

**Trust is derived, never stored.** Whether an account may post links is computed
from its published post count and the age of the account on each request. There
is no `trust_level` column to get out of step with reality, no promotion job, and
no way for a state machine to leave someone permanently stuck in a level they
have outgrown. The cost is one indexed count per write, which is nothing.

**Counters are recounted, not adjusted.** Publishing, approving and deleting all
change `post_count` and `last_post_at`, and incremental arithmetic in three
places eventually disagrees with the data. `recountThread` derives them from the
posts in one statement, so there is exactly one answer.

What this does not do: no CAPTCHA, no IP reputation, no machine learning, no
"shadow banning". Those either require a third-party service, which breaks the
single-binary promise, or are dishonest to the person on the other side.

## Security posture

- CSRF: `SameSite=Lax` cookies plus an `Origin` check on every unsafe method.
- Every state-changing endpoint is rate limited per identity, in memory.
- Uploads are content-sniffed; the extension comes from the bytes, not from the
  filename, and the name on disk is random. Served with `nosniff` and a sandbox
  CSP.
- Post HTML is sanitised server-side; external links get `rel="nofollow
  noopener noreferrer"`.
- The last administrator cannot be demoted — the easiest way to lock yourself
  out of a self-hosted instance.
- Passwords and tokens are never logged.

## Deliberate omissions

Things that are missing on purpose, with the reasoning, so they are decisions
rather than gaps:

- **No real-time updates.** Notifications are read on page load, not pushed. A
  forum tolerates a stale badge; adding WebSockets means sticky sessions and a
  second way for the page to change under the reader.
- **No email digests or subscriptions.** Notifications cover replies to your
  thread and mentions. A digest is a scheduled job and a second mail template
  set, and most small forums do not want it.
- **No plugin system.** Extensibility is the API plus a fork, which the AGPL
  makes useful to everyone.
- **No private messages.** They double the moderation surface and the data model.
- **No multi-tenancy.** One instance per community keeps the schema, the
  settings and the backup story simple.

## Repository layout

| Path | What lives there |
| --- | --- |
| `backend/cmd/knot` | The binary and its subcommands |
| `backend/internal/api` | Routing, middleware, handlers, JSON shapes |
| `backend/internal/store` | Every SQL statement |
| `backend/internal/db` | Connection, embedded migrations |
| `backend/internal/auth` | Password hashing, tokens, validation |
| `backend/internal/policy` | Who may post links, who needs review, blocked domains |
| `backend/internal/markdown` | Render, sanitise, plain text |
| `backend/internal/mail` | SMTP, or logging when unconfigured |
| `frontend/app` | Routes (App Router) |
| `frontend/components` | Shared UI; `"use client"` only where needed |
| `frontend/lib/server.ts` | Server-side API access, imports `next/headers` |
| `frontend/lib/api.ts` | Browser-side API access, must stay server-import free |
| `deploy/Caddyfile` | Reverse proxy, TLS, routing |
