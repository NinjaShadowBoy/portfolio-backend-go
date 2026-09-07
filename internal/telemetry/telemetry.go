// Package telemetry wires the OpenTelemetry SDK for traces, metrics and logs.
//
// Everything is exported over OTLP/HTTP to whatever OTEL_EXPORTER_OTLP_ENDPOINT
// points at (the SigNoz collector in production). When that variable is unset,
// Setup installs nothing: the global providers stay no-op, instrumentation
// middleware costs nothing measurable, and local runs stay quiet.
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Config identifies this process in the telemetry backend.
type Config struct {
	ServiceName    string
	ServiceVersion string
	// Environment becomes deployment.environment.name, so production and
	// staging can be told apart in SigNoz even though they share a host.
	Environment string
}

// Enabled reports whether an OTLP endpoint is configured. The exporters read
// the endpoint themselves; this only decides whether to install the SDK.
func Enabled() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
}

// Setup installs the global tracer, meter and logger providers and routes the
// default slog (and therefore the standard log package) through the OTLP log
// exporter while still printing to stderr for `docker logs`.
//
// The returned function flushes and shuts everything down; call it before the
// process exits or the last few seconds of telemetry are lost.
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !Enabled() {
		return func(context.Context) error { return nil }, nil
	}

	// Both spellings of the environment attribute: the current semconv name,
	// and the older deployment.environment that SigNoz's span-metrics
	// processor keys on to split production from staging.
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentNameKey.String(cfg.Environment),
		attribute.String("deployment.environment", cfg.Environment),
	))
	if err != nil {
		return nil, err
	}

	var shutdowns []func(context.Context) error
	shutdown := func(ctx context.Context) error {
		var errs error
		for i := len(shutdowns) - 1; i >= 0; i-- {
			errs = errors.Join(errs, shutdowns[i](ctx))
		}
		return errs
	}
	fail := func(err error) (func(context.Context) error, error) {
		_ = shutdown(ctx)
		return nil, err
	}

	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return fail(err)
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	shutdowns = append(shutdowns, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return fail(err)
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(30*time.Second))),
		sdkmetric.WithResource(res),
	)
	shutdowns = append(shutdowns, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)
	if err := runtime.Start(runtime.WithMeterProvider(meterProvider)); err != nil {
		return fail(err)
	}

	logExporter, err := otlploghttp.New(ctx)
	if err != nil {
		return fail(err)
	}
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)
	shutdowns = append(shutdowns, loggerProvider.Shutdown)

	// slog.SetDefault also redirects the standard log package, so the existing
	// log.Printf calls reach SigNoz without being rewritten.
	slog.SetDefault(slog.New(fanout{
		slog.NewTextHandler(os.Stderr, nil),
		otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(loggerProvider)),
	}))

	return shutdown, nil
}

// fanout is a slog.Handler that writes every record to all of its handlers.
type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, record slog.Record) error {
	var errs error
	for _, h := range f {
		if h.Enabled(ctx, record.Level) {
			errs = errors.Join(errs, h.Handle(ctx, record.Clone()))
		}
	}
	return errs
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
