package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/lmittmann/tint"
	"github.com/prometheus/client_golang/prometheus"
	slogotel "github.com/remychantenay/slog-otel"
	"go.opentelemetry.io/contrib/instrumentation/host"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

func CreateTelemetryResource(_ context.Context) *resource.Resource {
	res := resource.Default()
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Error("otel error", "err", err)
	}))
	return res
}

func ConfigureMetricProvider(_ context.Context, res *resource.Resource) (*metric.MeterProvider, error) {
	var reader metric.Reader
	// recreate default registry to remove built-in collectors
	// they are covered by otel
	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	prometheus.DefaultGatherer = reg
	var err error
	if reader, err = promexporter.New(); err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(reader),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	if err := host.Start(); err != nil {
		return nil, fmt.Errorf("failed to start host observer: %w", err)
	}

	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(5 * time.Second)); err != nil {
		return nil, fmt.Errorf("failed to start runtime observer: %w", err)
	}

	return mp, nil
}

// ConfigureLogProvider replace with OTEL log bridge when it's GA.
func ConfigureLogProvider(_ *resource.Resource, level string, format string) {
	lvl := slog.LevelDebug
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		//nolint:forbidigo,revive
		fmt.Printf("failed to parse log level: %v, fallback to DEBUG", err)
	}
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	} else {
		handler = tint.NewHandler(os.Stderr, &tint.Options{
			Level:      lvl,
			TimeFormat: time.TimeOnly,
		})
	}

	l := slog.New(slogotel.OtelHandler{Next: handler, NoTraceEvents: true})

	slog.SetDefault(l)
	otel.SetLogger(logr.FromSlogHandler(l.Handler()))
}
