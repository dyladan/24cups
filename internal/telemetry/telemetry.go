// Package telemetry sets up OpenTelemetry traces, metrics and logs.
//
// Everything is configured through the standard OTEL_* environment
// variables (endpoint, protocol, headers, sampler, export intervals,
// temporality, resource attributes), so nothing collector-specific lives in
// code. Telemetry is off unless an OTLP endpoint (or an explicit exporter)
// is configured, and OTEL_SDK_DISABLED=true turns it off entirely.
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

const ServiceName = "24cups"

// Enabled reports whether telemetry should be exported, from the standard
// environment variables.
func Enabled() bool {
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		return false
	}
	for _, k := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
		"OTEL_TRACES_EXPORTER",
		"OTEL_METRICS_EXPORTER",
		"OTEL_LOGS_EXPORTER",
	} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

// Setup installs global tracer, meter and logger providers and the W3C
// propagators, and starts Go runtime metrics. The returned function flushes
// and shuts everything down; call it on exit. If telemetry isn't enabled it
// returns a no-op and the global no-op providers stay in place.
func Setup(ctx context.Context, version string) (shutdown func(context.Context) error, err error) {
	if !Enabled() {
		return func(context.Context) error { return nil }, nil
	}

	// Defaults first; OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES (FromEnv)
	// come last so operators can override anything. host.name is always set:
	// without it, a collector may guess or mislabel the source of the data.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(ServiceName),
			semconv.ServiceVersion(version),
		),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithFromEnv(),
	)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, err
	}
	if res == nil {
		res = resource.Default()
	}

	var shutdowns []func(context.Context) error
	shutdown = func(ctx context.Context) error {
		var errs []error
		for i := len(shutdowns) - 1; i >= 0; i-- {
			errs = append(errs, shutdowns[i](ctx))
		}
		return errors.Join(errs...)
	}
	fail := func(e error) (func(context.Context) error, error) {
		_ = shutdown(ctx)
		return nil, e
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// Traces. The sampler comes from OTEL_TRACES_SAMPLER(_ARG).
	spanExp, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return fail(err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(spanExp), sdktrace.WithResource(res))
	shutdowns = append(shutdowns, tp.Shutdown)
	otel.SetTracerProvider(tp)

	// Metrics. Interval and temporality come from OTEL_METRIC_EXPORT_INTERVAL
	// and OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE.
	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return fail(err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithResource(res))
	shutdowns = append(shutdowns, mp.Shutdown)
	otel.SetMeterProvider(mp)
	if err := runtime.Start(runtime.WithMeterProvider(mp), runtime.WithMinimumReadMemStatsInterval(15*time.Second)); err != nil {
		return fail(err)
	}

	// Logs.
	logExp, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return fail(err)
	}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)), sdklog.WithResource(res))
	shutdowns = append(shutdowns, lp.Shutdown)
	global.SetLoggerProvider(lp)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Warn("opentelemetry", "err", err)
	}))
	return shutdown, nil
}

// LogHandler returns an slog handler that writes to stderr as before and,
// when telemetry is enabled, also emits OTLP log records correlated with the
// active span.
func LogHandler(level slog.Level) slog.Handler {
	text := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	if !Enabled() {
		return text
	}
	return fanout{text, otelslog.NewHandler(ServiceName, otelslog.WithLoggerProvider(global.GetLoggerProvider()))}
}

// fanout sends each record to every handler.
type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f fanout) WithGroup(name string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}
