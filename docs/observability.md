# Observability (OpenTelemetry)

The server exports traces, metrics and logs over OTLP. It's configured only
with the standard `OTEL_*` environment variables and is off unless
`OTEL_EXPORTER_OTLP_ENDPOINT` is set. `OTEL_SDK_DISABLED=true` also turns it
off.

## Traces

- A server span per API request, named by route (e.g.
  `GET /api/coffees/{id}`), using the stable HTTP semantic conventions with
  `http.route`.
- SQLite queries are child spans (`db.system.name=sqlite`).
- The Turnstile Siteverify call is a client span, with the outcome in
  `onyx_advent.turnstile.*` attributes.
- Not traced: static files, `/api/health`, the long-lived `/api/events`
  stream, and queries outside a request (startup).

Sampling is set with `OTEL_TRACES_SAMPLER` and `OTEL_TRACES_SAMPLER_ARG`.
Full-rate tracing cost about 10% of peak throughput in the
[benchmark](performance.md).

## Metrics

- HTTP: `http.server.request.duration` and request/response body sizes.
- Database: `db.client.operation.duration` and `db.sql.connection.*`, per pool
  (`read` / `write`).
- Go runtime: `go.*`.
- App:
  - `onyx_advent.signups`, `onyx_advent.logins` (by `onyx_advent.outcome`)
  - `onyx_advent.brews.created`
  - `onyx_advent.rate_limit.rejections` (by `onyx_advent.limiter`)
  - `onyx_advent.turnstile.verifications`
  - `onyx_advent.live_updates.streams`

## Logs

The server's `slog` output also goes out as OTLP logs, correlated with the
active span. It still prints to stderr.

## Resource attributes

`service.name=onyx-advent`, `service.version` (from the `VERSION` build arg),
`host.name`, and `deployment.environment.name` from `OTEL_RESOURCE_ATTRIBUTES`.

## With Docker Compose

`compose.yaml` sends OTLP/HTTP to whatever `OTEL_EXPORTER_OTLP_ENDPOINT` is in
`.env`, for example a collector on the Docker host at
`http://host.docker.internal:4318`. It asks for delta temporality for metrics,
which some backends (such as Dynatrace) require. Set
`host.name` in `OTEL_RESOURCE_ATTRIBUTES` so the data is labeled with the
machine rather than the container ID.
