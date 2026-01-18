//nolint:unused
package business

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"alexejk.io/go-xmlrpc"
	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"
)

type ServicePort struct {
	Name        string  `json:"name,omitempty"`
	Protocol    string  `json:"protocol,omitempty"`
	AppProtocol *string `json:"appProtocol,omitempty"`
	NodePort    int32   `json:"nodePort,omitempty"`
}

type pfsenseService struct {
	client *xmlrpc.Client
	dryRun bool
}

type PfsenseService interface {
	AllocateIP(ctx context.Context, namespace string, name string, ports []ServicePort) (string, error)
	UpdatePorts(ctx context.Context, ip string, ports []ServicePort) error
	ReleaseIP(ctx context.Context, ip string) error
}

func NewPfsenseService(client *xmlrpc.Client, dryRun bool) PfsenseService {
	return &pfsenseService{
		client: client,
		dryRun: dryRun,
	}
}

func (s *pfsenseService) AllocateIP(ctx context.Context, namespace string, name string, ports []ServicePort) (string, error) {
	slog.InfoContext(ctx, "allocating IP from pfsense", "namespace", namespace, "name", name, "ports", ports)
	return "10.3.1.1", nil
}

func (s *pfsenseService) UpdatePorts(ctx context.Context, ip string, ports []ServicePort) error {
	slog.InfoContext(ctx, "updating ports in pfsense", "ip", ip, "ports", ports)
	return nil
}

func (s *pfsenseService) ReleaseIP(ctx context.Context, ip string) error {
	slog.InfoContext(ctx, "releasing IP back to pfsense", "ip", ip)
	return nil
}

func (s *pfsenseService) execPhp(code string) error {
	req := &struct{ Data string }{Data: code}
	res := &integration.OperationResult{}
	if err := s.client.Call("pfsense.exec_php", req, res); err != nil {
		return fmt.Errorf("failed to exec php; %w", err)
	}
	if !res.Success {
		return errors.New("pfsense return 'false' as a result of exec php")
	}
	return nil
}
