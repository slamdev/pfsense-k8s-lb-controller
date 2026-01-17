//nolint:unused
package svc

import (
	"errors"
	"fmt"

	"alexejk.io/go-xmlrpc"
	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"
)

type pfsenseService struct {
	client *xmlrpc.Client
	dryRun bool
}

type PfsenseService any

func NewPfsenseService(client *xmlrpc.Client, dryRun bool) PfsenseService {
	return &pfsenseService{
		client: client,
		dryRun: dryRun,
	}
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
