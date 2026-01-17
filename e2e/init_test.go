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

func TestMain(m *testing.M) {
	if err := integration.BuildConfig("E2E_", "e2e", ConfigsFS, &testdata.Cfg); err != nil {
		slog.Error("failed to build config", "error", err)
		os.Exit(1)
	}

	var stopApp func() error
	if testdata.Cfg.App.AutoStart {
		stopApp = startApp()
	}

	code := m.Run()
	if stopApp != nil {
		if err := stopApp(); err != nil {
			slog.Error("failed to stop app", "err", err)
			os.Exit(1)
		}
	}
	os.Exit(code)
}

func startApp() func() error {
	pfsenseURL, pfsenseCloseFunc := testdata.MockPfsenseServer()

	os.Setenv("APP_PFSENSE_URL", pfsenseURL)

	os.Setenv("APP_TELEMETRY_METRICS_ENABLED", "false")

	healthPort := testdata.GetFreePort()
	os.Setenv("APP_TELEMETRY_HEALTH_BINDADDRESS", fmt.Sprintf(":%d", healthPort))

	app, err := pkg.NewManager()
	if err != nil {
		slog.Error("failed to start app", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		if err := app.Start(ctx); err != nil {
			slog.Error("failed to start app", "err", err)
			os.Exit(1)
		}
	}()
	for {
		r, err := http.Get(fmt.Sprintf("http://localhost:%d/readyz", healthPort)) //nolint:noctx
		if err == nil && r.StatusCode == http.StatusOK {
			break
		}
		r.Body.Close()
		time.Sleep(1 * time.Second)
	}

	slog.Info("app started for e2e tests")

	return func() error {
		pfsenseCloseFunc()
		cancel()
		return nil
	}
}
