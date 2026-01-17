package pkg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	"alexejk.io/go-xmlrpc"
	"github.com/go-logr/logr"
	"github.com/slamdev/pfsense-k8s-lb-controller/configs"
	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/business"
	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func NewManager() (ctrl.Manager, error) {
	var appConfig configs.Config
	if err := integration.BuildConfig("APP_", "application", configs.Configs, &appConfig); err != nil {
		return nil, fmt.Errorf("failed to populate config; %w", err)
	}

	runnableTelemetry, err := configureTelemetry(context.Background(), appConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to configure telemetry; %w", err)
	}

	pfsenseClient, err := configurePfsenseClient(appConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to configure pfsense client; %w", err)
	}
	pfsenseService := business.NewPfsenseService(pfsenseClient, appConfig.DryRun)

	kubecfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get kubeconfig: %w", err)
	}

	ctrl.SetLogger(logr.FromSlogHandler(slog.Default().Handler()))

	healthProbeBindAddress := ""
	if appConfig.Telemetry.Health.Enabled {
		healthProbeBindAddress = appConfig.Telemetry.Health.BindAddress
	}

	metricsBindAddress := ""
	if appConfig.Telemetry.Metrics.Enabled {
		metricsBindAddress = appConfig.Telemetry.Metrics.BindAddress
	}

	mgr, err := ctrl.NewManager(kubecfg, manager.Options{
		HealthProbeBindAddress: healthProbeBindAddress,
		Metrics: metricsserver.Options{
			BindAddress: metricsBindAddress,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to set up overall controller manager: %w", err)
	}

	if err := mgr.Add(runnableTelemetry); err != nil {
		return nil, fmt.Errorf("unable to set up telemetry in controller manager: %w", err)
	}

	if err := mgr.AddReadyzCheck("pfsense", integration.PfsenseHealthCheck(pfsenseClient)); err != nil {
		return nil, fmt.Errorf("unable to set up pfsense health check in controller manager: %w", err)
	}
	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up health check in controller manager: %w", err)
	}

	reconciler := business.NewReconciler(mgr.GetClient(), pfsenseService)

	err = ctrl.
		NewControllerManagedBy(mgr).
		Named("app").
		For(&corev1.Service{}).
		Complete(reconciler)
	if err != nil {
		return nil, fmt.Errorf("unable to create controller: %w", err)
	}

	return mgr, nil
}

func configureTelemetry(ctx context.Context, appConfig configs.Config) (manager.RunnableFunc, error) {
	telemetryResource := integration.CreateTelemetryResource(ctx)

	integration.ConfigureLogProvider(telemetryResource, appConfig.Telemetry.Logs.Level, appConfig.Telemetry.Logs.Format)

	metricProvider, err := integration.ConfigureMetricProvider(ctx, telemetryResource)
	if err != nil {
		return nil, fmt.Errorf("failed to init metric provider; %w", err)
	}

	r := func(ctx context.Context) error {
		<-ctx.Done()
		return errors.Join(ctx.Err(), metricProvider.Shutdown(ctx))
	}

	return r, nil
}

func configurePfsenseClient(appConfig configs.Config) (*xmlrpc.Client, error) {
	pfsenseURL := url.URL(appConfig.Pfsense.URL)
	pfsenseClient, err := integration.CreatePfsenseClient(pfsenseURL.String(), appConfig.Pfsense.Username, appConfig.Pfsense.Password, appConfig.Pfsense.Insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create pfsense client; %w", err)
	}
	return pfsenseClient, nil
}
