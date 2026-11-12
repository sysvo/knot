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
```

`KNOT_PUBLIC_URL` must be exactly the address readers type. It is baked into
OAuth callbacks, magic links, canonical links and the sitemap, so a mismatch
means those links point somewhere that does not work.

Then:

```bash
docker compose up -d --build
```

The first start creates the database and applies migrations. That takes a few
seconds; the `api` container reports healthy when it is done. Open the site: it
shows a setup page asking for the forum's name, the administrator account and,
if you have one, a mail relay. Filling that in is the whole first run.

Setup is open only while the instance has no accounts at all. As soon as the
administrator exists it closes, so the page cannot be used to claim a forum
somebody else already set up. To provision an administrator without a browser,
set `KNOT_ADMIN_USERNAME`, `KNOT_ADMIN_EMAIL` and `KNOT_ADMIN_PASSWORD` before
the first start instead; those are read once on an empty database.

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

knot backs itself up. Once a day it takes a snapshot of the database and every
upload into one zip, and keeps the newest `KNOT_BACKUP_KEEP` archives (7 by
default; set `0` to turn the schedule off). Administration → Overview lists them,
takes one on demand, and downloads one.

```bash
docker compose exec api ls -l /data/backups
```

They are written to `KNOT_BACKUP_DIR`, which defaults to a `backups` directory
beside the database — inside the `knot-data` volume, so they survive a rebuild of
the containers. They do not survive losing the volume, which is why you should
copy the newest one off the machine:

```bash
docker compose cp api:/data/backups ./backups
```

The snapshot is taken with `VACUUM INTO`, so it is consistent while people keep
posting: there is no need to stop the API to back up.

To restore, with the stack stopped:

```bash
docker compose down
docker run --rm \
  -v knot_knot-data:/data \
  -v "$PWD/backups:/restore:ro" \
  alpine sh -c 'apk add --no-cache unzip >/dev/null &&
    rm -rf /data/knot.db /data/uploads &&
    unzip -o /restore/knot-20261112-234221.zip -d /data'
docker compose up -d
```

Use a real archive name from the directory listing. The zip holds `knot.db`, an
`uploads/` directory and a `manifest.json` describing what it is; `manifest.json`
ends up in `/data` after the restore and can be deleted.

An untested backup is not a backup. Restore one into a scratch stack once, and
check that logging in and reading an old thread both work.

#### A copy of the whole volume

Sometimes you want everything at once, including the archived backups. Stop the
API first, so the WAL is checkpointed and no write is in flight:

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

**Administration → Settings → E-mail** sets the relay, and it wins over the
environment, so you can fix a typo without a shell. `KNOT_SMTP_*` still works
and is used until the panel has a host of its own, which is handy for a
container configured entirely from `.env`.

The panel has a **Send test** button, and it shows the relay's own error
verbatim. That is the point: "535 authentication failed" is the sentence that
fixes the problem, and a silent failure is how an evening disappears.

Without a relay, nothing is sent — but nothing is lost either. Magic links,
e-mail confirmation and password resets are unavailable (the login page stops
offering them), and anything that would have been sent waits in the **Outbox**
at the bottom of the settings tab. Configure mail later and it drains.

Mail that fails is retried with a growing delay — a minute, five, fifteen, an
hour, six hours, then daily — for eight attempts, after which it is marked as
given up and shown in the Outbox with its last error. You can retry or discard
each message there. Delivered messages are kept for a week.

Two limits worth knowing: STARTTLS on port 587 is supported, implicit TLS on 465
is not, and the password is stored in the database in plain text. The database
file is already the thing you back up and protect; treat it accordingly.

If *Require confirmed e-mail* is on and mail is broken, new members cannot post.
The admin overview says so in red rather than leaving you to work it out.

## Keeping spam out

The settings are under **Administration → Settings → Spam protection**, and they
take effect immediately — no restart. Moderation and administration accounts are
never restricted by them.

| Setting | Default | What it does |
| --- | --- | --- |
| Posts before links are allowed | 3 | A new account cannot post links to other sites until it has this many published posts. |
| Days before links are allowed | 2 | …and has been a member this long. Both must be met. `0` turns the rule off. |
| Daily post limit for new accounts | 20 | Applies only until an account qualifies above. `0` means no limit. |
| Hold the first posts for review | 0 (off) | How many of a new account's first posts wait for a moderator. |
| Blocked e-mail domains | empty | Refused at registration; subdomains included. |

**For a public forum, the first three at their defaults stop almost everything
without touching a legitimate person.** A held-post queue is the stronger option,
but it comes with a promise: someone has to look at it. Turn on *Hold the first
posts* only if you check the queue most days — a new member whose first post sits
invisible for a week concludes the forum is broken and does not come back.

Two things are always on and are not configurable: registration has a hidden
field that people never fill and bots usually do, and a form submitted in under a
second is rejected. Both produce the same vague failure as any other bad sign-up.

If you are being flooded anyway, the honest response is to close registration
(turn off *Allow registration*) until it passes, rather than to hope the limits
hold. Your existing members are unaffected.

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
