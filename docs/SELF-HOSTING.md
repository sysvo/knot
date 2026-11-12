# Self-hosting knot

Everything here assumes Docker Compose on a small Linux VPS with a domain name
pointing at it. You need ports 80 and 443 reachable, and nothing else.

## First deploy

```bash
git clone https://github.com/sysvo/knot.git
cd knot
cp .env.example .env
```

Edit `.env`:

```ini
KNOT_PUBLIC_URL=https://forum.example.com
KNOT_DOMAIN=forum.example.com
KNOT_ADMIN_USERNAME=admin
KNOT_ADMIN_EMAIL=you@example.com
KNOT_ADMIN_PASSWORD=a long passphrase you will keep
```

`KNOT_PUBLIC_URL` must be exactly the address readers type. It is baked into
OAuth callbacks and magic links, so a mismatch means those links point somewhere
that does not work.

Then:

```bash
docker compose up -d --build
```

The first start creates the database, applies migrations and creates the
administrator. That takes a few seconds; the `api` container reports healthy when
it is done.

Caddy asks Let's Encrypt for a certificate as soon as DNS resolves. Watch it:

```bash
docker compose logs -f caddy
```

If DNS is not ready yet, Caddy retries — it does not give up permanently.

### Trying it without a domain

Set `KNOT_DOMAIN=localhost` and `KNOT_PUBLIC_URL=http://localhost`. Caddy then
serves from its own internal certificate authority, so the browser will warn
about the certificate. Accept it and everything works.

## Day-to-day

```bash
docker compose ps                 # what is running
docker compose logs -f api        # request and error log
docker compose restart api        # after changing .env
docker compose down               # stop (keeps the data volumes)
```

### Backups

Two things matter: the database and the uploaded images. They share one volume.

SQLite runs in WAL mode, so copying `knot.db` on its own can miss the most recent
writes. Stop the API first and copy the whole volume:

```bash
docker compose stop api

docker run --rm \
  -v knot_knot-data:/data \
  -v "$PWD:/backup" \
  alpine tar czf "/backup/knot-$(date +%F).tgz" -C /data .

docker compose start api
```

The volume name is `<compose project>_<volume>`, which is `knot_knot-data` unless
you renamed the project. `docker volume ls` will tell you.

To restore, with the stack stopped:

```bash
docker compose down
docker run --rm -v knot_knot-data:/data -v "$PWD:/backup" alpine \
  sh -c 'rm -rf /data/* && tar xzf /backup/knot-2026-11-12.tgz -C /data'
docker compose up -d
```

An untested backup is not a backup. Restore one into a scratch stack once, and
check that logging in and reading an old thread both work.

### Upgrading

```bash
git pull
docker compose up -d --build
```

Migrations run on start. They only ever add; nothing is dropped. Downgrading is
not supported, so keep the backup from before you upgrade.

### Roles and recovery

```bash
docker compose exec api knot passwd alice
docker compose exec api knot role alice moderator
docker compose exec api knot role alice admin
```

`knot passwd` also signs that account out everywhere.

## E-mail

Without SMTP, mail is written to the container log instead of being sent. That
is deliberate: development needs no mail server, and a self-hoster without mail
still gets a working forum — except magic links, which the login page will not
offer.

To enable it:

```ini
KNOT_SMTP_HOST=smtp.example.com
KNOT_SMTP_PORT=587
KNOT_SMTP_USER=apikey
KNOT_SMTP_PASS=secret
KNOT_SMTP_FROM=forum@example.com
```

STARTTLS is used when the server offers it. After changing these, restart the
API; the login page then offers "sign in with an e-mail link", and turning on
*Require confirmed e-mail* in the admin settings becomes meaningful.

If that setting is on and mail is broken, new accounts cannot post. Check
`docker compose logs api` for `send verify mail` errors before assuming the
setting is wrong.

## Social sign-in

Each provider needs a client id and secret, and a callback URL registered with
the provider:

| Provider | Callback URL |
| --- | --- |
| GitHub | `https://your-host/api/v1/auth/oauth/github/callback` |
| Discord | `https://your-host/api/v1/auth/oauth/discord/callback` |

Then set `KNOT_GITHUB_CLIENT_ID` / `KNOT_GITHUB_CLIENT_SECRET` (or the Discord
pair) and restart. A provider only appears on the login page when both values are
present, so a half-configured provider is invisible rather than broken.

If an account with the same **verified** e-mail already exists, signing in with a
provider links to it instead of creating a second account.

## Administering

Everything is in the app, under **Administration** in the account menu:

- **Boards** — create, rename, reorder, delete (deleting takes the threads with it)
- **Users** — promote to moderator or admin, ban, unban
- **Reports** — the moderation queue, available to moderators too
- **Settings** — site name and description, whether registration is open,
  whether e-mail must be confirmed, and a note for the registration page
- **Audit log** — who pinned, locked, deleted or changed a role, and when

Moderators get **Reports** and **Audit log** only. They can also pin, lock and
delete any thread or post from the thread page.

The last remaining administrator cannot be demoted. If you somehow end up with an
account that cannot sign in, `knot role` and `knot passwd` above are the way back.

## Performance and limits

A single SQLite connection serves every request. That is not a bottleneck at
forum scale — statements run in tens of microseconds — but it does mean one slow
query blocks others. The queries that could get slow are the thread listings, and
they are covered by indexes on `(board_id, pinned, last_post_at)` and
`(last_post_at)`.

If you ever outgrow one box, the thing to replace is `internal/store`; nothing
else knows that the database is SQLite.

Uploads are capped by `KNOT_MAX_UPLOAD_MB` and restricted to PNG, JPEG, GIF and
WebP, identified by their content rather than their filename.

## Troubleshooting

**Pages load but nothing works.** Check `docker compose logs api`. If the API is
unhealthy, `docker compose ps` will say so; the usual cause is a permissions
problem on the data volume.

**Login redirects back to the login page.** `KNOT_PUBLIC_URL` probably does not
match the address you are using, so the cookie is being set for a different host.
It must be the exact scheme and host, with no trailing slash.

**Caddy never gets a certificate.** DNS is not pointing here yet, or port 80 is
blocked. Both are required: HTTP is used for the ACME challenge, even for an
HTTPS-only site.

**Search finds nothing.** The FTS index is maintained by triggers; a post written
before a schema change would not be indexed. For a fresh install this cannot
happen. If you restored an old database by copying tables rather than the file,
rebuild the index:

```sql
INSERT INTO search_index(search_index) VALUES('rebuild');
```

**Everything is 500 and the log says `database is locked`.** Another process is
holding the database — usually a second container or a local `go run` pointed at
the same file. Run one writer.
