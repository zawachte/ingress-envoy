/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/ingress-envoy/pkg/envoy"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	nodeID        string
	snapshotCache cache.SnapshotCache
}

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ingress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	ingress := &networkingv1.Ingress{}
	if err := r.Client.Get(ctx, req.NamespacedName, ingress); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Initialize the patch helper.
	//patchHelper, err := patch.NewHelper(ingress, r.Client)
	//if err != nil {
	//	return ctrl.Result{}, err
	//}

	// Handle deletion reconciliation loop.
	if !ingress.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, ingress)
	}

	return r.reconcile(ctx, ingress)
}

func (r *IngressReconciler) reconcile(ctx context.Context, ingress *networkingv1.Ingress) (ctrl.Result, error) {

	// delta

	version, err := r.getXDSVersionFromConfigMap(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	simps := []envoy.SimpleEnvoyConfig{}

	for _, rule := range ingress.Spec.Rules {
		svc := rule.HTTP.Paths[0].Backend.Service
		port := svc.Port.Number
		clusterName := fmt.Sprintf("%s||%d", svc.Name, port)
		path := ingress.Spec.Rules[0].HTTP.Paths[0].Path
		routeName := fmt.Sprintf("%s_route", svc.Name)
		listenerName := fmt.Sprintf("%s_listener", svc)

		simps = append(simps, envoy.SimpleEnvoyConfig{
			ClusterName:  clusterName,
			Port:         uint32(port),
			RouteName:    routeName,
			PathPrefix:   path,
			ListenerName: listenerName,
		})

	}
	params := envoy.GenerateSnapshotParams{
		Version:            version,
		SimpleEnvoyConfigs: simps,
	}

	snapshot := envoy.GenerateSnapshot(params)

	err = envoy.SetSnapshot(ctx, r.nodeID, r.snapshotCache, snapshot)
	if err != nil {
		return ctrl.Result{}, err
	}

	//persist change
	err = r.setXDSVersion(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) getXDSVersionFromConfigMap(ctx context.Context) (string, error) {

	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{
		Name:      "version-map",
		Namespace: "ingress-envoy-system",
	}

	if err := r.Client.Get(ctx, key, cm); err != nil {
		return "", err
	}

	return cm.Data["version"], nil
}

func (r *IngressReconciler) setXDSVersion(ctx context.Context) error {

	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{
		Name:      "version-map",
		Namespace: "ingress-envoy-system",
	}

	if err := r.Client.Get(ctx, key, cm); err != nil {
		return err
	}

	versionInt, err := strconv.Atoi(cm.Data["version"])
	if err != nil {
		return err
	}
	cm.Data["version"] = strconv.Itoa(versionInt + 1)

	if err := r.Client.Patch(ctx, cm, client.Merge, &client.PatchOptions{}); err != nil {
		return err
	}

	return nil
}

func (r *IngressReconciler) reconcileDelete(ctx context.Context, ingress *networkingv1.Ingress) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(r)
}
