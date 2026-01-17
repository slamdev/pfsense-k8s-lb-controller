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

	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"
	"github.com/slamdev/pfsense-k8s-lb-controller/testdata"
)

//go:embed *.yaml
var ConfigsFS embed.FS

const healthCheckTimeout = 15 * time.Second

func TestMain(m *testing.M) {
	if err := integration.BuildConfig("E2E_", "e2e", ConfigsFS, &testdata.Cfg); err != nil {
		slog.Error("failed to build config", "error", err)
		os.Exit(1)
	}

	var cancel context.CancelFunc
	var cleanup func()
	if testdata.Cfg.App.AutoStart {
		var err error
		cancel, cleanup, err = startApp()
		if err != nil {
			slog.Error("failed to start app", "err", err)
			os.Exit(1)
		}
	}

	code := m.Run()

	if cancel != nil {
		cancel()
	}
	if cleanup != nil {
		cleanup()
	}

	os.Exit(code)
}

func startApp() (context.CancelFunc, func(), error) {
	pfsenseURL, pfsenseCloseFunc := testdata.MockPfsenseServer()

	os.Setenv("APP_PFSENSE_URL", pfsenseURL)
	os.Setenv("APP_TELEMETRY_METRICS_ENABLED", "false")

	healthPort := testdata.GetFreePort()
	os.Setenv("APP_TELEMETRY_HEALTH_BINDADDRESS", fmt.Sprintf(":%d", healthPort))

	app, err := pkg.NewManager()
	if err != nil {
		pfsenseCloseFunc()
		return nil, nil, fmt.Errorf("failed to create manager: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	appErrCh := make(chan error, 1)
	go func() {
		if err := app.Start(ctx); err != nil {
			appErrCh <- err
		}
		close(appErrCh)
	}()

	if err := waitForReady(ctx, healthPort, appErrCh); err != nil {
		cancel()
		pfsenseCloseFunc()
		return nil, nil, fmt.Errorf("health check failed: %w", err)
	}

	slog.Info("app started for e2e tests")

	return cancel, pfsenseCloseFunc, nil
}

func waitForReady(ctx context.Context, healthPort int, appErrCh <-chan error) error {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	healthURL := fmt.Sprintf("http://localhost:%d/readyz", healthPort)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for readyz endpoint after %v", healthCheckTimeout)
		case err := <-appErrCh:
			if err != nil {
				return fmt.Errorf("app failed to start: %w", err)
			}
			return fmt.Errorf("app exited unexpectedly")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
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
