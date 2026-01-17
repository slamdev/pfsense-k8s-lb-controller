package main

import (
	"log/slog"
	"os"

	"github.com/slamdev/pfsense-k8s-lb-controller/pkg"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	_ "golang.org/x/mod/modfile" // transitive dependency that is not recognized by go mod tidy
)

func main() {
	mgr, err := pkg.NewManager()
	if err != nil {
		slog.Error("failed to create manager", "err", err)
		os.Exit(1)
	}

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		slog.Error("unable to run manager", "err", err)
		os.Exit(1)
	}

	slog.Info("manager is stopped")
}
