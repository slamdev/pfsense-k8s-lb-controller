//nolint:revive
package e2e

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/slamdev/pfsense-k8s-lb-controller/pkg"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"
	"github.com/slamdev/pfsense-k8s-lb-controller/testdata"
)

//go:embed *.yaml
var ConfigsFS embed.FS

const healthCheckTimeout = 5 * time.Second

func TestMain(m *testing.M) {
	if err := integration.BuildConfig("E2E_", "e2e", ConfigsFS, &testdata.Cfg); err != nil {
		slog.Error("failed to build config", "error", err)
		os.Exit(1)
	}

	if testdata.Cfg.App.AutoStart {
		if err := startApp(); err != nil {
			slog.Error("failed to start app", "err", err)
			os.Exit(1)
		}
	}

	code := m.Run()

	os.Exit(code)
}

func startApp() error {
	integration.ConfigureLogProvider(nil, "info", "text")

	pfsenseURL, pfsenseStart := testdata.MockPfsenseServer()
	os.Setenv("APP_PFSENSE_URL", pfsenseURL)

	kubeconfigPath, k8sStart, err := testdata.KubernetesCluster()
	if err != nil {
		return fmt.Errorf("failed to setup kubernetes cluster: %w", err)
	}
	os.Setenv("KUBECONFIG", kubeconfigPath)

	os.Setenv("APP_TELEMETRY_METRICS_ENABLED", "false")
	healthPort := testdata.GetFreePort()
	os.Setenv("APP_TELEMETRY_HEALTH_BINDADDRESS", fmt.Sprintf(":%d", healthPort))

	mgr, err := pkg.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	if err := mgr.Add(pfsenseStart); err != nil {
		return fmt.Errorf("failed to add pfsense mock server to manager: %w", err)
	}

	if err := mgr.Add(k8sStart); err != nil {
		return fmt.Errorf("failed to add kubernetes cluster to manager: %w", err)
	}

	go func() {
		if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
			slog.Error("failed to start manager", "err", err)
		}
	}()

	if err := waitForReady(context.Background(), healthPort); err != nil {
		return fmt.Errorf("failed to wait for readyz endpoint: %w", err)
	}

	slog.Info("connect to cluster", "cmd", "KUBECONFIG="+kubeconfigPath+" kubectl get svc")

	return nil
}

func waitForReady(ctx context.Context, healthPort int) error {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	healthURL := fmt.Sprintf("http://localhost:%d/readyz", healthPort)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for readyz endpoint after %v", healthCheckTimeout)
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, http.NoBody)
			if err != nil {
				continue
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}
