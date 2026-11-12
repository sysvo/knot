# Contributing to knot

Thanks for taking a look. This file covers the things that are easy to get wrong;
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) explains how the pieces fit together.

## Getting set up

You need Go 1.27 or newer, Node 22 or newer, and nothing else.

```bash
# terminal 1
cd backend && go run ./cmd/knot serve

# terminal 2
cd frontend && npm install && npm run dev
```

The database lands in `backend/data/` and is created on first start. Delete that
folder to start over. The first visit to `localhost:3000` offers a setup page that
creates the administrator, which is how you get into `/admin`. If you deleted the
database you get that page again; on an instance that already has accounts it
redirects away.

## Before you open a pull request

```bash
cd backend  && gofmt -l . && go vet ./... && go test ./...
cd frontend && npm run typecheck && npm run build
```

For anything that touches the API by hand, run the smoke test against a server
with an empty database:

```bash
cd backend
$env:KNOT_DB = "$env:TEMP/knot-smoke.db"      # PowerShell
Remove-Item $env:KNOT_DB -ErrorAction SilentlyContinue
go run ./cmd/knot serve

# separately
powershell -File scripts/smoke.ps1 -Base http://localhost:8080
```

## Conventions worth knowing

**SQL lives in `internal/store`.** Handlers do not write queries, and the store
does not know about HTTP. If you find yourself importing `net/http` in the store,
something has gone wrong.

**Server-only and browser-only frontend code are separate files.**
`lib/server.ts` imports `next/headers`; `lib/api.ts` must never do so. A client
component that imports the server module fails the build with a message about the
Pages Router, which is confusing but accurate.

**Use `"use client"` only where something responds to input.** Pages are server
components that fetch with the reader's cookie. Voting, composing and admin forms
are client components.

**Migrations are append-only.** Add `0004_something.sql`; never edit an applied
one. Each file runs in its own transaction and is recorded in
`schema_migrations`.

**Spam rules live in `internal/policy`.** They are pure functions taking an
account and the thresholds, with no HTTP and no SQL, because they are the part
most likely to be argued about. Add a rule there first and wire it in second.

**Write the test that would have caught the bug.** Where the property matters —
sanitisation, single-use tokens, authorisation — assert the property, not the
current output. `internal/markdown/markdown_test.go` parses the rendered HTML and
walks it, because substring matching reports escaped text as a vulnerability.

**Comments explain why.** The code says what. A comment that restates the line
below it will be removed in review; a comment that explains an
otherwise-surprising choice (a single database connection, a timing
equalisation, a `ref` guard against React's double effect) is the point of
having comments at all.

## Reporting a security issue

Please do not open a public issue for a vulnerability. Send a description and a
reproduction to the address on the maintainer's GitHub profile, and give a few
days before disclosing it.
