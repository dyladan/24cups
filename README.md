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

The checked-in file has **placeholders**. Replace it with the real list.

## Theming

Colors live in CSS variables at the top of `web/src/styles.css` (light and dark).
