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

package main

import (
	"context"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"k8s.io/ingress-envoy/controllers"
	envoydiscovery "k8s.io/ingress-envoy/pkg/discovery"
	"k8s.io/ingress-envoy/pkg/envoy"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var serviceMode bool
	var nodeID string
	var clusterID string
	var namespace string
	var serviceName string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&nodeID, "node-id", "ingress-envoy-0", "The envoy node id")
	flag.StringVar(&clusterID, "cluster-id", "ingress-envoy", "The envoy cluster id")
	flag.StringVar(&namespace, "namespace", "ingress-envoy-system", "The namespace the controller is running in")
	flag.StringVar(&serviceName, "service-name", "ingress-envoy-controller-manager", "The service name in front of the ingress controller")
	flag.BoolVar(&serviceMode, "service-mode", false, "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	envoyDiscoveryParams := envoydiscovery.EnvoyServerParams{
		NodeID: nodeID,
	}

	envoyServer := envoydiscovery.NewEnvoyServer(envoyDiscoveryParams)

	go func() {
		err := envoyServer.Serve(context.Background())
		if err != nil {
			setupLog.Error(err, "problem running envoy xds server")
			os.Exit(1)
		}
	}()

	pc := envoy.ProxyConfig{
		Node:        nodeID,
		ClusterName: clusterID,
	}

	proxy := envoy.NewProxy(pc)
	abortCh := make(chan error, 1)

	go func() {
		err := proxy.Run(nil, 1, abortCh)
		if err != nil {
			panic(err)
		}
		//proxy.Cleanup(epoch)
		//a.statusCh <- exitStatus{epoch: epoch, err: err}
	}()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "aac4af9d.k8s.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.IngressReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		NodeID:        nodeID,
		SnapshotCache: envoyServer.GetSnapshotCache(),
		ServiceMode:   serviceMode,
		Namespace:     namespace,
		ServiceName:   serviceName,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Ingress")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
