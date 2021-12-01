/*
Copyright 2021.

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
	"flag"
	"os"
	"sync"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	l7mpiov1 "github.com/davidkornel/operator/api/v1"
	"github.com/davidkornel/operator/controllers"

	servicemeshv1 "github.com/davidkornel/operator/api/v1"
	"github.com/davidkornel/operator/controlplane"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	metricsAddr          string
	enableLeaderElection bool
	probeAddr            string

	//controlplane
	l       controlplane.Logger
	port    uint
	nodeID  string
	tcpPort uint
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(l7mpiov1.AddToScheme(scheme))
	utilruntime.Must(servicemeshv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	l = controlplane.Logger{}
	l.Logger = zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(l.Logger)

	//cluster state
	//state.ClusterState = state.Cluster{}

	//controlplane

	flag.BoolVar(&l.Debug, "debug", false, "Enable xDS server debug logging")

	// The port that this xDS server listens on
	flag.UintVar(&port, "port", 18000, "xDS management server port")

	// Tell Envoy to use this Node ID
	flag.StringVar(&nodeID, "nodeID", "test-id", "Node ID")

	// The port that this xDS server listens on
	flag.UintVar(&tcpPort, "tcpPort", 1234, "TCP server port")

}

func main() {
	flag.Parse()

	//new waitgroup
	wg := new(sync.WaitGroup)
	wg.Add(2)

	// Run the xDS server
	go func() {
		setupLog.Info("controlplane runserver")
		controlplane.RunServer(port)
		wg.Done()
	}()

	////Run the UDP server
	//go func() {
	//	err := controlplane.Server(tcpPort, &l)
	//	if err != nil {
	//		return
	//	}
	//	wg.Done()
	//}()

	go func() {
		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     metricsAddr,
			Port:                   9443,
			HealthProbeBindAddress: probeAddr,
			LeaderElection:         enableLeaderElection,
			LeaderElectionID:       "842ce177.l7mp.io",
			Namespace:              "",
		})
		if err != nil {
			setupLog.Error(err, "unable to start manager")
			os.Exit(1)
		}

		if err = (&controllers.VirtualServiceReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "VirtualService")
			os.Exit(1)
		}
		if err = (&controllers.PodReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Pod")
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
		wg.Done()
	}()
	wg.Wait()

}
