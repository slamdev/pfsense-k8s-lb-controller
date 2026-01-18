package business

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/slamdev/pfsense-k8s-lb-controller/pkg/integration"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//nolint:unused
type reconciler struct {
	k8s     client.Client
	pfsense PfsenseService
}

const finalizerName = "loadbalancer.example.com/ip-cleanup"
const loadBalancerClass = "example.com/my-lb"
const portsHashAnnotation = "loadbalancer.example.com/ports-hash"

func NewReconciler(k8s client.Client, pfsense PfsenseService) reconcile.Reconciler {
	return &reconciler{
		k8s:     k8s,
		pfsense: pfsense,
	}
}

func (r *reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	var svc corev1.Service
	if err := r.k8s.Get(ctx, req.NamespacedName, &svc); err != nil {
		if apierrors.IsNotFound(err) {
			// Already gone, nothing to do (finalizer would have handled cleanup)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get service: %w", err)
	}

	// Always handle deletion if we have a finalizer, even if service type changed
	if controllerutil.ContainsFinalizer(&svc, finalizerName) {
		if !svc.DeletionTimestamp.IsZero() || !r.isOurService(&svc) {
			return r.handleDeletion(ctx, &svc)
		}
	}

	// Filter: not our problem
	if !r.isOurService(&svc) {
		logger.V(1).Info("skipping service: not our load balancer class or type", "type", svc.Spec.Type, "lbClass", integration.FromPtr(svc.Spec.LoadBalancerClass, "<nil>"))
		return ctrl.Result{}, nil
	}

	// Handle create/update
	res, err := r.handleCreateOrUpdate(ctx, &svc)
	if err != nil {
		if apierrors.IsConflict(err) {
			logger.V(1).Info("conflict updating service, requeuing", "error", err)
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}
	return res, nil
}

func (r *reconciler) isOurService(svc *corev1.Service) bool {
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}
	if svc.Spec.LoadBalancerClass == nil {
		return false
	}
	return *svc.Spec.LoadBalancerClass == loadBalancerClass
}

func (r *reconciler) handleCreateOrUpdate(ctx context.Context, svc *corev1.Service) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Add finalizer if missing
	if !controllerutil.ContainsFinalizer(svc, finalizerName) {
		controllerutil.AddFinalizer(svc, finalizerName)
		if err := r.k8s.Update(ctx, svc); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		logger.V(0).Info("added finalizer to service")
		// the update triggers another reconcile
		return ctrl.Result{}, nil
	}

	ports := extractServicePorts(svc)
	currentPortsHash := computePortsHash(ports)

	// Assign IP from external LB if not already assigned
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		ip, err := r.pfsense.AllocateIP(ctx, svc.Namespace, svc.Name, ports)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("allocate IP: %w", err)
		}

		svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
			{
				IP:     ip,
				IPMode: integration.ToPointer(corev1.LoadBalancerIPModeVIP),
				Ports:  r.toLoadBalancerIngressPorts(ports),
			},
		}
		if err := r.k8s.Status().Update(ctx, svc); err != nil {
			// Failed to persist — release the IP to avoid leak
			rerr := r.pfsense.ReleaseIP(ctx, ip)
			return ctrl.Result{}, fmt.Errorf("update status: %w", errors.Join(err, rerr))
		}
		logger.V(0).Info("assigned load balancer IP", "ip", ip)

		// Store ports hash in annotation
		if err := r.updateServicePorts(ctx, svc, currentPortsHash); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		ip := svc.Status.LoadBalancer.Ingress[0].IP
		logger.V(0).Info("service already has load balancer IP", "ip", ip)

		// Check if ports have changed
		lastPortsHash := svc.Annotations[portsHashAnnotation]
		if lastPortsHash != currentPortsHash {
			logger.V(0).Info("ports changed, updating pfsense", "ip", ip, "oldHash", lastPortsHash, "newHash", currentPortsHash)
			if err := r.pfsense.UpdatePorts(ctx, ip, ports); err != nil {
				return ctrl.Result{}, fmt.Errorf("update ports: %w", err)
			}

			// Update ports hash annotation
			if err := r.updateServicePorts(ctx, svc, currentPortsHash, ports...); err != nil {
				return ctrl.Result{}, err
			}
			logger.V(0).Info("updated ports in pfsense", "ip", ip)
		}
	}

	return ctrl.Result{}, nil
}

func (r *reconciler) updateServicePorts(ctx context.Context, svc *corev1.Service, hash string, ports ...ServicePort) error {
	if svc.Annotations == nil {
		svc.Annotations = make(map[string]string)
	}
	svc.Annotations[portsHashAnnotation] = hash

	if len(ports) > 0 {
		svc.Status.LoadBalancer.Ingress[0].Ports = r.toLoadBalancerIngressPorts(ports)
		if err := r.k8s.Status().Update(ctx, svc); err != nil {
			return fmt.Errorf("update status ports: %w", err)
		}
	}

	if err := r.k8s.Update(ctx, svc); err != nil {
		return fmt.Errorf("update ports hash annotation: %w", err)
	}
	return nil
}

func extractServicePorts(svc *corev1.Service) []ServicePort {
	ports := make([]ServicePort, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		ports = append(ports, ServicePort{
			Name:        p.Name,
			Protocol:    string(p.Protocol),
			AppProtocol: p.AppProtocol,
			NodePort:    p.NodePort,
		})
	}
	return ports
}

func computePortsHash(ports []ServicePort) string {
	data, _ := json.Marshal(ports)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (r *reconciler) handleDeletion(ctx context.Context, svc *corev1.Service) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(svc, finalizerName) {
		logger.V(0).Info("no finalizer present on service, skipping deletion handling")
		// No finalizer, nothing to clean up
		return ctrl.Result{}, nil
	}

	// Release IP from external LB
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			if err := r.pfsense.ReleaseIP(ctx, ingress.IP); err != nil {
				// Log and retry — don't remove finalizer until cleanup succeeds
				return ctrl.Result{}, fmt.Errorf("release IP %s: %w", ingress.IP, err)
			}
			logger.V(0).Info("released load balancer IP", "ip", ingress.IP)
		}
	}

	// Cleanup done — remove finalizer
	controllerutil.RemoveFinalizer(svc, finalizerName)
	if err := r.k8s.Update(ctx, svc); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
	}
	logger.V(0).Info("removed finalizer from service")

	return ctrl.Result{}, nil
}

func (r *reconciler) toLoadBalancerIngressPorts(ports []ServicePort) []corev1.PortStatus {
	return integration.MapSlice(ports, func(p ServicePort) corev1.PortStatus {
		return corev1.PortStatus{
			Port:     p.NodePort,
			Protocol: corev1.Protocol(p.Protocol),
		}
	})
}
