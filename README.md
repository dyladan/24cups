# Advent Cup

A community app for logging, rating, and comparing the coffees in this year's
coffee advent calendar. It's independent and **not affiliated with Onyx Coffee
Lab or Fellow**.

- **Calendar**: one door per day. Unopened doors hide the coffee and, by default,
  its scores (no spoilers). You can open a door without logging a brew.
- **Brews**: coffee + recipe (equipment, ratio, temp, target time) + actual time,
  grinder, burrs, grind setting, half-star rating, notes. Brews are public or private.
  Private brews can be excluded from the global ratings.
- **Presets and grinders**: save recipe presets and grinders on your profile. Each
  grinder can have several burrsets. Common models and burrs are suggested, and
  you can type a custom value for anything.
- **Ratio entry**: `1:X`, `g in / mL out`, or `g/L`, all stored as one ratio.
- **Groups**: public (discoverable) or private (invite link or code). Each group
  has its own leaderboard and brew feed. A user can belong to many groups.
- **Rankings**: opened coffees ranked by community average, your own average, or favorites.

Global averages are computed **per user first** (each user's included brews are
averaged), then averaged across users, so one prolific brewer can't skew a coffee.

## Live updates

Pages update in near real time over Server-Sent Events (`GET /api/events`).
Events carry no data, only hints like "coffee data changed", which tell the
browser which cached queries to refetch through the normal API. Privacy rules
therefore apply exactly as they do for any other request. Your own changes
aren't echoed back to the tab that made them; recipe, grinder, settings and
my-brews hints go only to your own devices. A tab refetches everything after
reconnecting.

If you put a proxy in front, it must not buffer `text/event-stream` responses.
Caddy and Cloudflare Tunnel work as is; nginx honors the `X-Accel-Buffering: no`
header the server sends. Over plain HTTP/1.1, browsers allow about 6
connections per site, so serve over HTTPS (HTTP/2) if people keep many tabs open.

## Performance

Community stats (averages, opens, favorites, rating distributions, group
leaderboards) are kept in memory. They're loaded at startup and updated
after each write by re-reading only the affected user/coffee pair, so page
views never scan the brews table. Reads use a pool of read-only SQLite
connections; writes go through a single connection.

Measured on a Raspberry Pi 5 (4 GB) with Dec-24-sized data, with the load
generator on the same Pi limited to 1–2 cores:

| Scenario | Result |
|---|---|
| Mixed API traffic, 500 / 2,000 / 5,000 users | ~2,900 / 2,850 / 2,700 req/s, p95 ≈ 11 ms, no errors |
| 2,000 open pages + 2 brews/s (live-update refetch storm) | ~2,000 refetches/s, p95 2.7 ms, p99 9.5 ms, ≤ 1.2 cores |
| Memory | ~80 MB at 5,000 users; ~310 MB with 2,000 open live streams |

## Stack

Go (net/http, SQLite via pure-Go `modernc.org/sqlite`, bcrypt sessions) serving
a React + TypeScript SPA (Vite, TanStack Router, TanStack Query, Tailwind v4)
embedded in a single static binary.

## Running

```sh
make build && ./advent-cup -db advent.db          # http://localhost:8080
# or
docker build -t advent-cup . && docker run -p 8080:8080 -v advent-data:/data advent-cup
```

| Flag / env | Default | |
|---|---|---|
| `-addr` / `ADDR` | `:8080` | listen address |
| `-db` / `DB_PATH` | `advent.db` | SQLite file |
| `-coffees` / `COFFEES_FILE` | embedded `data/coffees.json` | calendar contents |
| `-secure-cookies` / `SECURE_COOKIES=true` | false | set when served over HTTPS |

## Calendar contents

`data/coffees.json` holds the lineup. It's upserted by `day` on every start, so
you can fix typos without touching the database:

```json
[{ "day": 1, "name": "…", "origin": "…", "region": "…", "producer": "…",
   "process": "…", "variety": "…", "elevation": "…", "roastLevel": "…",
   "tastingNotes": "…", "description": "…" }]
```

The checked-in file has the 2026 lineup, transcribed from the printed card. Days 5 (Framily) and 13 (Krampus) are blends; their origin is inferred from the listed region.

## Theming

Colors live in CSS variables at the top of `web/src/styles.css`. The light theme follows the 2026 box and lineup card (crimson, gold foil, cream, burgundy); dark mode follows the black puzzle box. Brand marks (the blackletter wordmark and the horse artwork) are deliberately not reproduced.
