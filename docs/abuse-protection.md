# Abuse protection

A public instance is open to anyone, so the server limits what a single client
can do. The defaults are in `DefaultLimits` in `internal/app/ratelimit.go`.

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
IPv6 clients are grouped by /64.

Sign-up can also require a Cloudflare Turnstile check; see
[deployment.md](deployment.md#turnstile-bot-check-on-sign-up).

## Real client IPs

Behind a proxy or tunnel, set `CLIENT_IP_HEADER` (`CF-Connecting-IP` for
Cloudflare). Otherwise every visitor appears to come from the proxy and they
all share one rate-limit bucket.

Only set it if the app's port can't be reached except through the proxy:
anyone who can reach the port directly could fake the header. In Docker,
don't publish the port; attach the container to the proxy's network only.
`compose.yaml` already does this.

## Cloudflare settings

Suggested settings in the Cloudflare dashboard (free plan):

- Keep DDoS protection on (it's automatic). Use "Under Attack" mode only during
  an active attack.
- Add a WAF rate-limiting rule for paths starting with `/api/auth/` as an outer
  layer.
- Bot Fight Mode can challenge real browsers' API and live-update requests.
  Test it before relying on it.
