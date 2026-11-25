# Deployment

The supported setup is Docker Compose behind a Cloudflare Tunnel, with the
SQLite database backed up continuously to Cloudflare R2. Everything here runs
fine on a Raspberry Pi.

`compose.yaml` runs four containers:

| Service | What it does |
|---|---|
| `app` | The 24 Cups server. Publishes no ports. |
| `cloudflared` | Carries traffic from Cloudflare to the app. |
| `litestream` | Streams every database change to R2. |
| `litestream-restore` | Runs once at startup and restores the database from R2 if the volume is empty. |

Only `cloudflared` can reach the app, over a private Docker network, so the app
can trust Cloudflare's `CF-Connecting-IP` header for rate limiting. The app's
separate `outbound` network is for the calls it makes itself: Cloudflare
Siteverify and, if you use one, an OpenTelemetry collector.

## Setup

1. **Create a tunnel.** In the Cloudflare dashboard, go to Zero Trust →
   Networks → Tunnels → Create a tunnel (Cloudflared). Copy the token from the
   install command.
2. **Route a hostname.** In the tunnel's *Public hostnames*, add your hostname
   (e.g. `advent.example.com`) with service type **HTTP** and URL **`app:8080`**.
3. **Create an R2 bucket** for backups (see [Backups](#backups)).
4. **Configure and start:**
   ```sh
   cp .env.example .env   # fill it in; .env is git-ignored
   docker compose up -d --build
   docker compose ps      # app should be "healthy"
   ```
5. **Recommended Cloudflare settings** (SSL/TLS → Edge Certificates): Always
   Use HTTPS on, minimum TLS 1.2, and HSTS once you've confirmed HTTPS works.

See [abuse-protection.md](abuse-protection.md) for the Cloudflare settings
worth turning on in front of a public instance.

## HTTPS

Visitors connect to Cloudflare's edge, which serves a free, auto-renewing
certificate for your hostname. The hop from Cloudflare to cloudflared is an
encrypted tunnel that cloudflared opens outbound. The only plain-HTTP hop is
cloudflared → app, inside Docker's private network on the same machine. No
origin certificate is needed.

## Turnstile (bot check on sign-up)

Create a Turnstile widget in the Cloudflare dashboard for your hostname, then
set these in `.env`:

- `TURNSTILE_SITE_KEY`: your widget's public site key. `compose.yaml` falls
  back to the original instance's key, which won't work for your hostname.
- `TURNSTILE_SECRET_KEY`: the secret key. Keep it only in `.env`.
- `TURNSTILE_HOSTNAMES`: your hostname(s), comma-separated. They must match the
  widget's domains. The app refuses to start if a secret is set without them.

Each sign-up token is verified server-side with Cloudflare Siteverify: it must
pass, be for action `signup`, come from an allowed hostname, and not be reused.
Any failure or timeout rejects the sign-up. Without a secret, sign-up has no
bot check, which is fine for local development.

The app needs outbound access to `challenges.cloudflare.com` (the `outbound`
network in compose). At startup it checks this and that the secret is
accepted, logging `turnstile check passed` or `turnstile check failed: …`.
Each rejected sign-up logs `turnstile rejected sign-up` with a reason
(`unreachable`, `not_success` plus Cloudflare's error codes,
`hostname_mismatch`, `action_mismatch`, …).

## Backups

Data lives in the `advent-data` volume (`/data/advent.db`). The `litestream`
container streams every committed change to R2 with
[Litestream](https://litestream.io). It takes a full snapshot daily and keeps
7 days of history (`litestream.yml`).

**Setup:** in the Cloudflare dashboard → R2, create a bucket (e.g.
`24cups-backups`), then an API token with **Object Read & Write** limited
to that bucket. Put the account ID, bucket, access key ID and secret in `.env`.

**Automatic restore:** before the app starts, `litestream-restore` checks the
volume. If there's no database and R2 has a backup, it restores the latest
one. To move hosts or recover from a lost disk, run `docker compose up` with
the same `.env` on an empty volume.

**Check replication:**
```sh
docker compose logs litestream                                  # activity / errors
docker compose run --rm litestream-restore ltx /data/advent.db  # files in R2
```

**Restore to a point in time** (e.g. to undo a bad change), into a separate
file first:
```sh
docker compose run --rm litestream-restore restore \
  -timestamp 2026-12-10T08:00:00Z -o /data/restored.db /data/advent.db
# inspect it, then: docker compose stop app litestream,
# move restored.db over advent.db (remove advent.db-wal/-shm), start again
```

Test a restore into a scratch path now and then, so you know the backups work.

## Other proxies

Any reverse proxy works if you'd rather not use Cloudflare Tunnel. Two things
to get right:

- Set `CLIENT_IP_HEADER` to the header your proxy uses for the real client IP,
  and make sure the app's port can't be reached except through the proxy.
  See [abuse-protection.md](abuse-protection.md).
- The proxy must not buffer `text/event-stream` responses, or live updates
  stall. Caddy and Cloudflare Tunnel work as is; nginx honors the
  `X-Accel-Buffering: no` header the server sends. Serve over HTTPS (HTTP/2):
  over plain HTTP/1.1, browsers allow only about 6 connections per site, and
  each open tab holds one.

All settings are listed in [configuration.md](configuration.md).
