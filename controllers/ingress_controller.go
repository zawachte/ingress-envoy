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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	envoy "k8s.io/ingress-envoy/pkg/discovery"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// IngressReconciler reconciles a Ingress object
type IngressReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	EnvoyManager *envoy.EnvoyManager
	ServiceMode  bool
	ServiceName  string
	Namespace    string
}

const (
	envoyIngressLabel        = "networking.k8s.io/ingress-envoy-controller"
	rewriteTargetAnnotations = "envoy.ingress.kubernetes.io/rewrite-target"
)

//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingressclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

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

	if ingress.Spec.IngressClassName != nil {
		ingressClassKey := client.ObjectKey{
			Namespace: ingress.Namespace,
			Name:      *ingress.Spec.IngressClassName,
		}

		ingressClass := &networkingv1.IngressClass{}
		if err := r.Client.Get(ctx, ingressClassKey, ingressClass); err != nil {
			return ctrl.Result{}, err
		}

		if ingressClass.Spec.Controller != envoyIngressLabel {
			return ctrl.Result{}, nil
		}
	}

	// Handle deletion reconciliation loop.
	if !ingress.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, ingress)
	}

	return r.reconcile(ctx, ingress)
}

func (r *IngressReconciler) buildSimpleEnvoyConfig(ctx context.Context, version string) (*envoy.GenerateSnapshotParams, error) {

	simpMap := make(map[string][]envoy.SimpleEnvoyConfig)

	ingressList, err := r.getAllIngresses(ctx)
	if err != nil {
		return nil, err
	}

	tlsConfigs := []envoy.TLSConfig{}
	for _, ingress := range ingressList.Items {
		if !ingress.ObjectMeta.DeletionTimestamp.IsZero() {
			continue
		}

		for _, tls := range ingress.Spec.TLS {
			tlsConfig, err := r.tlsSecretToEnvoyTLSConfig(ctx, tls.SecretName, ingress.Namespace)
			if err != nil {
				return nil, err
			}

			tlsConfigs = append(tlsConfigs, tlsConfig)
		}

		for _, rule := range ingress.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				svc := path.Backend.Service
				port := svc.Port.Number
				clusterName := fmt.Sprintf("%s||%d", svc.Name, port)
				path := path.Path

				endpoints := []string{}
				if r.ServiceMode {
					endpoints, err = r.getServiceClusterIP(ctx, svc.Name, ingress.Namespace)
					if err != nil {
						continue
					}
				} else {
					endpoints, err = r.getServiceEndpoints(ctx, svc.Name, ingress.Namespace)
					if err != nil {
						continue
					}
				}

				rewrite := ""
				val, ok := ingress.ObjectMeta.Annotations[rewriteTargetAnnotations]
				if ok {
					rewrite = val
				}

				hostName := "*"
				if rule.Host != "" {
					hostName = rule.Host
				}

				simpMap[hostName] = append(simpMap[hostName], envoy.SimpleEnvoyConfig{
					ClusterName: clusterName,
					Port:        uint32(port),
					Path: envoy.SimpleEnvoyPath{
						Value:        path,
						Type:         envoy.SimpleEnvoyPathTypePrefix,
						RewriteRegex: rewrite,
					},
					Endpoints: endpoints,
				})
			}
		}
	}

	params := &envoy.GenerateSnapshotParams{
		Version:              version,
		SimpleEnvoyConfigMap: simpMap,
		ListenerConfigs: []envoy.SimpleListenerConfig{
			{
				ListenerName: envoy.ListenerNameHTTP,
				ListenerPort: envoy.ListenerPortHTTP,
			},
			{
				ListenerName: envoy.ListenerNameHTTPS,
				ListenerPort: envoy.ListenerPortHTTPS,
				TLSConfig:    &tlsConfigs,
			},
		},
		RouteName: envoy.RouteName,
	}

	return params, nil
}

func (r *IngressReconciler) reconcile(ctx context.Context, ingress *networkingv1.Ingress) (ctrl.Result, error) {

	version := fmt.Sprintf("%s-%s", ingress.UID, ingress.ResourceVersion)
	params, err := r.buildSimpleEnvoyConfig(ctx, version)
	if err != nil {
		return ctrl.Result{}, err
	}

	snapshot, err := envoy.GenerateSnapshot(*params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.EnvoyManager.PushChanges(ctx, *snapshot)
	if err != nil {
		return ctrl.Result{}, err
	}

	svc, err := r.getService(ctx, r.ServiceName, r.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	patchHelper, err := patch.NewHelper(ingress, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	ingress.Status.LoadBalancer = svc.Status.LoadBalancer

	if err := patchHelper.Patch(ctx, ingress); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) getService(ctx context.Context, serviceName string, namespace string) (*corev1.Service, error) {
	svc := &corev1.Service{}
	key := client.ObjectKey{
		Name:      serviceName,
		Namespace: namespace,
	}
	if err := r.Client.Get(ctx, key, svc); err != nil {
		return nil, err
	}

	return svc, nil
}

func (r *IngressReconciler) tlsSecretToEnvoyTLSConfig(ctx context.Context, secretName string, namespace string) (envoy.TLSConfig, error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Name:      secretName,
		Namespace: namespace,
	}

	if err := r.Client.Get(ctx, key, secret); err != nil {
		return envoy.TLSConfig{}, err
	}

	cert, ok := secret.Data[corev1.TLSCertKey]
	if !ok {
		return envoy.TLSConfig{}, errors.New("invalid TLS secret")
	}

	privKey, ok := secret.Data[corev1.TLSPrivateKeyKey]
	if !ok {
		return envoy.TLSConfig{}, errors.New("invalid TLS secret")
	}

	return envoy.TLSConfig{
		PrivateKeyString:  string(privKey),
		CertificateString: string(cert),
	}, nil
}

func (r *IngressReconciler) getServiceClusterIP(ctx context.Context, serviceName string, namespace string) ([]string, error) {
	svc, err := r.getService(ctx, serviceName, namespace)
	if err != nil {
		return nil, err
	}

	return []string{svc.Spec.ClusterIP}, nil
}

func (r *IngressReconciler) getAllIngresses(ctx context.Context) (*networkingv1.IngressList, error) {
	ingressList := &networkingv1.IngressList{}
	if err := r.Client.List(context.TODO(), ingressList); err != nil {
		return nil, nil
	}

	return ingressList, nil
}

func (r *IngressReconciler) getServiceEndpoints(ctx context.Context, serviceName string, namespace string) ([]string, error) {

	svc := &corev1.Service{}
	key := client.ObjectKey{
		Name:      serviceName,
		Namespace: namespace,
	}
	if err := r.Client.Get(ctx, key, svc); err != nil {
		return nil, err
	}

	returnEndpoints := []string{}

	for k, v := range svc.Spec.Selector {
		podList := &corev1.PodList{}

		labelString := fmt.Sprintf("%s=%s", k, v)
		lblSelector, err := labels.Parse(labelString)
		if err != nil {
			return nil, err
		}

		if err := r.Client.List(ctx, podList, &client.ListOptions{LabelSelector: lblSelector}); err != nil {
			return nil, err
		}

		for _, pod := range podList.Items {
			returnEndpoints = append(returnEndpoints, string(pod.Status.PodIP))
		}

	}
	return returnEndpoints, nil
}

func (r *IngressReconciler) reconcileDelete(ctx context.Context, ingress *networkingv1.Ingress) (ctrl.Result, error) {

	version := fmt.Sprintf("%s-%s", ingress.UID, ingress.ResourceVersion)
	params, err := r.buildSimpleEnvoyConfig(ctx, version)
	if err != nil {
		return ctrl.Result{}, err
	}

	snapshot, err := envoy.GenerateSnapshot(*params)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.EnvoyManager.PushChanges(ctx, *snapshot)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *IngressReconciler) serviceToIngress(o client.Object) []ctrl.Request {

	svc, ok := o.(*corev1.Service)
	if !ok {
		panic(fmt.Sprintf("Expected a Service but got a %T", o))
	}

	// improve with filter
	ingressList := &networkingv1.IngressList{}
	if err := r.Client.List(context.TODO(), ingressList); err != nil {
		return nil
	}

	requestList := []ctrl.Request{}

	for _, ingress := range ingressList.Items {
		for _, rule := range ingress.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				ingressService := path.Backend.Service
				if svc.Name == ingressService.Name {
					requestList = append(requestList, ctrl.Request{
						NamespacedName: ObjectKey(&ingress),
					})
				}
			}
		}
	}

	return requestList
}

// ObjectKey returns client.ObjectKey for the object.
func ObjectKey(object metav1.Object) client.ObjectKey {
	return client.ObjectKey{
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
}

func (r *IngressReconciler) podToIngress(o client.Object) []ctrl.Request {

	pod, ok := o.(*corev1.Pod)
	if !ok {
		panic(fmt.Sprintf("Expected a Pod but got a %T", o))
	}

	// improve with filter
	labels := pod.ObjectMeta.Labels
	serviceList := &corev1.ServiceList{}
	if err := r.Client.List(context.TODO(), serviceList); err != nil {
		return nil
	}

	filteredServiceList := &corev1.ServiceList{}

	for _, svc := range serviceList.Items {
		for k, v := range svc.Spec.Selector {
			if val, ok := labels[k]; ok {
				if val == v {
					filteredServiceList.Items = append(filteredServiceList.Items, svc)
				}
			}
		}
	}

	ingressList := &networkingv1.IngressList{}
	if err := r.Client.List(context.TODO(), ingressList); err != nil {
		return nil
	}

	requestList := []ctrl.Request{}

	for _, ingress := range ingressList.Items {
		for _, rule := range ingress.Spec.Rules {
			for _, path := range rule.HTTP.Paths {
				ingressService := path.Backend.Service
				for _, svc := range filteredServiceList.Items {
					if svc.Name == ingressService.Name {
						requestList = append(requestList, ctrl.Request{
							NamespacedName: ObjectKey(&ingress),
						})
					}
				}
			}
		}
	}

	return requestList
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Watches(
			&source.Kind{Type: &corev1.Service{}},
			handler.EnqueueRequestsFromMapFunc(r.serviceToIngress),
		).
		Watches(
			&source.Kind{Type: &corev1.Pod{}},
			handler.EnqueueRequestsFromMapFunc(r.podToIngress),
		).
		Complete(r)
}
