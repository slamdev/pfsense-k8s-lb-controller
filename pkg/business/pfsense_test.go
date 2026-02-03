package business

import (
	"net/netip"
	"testing"

	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"
	"github.com/slamdev/pfsense-k8s-lb-controller/testdata"
	"github.com/stretchr/testify/require"
)

func Test_should_verify_pfsense_service(t *testing.T) {
	t.Parallel()
	testdata.SetTestLogger(t)

	pfsenseURL, pfsenseStart := testdata.MockPfsenseServer()
	go func() {
		if err := pfsenseStart(t.Context()); err != nil {
			t.Logf("mock pfsense server stopped with error: %v", err)
		}
	}()

	client, err := integration.CreatePfsenseClient(pfsenseURL, "", "", true)
	require.NoError(t, err)

	subnet, err := netip.ParsePrefix("150.150.150.0/24")
	require.NoError(t, err)

	svc := NewPfsenseService(client, false, subnet)

	ip, err := svc.AllocateIP(t.Context(), testdata.RndName(), testdata.RndName(), "10.1.2.3", []ServicePort{
		{
			Name:        "http",
			Protocol:    "TCP",
			AppProtocol: integration.ToPointer("http"),
			NodePort:    8080,
			TargetPort:  80,
		},
	})

	require.NoError(t, err)
	require.NotEmpty(t, ip)
}
