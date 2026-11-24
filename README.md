# Advent Cup

A community app for logging, rating, and comparing the coffees in this year's
coffee advent calendar. It's independent and **not affiliated with Onyx Coffee
Lab or Fellow**.

- **Calendar**: one door per day. Unopened doors hide the coffee. You can open
  a door without logging a brew.
- **Signed-out visitors** see everything for days before today: coffee,
  tasting notes, ratings and public brews. Today's door stays sealed, so a
  member browsing signed out isn't spoiled before opening it, unless they flip the
  **Spoilers** switch in the header (saved on the device). Later doors are
  always sealed. "Today" is the visitor's local date, capped at the date in
  UTC+14 so a faked clock can't unlock doors early. The December is set with
  `-calendar-year` / `CALENDAR_YEAR`.
- **Blind tasting** (default): after opening a coffee, its tasting notes,
  community scores and other people's brews stay hidden until you log a rated
  brew or tap "Reveal now". This is enforced by the API, not just the UI. Profile → Spoilers can
  switch this to "once opened" or "always" (scores even on sealed doors).
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

## Abuse protection

Built in (defaults in `DefaultLimits`, `internal/app/ratelimit.go`):

| What | Limit |
|---|---|
| All API requests | 30/s per IP (burst 150), 15/s per signed-in user (burst 75) |
| Changes (brews, groups, …) | 1/s per user (burst 20) |
| Sign-up | 5 per hour per IP |
| Sign-in | 10/min per IP; 10 failures per 15 min per username, from any IP |
| Failed sign-in | ~1 s delay; unknown usernames take the same time |
| Password hashing | at most 3 at once, then "busy" (bcrypt is the costliest thing an anonymous client can trigger) |
| Live-update streams | 10 per user, 40 per IP |
| Content per user | 30 brews per coffee, 50 presets, 20 grinders, 10 owned groups |
| Requests | 64 KB bodies, 32 KB headers |

Limited requests get HTTP 429 with `Retry-After` and a readable message.

**Behind a proxy or tunnel, set `CLIENT_IP_HEADER=CF-Connecting-IP`.**
Otherwise every visitor appears to come from the proxy and shares one
bucket. Only set it if the app's port isn't reachable except through
Cloudflare: anyone who can reach the port directly could fake the header. In
Docker, don't publish the port; attach the container to the proxy network
only. IPv6 clients are grouped by /64.

Suggested Cloudflare settings (dashboard, free plan):
- Keep DDoS protection on (automatic) and use "Under Attack" mode only during an active attack.
- Add a WAF rate-limiting rule for paths starting with `/api/auth/` as an outer layer.
- Bot Fight Mode can challenge real browsers' API and live-update requests. Test before relying on it.
- Optional: Turnstile on sign-up to stop scripted account creation (not implemented yet).

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

Benchmarks run with `-no-rate-limits` (one load generator = one IP).
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

## Deploying (Cloudflare Tunnel)

`compose.yaml` runs the app plus `cloudflared`. The app publishes no ports
and has no internet access. Only cloudflared reaches it, over a private
Docker network, so it can trust Cloudflare's `CF-Connecting-IP` header for
rate limiting.

1. **Create a tunnel.** In the Cloudflare dashboard, go to Zero Trust → Networks →
   Tunnels → Create a tunnel (Cloudflared). Copy the token from the install
   command.
2. **Route a hostname.** In the tunnel's *Public hostnames*, add e.g.
   `advent.example.com` with service type **HTTP** and URL **`app:8080`**.
3. **Configure and start:**
   ```sh
   cp .env.example .env   # paste the token; .env is git-ignored
   docker compose up -d --build
   docker compose ps      # app should be "healthy"
   ```
4. **Recommended Cloudflare settings** (SSL/TLS → Edge Certificates): Always
   Use HTTPS on, minimum TLS 1.2, and HSTS once you've confirmed HTTPS works.

**HTTPS:** visitors connect to Cloudflare's edge, which serves a free,
auto-renewing certificate for your hostname. The hop from Cloudflare to
cloudflared is an encrypted tunnel that cloudflared opens outbound. The only
plain-HTTP hop is cloudflared → app, inside Docker's private network on the
same machine. No origin certificate is needed.

**Data** lives in the `advent-data` volume (`/data/advent.db`). Back it up
with SQLite's online backup (e.g. `VACUUM INTO`), not by copying the file
while the app is running.

## Running

```sh
make build && ./advent-cup -db advent.db          # http://localhost:8080
# or
docker build -t advent-cup . && docker run -p 8080:8080 -v advent-data:/data advent-cup   # local only; see Deploying
```

| Flag / env | Default | |
|---|---|---|
| `-addr` / `ADDR` | `:8080` | listen address |
| `-db` / `DB_PATH` | `advent.db` | SQLite file |
| `-coffees` / `COFFEES_FILE` | embedded `data/coffees.json` | calendar contents |
| `-secure-cookies` / `SECURE_COOKIES=true` | false | set when served over HTTPS |
| `-client-ip-header` / `CLIENT_IP_HEADER` | none | trusted real-IP header, e.g. `CF-Connecting-IP` |
| `-calendar-year` / `CALENDAR_YEAR` | 2026 | the December the calendar covers |
| `-no-rate-limits` / `NO_RATE_LIMITS=true` | false | benchmarks only |

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
