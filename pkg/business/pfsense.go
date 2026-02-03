//nolint:unused,revive,staticcheck
package business

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"

	"alexejk.io/go-xmlrpc"
	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"
)

type ServicePort struct {
	Name        string  `json:"name,omitempty"`
	Protocol    string  `json:"protocol,omitempty"`
	AppProtocol *string `json:"appProtocol,omitempty"`
	NodePort    int32   `json:"nodePort,omitempty"`
	TargetPort  int32   `json:"targetPort,omitempty"`
}

const natConfigSection = "nat"

type pfsenseService struct {
	client     *xmlrpc.Client
	dryRun     bool
	subnet     netip.Prefix
	exclusions []integration.Range[netip.Addr]
}

type PfsenseService interface {
	AllocateIP(ctx context.Context, namespace string, name string, clusterIP string, ports []ServicePort) (string, error)
	UpdatePorts(ctx context.Context, loadBalancerIP string, ports []ServicePort) error
	ReleaseIP(ctx context.Context, loadBalancerIP string) error
}

func NewPfsenseService(client *xmlrpc.Client, dryRun bool, subnet netip.Prefix, exclusions ...integration.Range[netip.Addr]) PfsenseService {
	return &pfsenseService{
		client:     client,
		dryRun:     dryRun,
		subnet:     subnet,
		exclusions: exclusions,
	}
}

func (s *pfsenseService) AllocateIP(ctx context.Context, namespace string, name string, clusterIP string, ports []ServicePort) (string, error) {
	slog.InfoContext(ctx, "allocating IP from pfsense", "namespace", namespace, "name", name, "ports", ports)
	natSection, err := s.fetchNATSection()
	if err != nil {
		return "", fmt.Errorf("failed to fetch nat section; %w", err)
	}

	allocatedIPs := integration.UniqueSlice(integration.MapSlice(*natSection.Rule, func(r rule) string {
		return *r.Destination.Address
	}))

	ip, err := integration.AllocateIP(s.subnet, s.exclusions, allocatedIPs)
	if err != nil {
		return "", fmt.Errorf("failed to allocate IP; %w", err)
	}

	newRules := integration.MapSlice(ports, func(p ServicePort) rule {
		return rule{
			Destination: &destination{
				Address: &ip,
				Port:    integration.ToPointer(strconv.Itoa(int(p.TargetPort))),
			},
			Ipprotocol: integration.ToPointer("inet"),
			Protocol:   integration.ToPointer(strings.ToLower(p.Protocol)),
			Target:     &clusterIP,
			LocalPort:  integration.ToPointer(strconv.Itoa(int(p.NodePort))),
			Interface:  integration.ToPointer("wan"),
			Descr:      integration.ToPointer(fmt.Sprintf("%s/%s %d", namespace, name, p.TargetPort)),
		}
	})
	natSection.Rule = integration.ToPointer(append(*natSection.Rule, newRules...))

	if err := s.saveNATSection(natSection); err != nil {
		return "", fmt.Errorf("failed to save nat section; %w", err)
	}

	return ip, nil
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

func (s *pfsenseService) fetchNATSection() (nat, error) {
	req := &struct{ Data []string }{Data: []string{natConfigSection}}
	res := &integration.NestedXMLRPC[natStruct]{}
	if err := s.client.Call("pfsense.backup_config_section", req, res); err != nil {
		return nat{}, fmt.Errorf("failed to call %s; %w", "backup_config_section", err)
	}
	return *res.Nested.Nat, nil
}

func (s *pfsenseService) saveNATSection(section nat) error {
	req := &struct {
		Sections any
		Timeout  int
	}{
		Sections: map[string]any{natConfigSection: section},
		Timeout:  30,
	}

	if s.dryRun {
		slog.Info("dry run enabled, pfsense.restore_config_section call skipped", "request", integration.ToUnsafeJSONString(req))
		return nil
	}

	res := &integration.OperationResult{}
	if err := s.client.Call("pfsense.restore_config_section", req, res); err != nil {
		return fmt.Errorf("failed to call %s; %w", "restore_config_section", err)
	}
	if !res.Success {
		return errors.New("pfsense return 'false' as a result of config restoring")
	}
	return nil
}

type natStruct struct {
	Nat *nat `xmlrpc:"nat"`
}

//nolint:revive,staticcheck
type nat struct {
	Separator *string   `xmlrpc:"separator"`
	Outbound  *outbound `xmlrpc:"outbound"`
	Rule      *[]rule   `xmlrpc:"rule"`
}

type outbound struct {
	Rule *[]outboundRule `xmlrpc:"rule"`
	Mode *string         `xmlrpc:"mode"`
}

type outboundRule struct {
	Source         *source      `xmlrpc:"source"`
	Sourceport     *string      `xmlrpc:"sourceport"`
	Descr          *string      `xmlrpc:"descr"`
	Target         *string      `xmlrpc:"target"`
	Targetip       *string      `xmlrpc:"targetip"`
	TargetipSubnet *string      `xmlrpc:"targetip_subnet"`
	Interface      *string      `xmlrpc:"interface"`
	Poolopts       *string      `xmlrpc:"poolopts"`
	SourceHashKey  *string      `xmlrpc:"source_hash_key"`
	Destination    *destination `xmlrpc:"destination"`
	Updated        *timestamp   `xmlrpc:"updated"`
	Created        *timestamp   `xmlrpc:"created"`
}

type rule struct {
	Source           *source      `xmlrpc:"source"`
	Destination      *destination `xmlrpc:"destination"`
	Ipprotocol       *string      `xmlrpc:"ipprotocol"`
	Protocol         *string      `xmlrpc:"protocol"`
	Target           *string      `xmlrpc:"target"`
	LocalPort        *string      `xmlrpc:"local-port"`
	Interface        *string      `xmlrpc:"interface"`
	Descr            *string      `xmlrpc:"descr"`
	AssociatedRuleId *string      `xmlrpc:"associated-rule-id"`
	Updated          *timestamp   `xmlrpc:"updated"`
	Created          *timestamp   `xmlrpc:"created"`
}

type source struct {
	Network *string `xmlrpc:"network"`
	Any     *string `xmlrpc:"any"`
}

type destination struct {
	Any     *string `xmlrpc:"any"`
	Address *string `xmlrpc:"address"`
	Port    *string `xmlrpc:"port"`
}

type timestamp struct {
	Time     *string `xmlrpc:"time"`
	Username *string `xmlrpc:"username"`
}
