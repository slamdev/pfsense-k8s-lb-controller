package business

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//nolint:unused
type reconciler struct {
	k8s     client.Client
	pfsense PfsenseService
}

func NewReconciler(k8s client.Client, pfsense PfsenseService) reconcile.Reconciler {
	return &reconciler{
		k8s:     k8s,
		pfsense: pfsense,
	}
}

func (r reconciler) Reconcile(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
