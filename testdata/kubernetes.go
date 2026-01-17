package testdata

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func KubernetesCluster() (string, manager.RunnableFunc, error) {
	host := "host.docker.internal"

	k3sContainer, err := k3s.Run(context.Background(),
		"rancher/k3s:v1.33.6-k3s1",
		testcontainers.WithLogger(testLogger{}),
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{ContainerRequest: testcontainers.ContainerRequest{ConfigModifier: func(config *container.Config) {
			config.Hostname = host
		}}}),
	)

	if err != nil {
		return "", nil, err
	}

	kubeConfigYaml, err := k3sContainer.GetKubeConfig(context.Background())
	if err != nil {
		return "", nil, err
	}

	file, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	if err := os.WriteFile(file.Name(), kubeConfigYaml, 0600); err != nil {
		return "", nil, err
	}

	return file.Name(), func(ctx context.Context) error {
		<-ctx.Done()
		os.Remove(file.Name())
		return k3sContainer.Terminate(context.Background())
	}, nil
}

type testLogger struct{}

func (t testLogger) Printf(format string, v ...any) {
	slog.Info(fmt.Sprintf(format, v...))
}
