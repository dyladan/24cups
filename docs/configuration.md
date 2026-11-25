# Configuration

Every setting can be passed as a command-line flag or an environment variable.

| Flag / env | Default | |
|---|---|---|
| `-addr` / `ADDR` | `:8080` | listen address |
| `-db` / `DB_PATH` | `advent.db` | SQLite file |
| `-coffees` / `COFFEES_FILE` | embedded `data/coffees.json` | calendar contents |
| `-calendar-year` / `CALENDAR_YEAR` | 2026 | the December the calendar covers |
| `-secure-cookies` / `SECURE_COOKIES=true` | false | set when served over HTTPS |
| `-client-ip-header` / `CLIENT_IP_HEADER` | none | trusted real-IP header, e.g. `CF-Connecting-IP` |
| `TURNSTILE_SECRET_KEY` (env only) | none | enables the Turnstile check on sign-up |
| `-turnstile-site-key` / `TURNSTILE_SITE_KEY` | none | public widget key |
| `-turnstile-hostnames` / `TURNSTILE_HOSTNAMES` | none | comma-separated allowed hostnames (required with a secret) |
| `-no-rate-limits` / `NO_RATE_LIMITS=true` | false | benchmarks and local development only |
| `-dev-day` / `DEV_DAY` | 0 (off) | development only: pretend it's this day of December |

Telemetry uses the standard `OTEL_*` variables; see
[observability.md](observability.md).

In Docker Compose, secrets and per-site values go in `.env` (copy
`.env.example`); the rest is set in `compose.yaml`.

## Calendar contents

`data/coffees.json` holds the lineup and is built into the binary. It's
upserted by `day` on every start, so you can fix a typo and restart without
touching the database. Point `-coffees` at another file to use a different
lineup.

```json
[{ "day": 1, "name": "…", "origin": "…", "region": "…", "producer": "…",
   "process": "…", "variety": "…", "elevation": "…", "roastLevel": "…",
   "tastingNotes": "…", "description": "…" }]
```

The checked-in file is the 2026 lineup, transcribed from the printed card.
Opening it spoils the whole calendar. For blends, the origin is inferred from
the listed region.

## Theming

Colors live in CSS variables at the top of `web/src/styles.css`. The light
theme follows the 2026 box and lineup card (crimson, gold foil, cream,
burgundy); dark mode follows the black puzzle box. Brand marks (the
blackletter wordmark and the horse artwork) are deliberately not reproduced.
