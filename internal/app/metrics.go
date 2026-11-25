package app

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// App-level metrics. Instruments come from the global meter provider, which
// is a no-op until telemetry.Setup installs a real one (instruments created
// earlier are bound when it does).

const instrumentationName = "github.com/dyladan/24cups/internal/app"

var (
	meter = otel.Meter(instrumentationName)

	mSignups, _ = meter.Int64Counter("twentyfourcups.signups",
		metric.WithDescription("Accounts created."), metric.WithUnit("{account}"))
	mLogins, _ = meter.Int64Counter("twentyfourcups.logins",
		metric.WithDescription("Sign-in attempts by outcome."), metric.WithUnit("{attempt}"))
	mBrews, _ = meter.Int64Counter("twentyfourcups.brews.created",
		metric.WithDescription("Brews logged."), metric.WithUnit("{brew}"))
	mRateLimited, _ = meter.Int64Counter("twentyfourcups.rate_limit.rejections",
		metric.WithDescription("Requests rejected by a rate limit or cap."), metric.WithUnit("{request}"))
	mTurnstile, _ = meter.Int64Counter("twentyfourcups.turnstile.verifications",
		metric.WithDescription("Turnstile token verifications by outcome."), metric.WithUnit("{verification}"))
)

func count(ctx context.Context, c metric.Int64Counter, attrs ...attribute.KeyValue) {
	c.Add(ctx, 1, metric.WithAttributes(attrs...))
}

func outcome(v string) attribute.KeyValue { return attribute.String("twentyfourcups.outcome", v) }

// authMethod is how someone signed up or in: password, passkey or recovery_code.
func authMethod(v string) attribute.KeyValue { return attribute.String("twentyfourcups.auth.method", v) }

// registerStreamGauge reports open live-update streams.
func (b *broker) registerStreamGauge() {
	_, _ = meter.Int64ObservableUpDownCounter("twentyfourcups.live_updates.streams",
		metric.WithDescription("Open live-update (SSE) connections."), metric.WithUnit("{connection}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			b.mu.Lock()
			n := len(b.subs)
			b.mu.Unlock()
			o.Observe(int64(n))
			return nil
		}))
}
