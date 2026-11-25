# Development

## Stack

A Go server (`net/http`, SQLite via the pure-Go `modernc.org/sqlite`, bcrypt,
passkeys via `go-webauthn`, and cookie sessions) serving a React + TypeScript single-page app (Vite,
TanStack Router, TanStack Query, Tailwind v4). The built frontend is embedded
in a single static binary.

```
cmd/24cups/     entry point and flags
internal/app/        HTTP API, database, rules, rate limits
internal/telemetry/  OpenTelemetry setup
web/                 React frontend
data/coffees.json    the calendar lineup (spoilers!)
```

## Running locally

You need Go and Node.js. Run the server and the frontend dev server in two
terminals:

```sh
make dev                 # Go server on :8080, rate limits off
cd web && npm run dev    # Vite on http://localhost:5173, proxies /api to :8080
```

`DAY=3 make dev` pretends it's December 3, for both the server and the
frontend, so you can try the calendar outside December.

`make dev` turns passkeys on for `localhost`, which browsers treat as secure,
so you can try them without HTTPS. Use `http://localhost:5173` rather than
`127.0.0.1`: passkeys are bound to the hostname. In Chrome, DevTools → More
tools → WebAuthn adds a virtual authenticator if your machine has none.

To build and run the single binary:

```sh
make build && ./24cups -db advent.db   # http://localhost:8080
```

Or with Docker (local only; see [deployment.md](deployment.md) for a real
deployment):

```sh
docker build -t 24cups . && docker run -p 8080:8080 -v advent-data:/data 24cups
```

## Tests

```sh
make test   # go test ./... and the frontend typecheck
```

## How the rules work

- **Doors.** "Today" is the visitor's local date, capped at the date in UTC+14
  so a faked clock can't unlock doors early. Later doors are always sealed.
  Today's coffee name and origin are shown on its sealed door; the rest stays
  hidden until it's opened.
- **Signed-out visitors** see everything for days before today. Today's door
  stays sealed unless they turn on Spoilers (saved per device) or peek.
- **Blind tasting** is enforced by the API, not just the UI. Until you log a
  rated brew or choose to reveal, the API doesn't send a coffee's tasting
  notes, community scores, flavors or other people's brews.
- **Averages** are computed per user first, then across users, so one
  prolific brewer can't skew a coffee. Private brews can be excluded.
- **Flavors** count once per person per coffee. The list appears once 3
  people have tagged a coffee, and a typed-in flavor appears once 2 people
  use it.
