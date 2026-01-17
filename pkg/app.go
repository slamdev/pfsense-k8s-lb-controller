package pkg

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"alexejk.io/go-xmlrpc"
	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/business/svc"

	"github.com/slamdev/pfsense-k8s-lb-controller/configs"
	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"

	healthlib "github.com/alexliesenfeld/health"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

type App interface {
	Start() error
	Stop() error
}

type app struct {
	config         configs.Config
	actuatorServer integration.HTTPServer
	traceProvider  *trace.TracerProvider
	metricProvider *metric.MeterProvider
	healthChecker  healthlib.Checker
	pfsenseClient  *xmlrpc.Client
}

func NewApp() (App, error) {
	ctx := context.Background()
	app := app{}

	if err := integration.BuildConfig("APP_", "application", configs.Configs, &app.config); err != nil {
		return nil, fmt.Errorf("failed to populate config; %w", err)
	}

	if err := app.configureTelemetry(ctx); err != nil {
		return nil, fmt.Errorf("failed to configure telemetry; %w", err)
	}

	if err := app.configurePfsenseClient(); err != nil {
		return nil, fmt.Errorf("failed to configure pfsense client; %w", err)
	}

	app.configureHealthChecker()

	_ = svc.NewPfsenseService(app.pfsenseClient, app.config.DryRun)

	app.actuatorServer = integration.NewHTTPServer(app.config.Actuator.Port, integration.TelemetryHandler(app.healthChecker))
	return &app, nil
}

func (a *app) configureHealthChecker() {
	healthChecks := []healthlib.Check{
		integration.PfsenseHealthCheck(a.pfsenseClient),
	}
	a.healthChecker = integration.HealthChecker(healthChecks...)
}

func (a *app) configureTelemetry(ctx context.Context) error {
	telemetryResource := integration.CreateTelemetryResource(ctx)

	integration.ConfigureLogProvider(telemetryResource, a.config.Telemetry.Logs.Level, a.config.Telemetry.Logs.Format)

	var err error
	a.traceProvider, err = integration.ConfigureTraceProvider(ctx, telemetryResource, a.config.Telemetry.Traces.Output)
	if err != nil {
		return fmt.Errorf("failed to init tracer; %w", err)
	}

	a.metricProvider, err = integration.ConfigureMetricProvider(ctx, telemetryResource, a.config.Telemetry.Metrics.Output)
	if err != nil {
		return fmt.Errorf("failed to init metric provider; %w", err)
	}

	return nil
}

func (a *app) configurePfsenseClient() error {
	pfsenseURL := url.URL(a.config.Pfsense.URL)
	pfsenseClient, err := integration.CreatePfsenseClient(pfsenseURL.String(), a.config.Pfsense.Username, a.config.Pfsense.Password, a.config.Pfsense.Insecure)
	if err != nil {
		return fmt.Errorf("failed to create pfsense client; %w", err)
	}
	a.pfsenseClient = pfsenseClient
	return nil
}

func (a *app) Start() error {
	starters := []func() error{
		a.actuatorServer.Start,
		func() error { a.healthChecker.Start(); return nil },
	}
	done := make(chan error, len(starters))
	for i := range starters {
		starter := starters[i]
		go func(starter func() error) {
			done <- starter()
		}(starter)
	}

	for range cap(done) {
		if err := <-done; err != nil {
			return err
		}
	}

	return nil
}

func (a *app) Stop() error {
	a.healthChecker.Stop()
	ctx := context.Background()

	err := errors.Join(
		a.actuatorServer.Stop(ctx),
		a.traceProvider.Shutdown(ctx),
		a.metricProvider.Shutdown(ctx),
	)
	return err
}
