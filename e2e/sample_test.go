//nolint:wrapcheck
package e2e

import (
	"testing"

	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"
	"github.com/slamdev/pfsense-k8s-lb-controller/testdata"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func Test_should_work(t *testing.T) {
	t.Parallel()
	kubecfg, err := config.GetConfig()
	require.NoError(t, err)
	k8s, err := kubernetes.NewForConfig(kubecfg)
	require.NoError(t, err)

	svcName := testdata.RndName()

	_, err = k8s.CoreV1().Services("default").Create(t.Context(), &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: svcName},
		Spec: corev1.ServiceSpec{
			LoadBalancerClass: integration.ToPointer("example.com/my-lb"),
			Type:              "LoadBalancer",
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
				},
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromInt32(8443),
				},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	svc := testdata.WaitFor(t, func(c *assert.CollectT) (corev1.Service, error) {
		svc, err := k8s.CoreV1().Services("default").Get(t.Context(), svcName, metav1.GetOptions{})
		if err != nil {
			return corev1.Service{}, err
		}
		assert.NotEmpty(c, svc.Status.LoadBalancer.Ingress)
		return *svc, nil
	})

	t.Logf("Service LoadBalancer Ingress: %+v", svc.Status.LoadBalancer.Ingress)

	err = k8s.CoreV1().Services("default").Delete(t.Context(), svcName, metav1.DeleteOptions{})
	require.NoError(t, err)

	testdata.WaitFor(t, func(c *assert.CollectT) (any, error) {
		_, err := k8s.CoreV1().Services("default").Get(t.Context(), svcName, metav1.GetOptions{})
		assert.Error(c, err)
		if err != nil {
			assert.True(c, apierrors.IsNotFound(err), err)
		}
		return nil, nil //nolint:nilnil
	})
}
