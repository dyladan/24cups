# Performance

The app is built to handle a busy December on a single small machine.

## How it stays fast

- **Community stats are kept in memory.** Averages, opens, favorites, rating
  distributions and group leaderboards are loaded at startup. After each
  write, only the affected user/coffee pair is re-read, so page views never
  scan the brews table.
- **Reads and writes are split.** Reads use a pool of read-only SQLite
  connections; writes go through a single connection.
- **Live updates carry no data.** Pages update over Server-Sent Events
  (`GET /api/events`), but an event is only a hint like "coffee data changed".
  The browser then refetches the affected queries through the normal API, so
  the same privacy rules apply as for any other request. Your own changes
  aren't echoed back to the tab that made them, and hints about your recipes,
  grinders, settings and brews go only to your own devices. Hints are batched,
  and a tab refetches everything after reconnecting.

## Benchmarks

Benchmarks run with `-no-rate-limits` (one load generator is one IP). They
were measured on a Raspberry Pi 5 (4 GB) with data sized like December 24, and
the load generator on the same Pi limited to 1–2 cores.

| Scenario | Result |
|---|---|
| Mixed API traffic, 500 / 2,000 / 5,000 users | ~2,900 / 2,850 / 2,700 req/s, p95 ≈ 11 ms, no errors |
| 2,000 open pages + 2 brews/s (live-update refetch storm) | ~2,000 refetches/s, p95 2.7 ms, p99 9.5 ms, ≤ 1.2 cores |
| Memory | ~80 MB at 5,000 users; ~310 MB with 2,000 open live streams |

Full-rate tracing cost about 10% of peak throughput; see
[observability.md](observability.md).
