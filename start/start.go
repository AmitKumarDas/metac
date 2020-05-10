/*
Copyright 2019 The MayaData Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package start

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/golang/glog"
	"go.opencensus.io/stats/view"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"openebs.io/metac/server"
)

var (
	discoveryInterval = flag.Duration(
		"discovery-interval",
		30*time.Second,
		"How often to refresh discovery cache to pick up newly-installed resources",
	)
	informerRelist = flag.Duration(
		"cache-flush-interval",
		30*time.Minute,
		"How often to flush local caches and relist objects from the API server",
	)
	debugAddr = flag.String(
		"debug-addr",
		":9999",
		"The address to bind the debug http endpoints",
	)
	kubeAPIServerURL = flag.String(
		"kube-apiserver-url",
		"",
		`Kubernetes api server url (same format as used by kubectl).
		If not specified, uses in-cluster config`,
	)
	clientConfigPath = flag.String(
		"client-config-path",
		"",
		`Path to kubeconfig file (same format as used by kubectl).
		If not specified, uses in-cluster config`,
	)
	workerCount = flag.Int(
		"workers-count",
		5,
		"How many workers to start per controller to process queued events",
	)
	clientGoQPS = flag.Float64(
		"client-go-qps",
		5,
		"Number of queries per second client-go is allowed to make (default 5)",
	)
	clientGoBurst = flag.Int(
		"client-go-burst",
		10,
		"Allowed burst queries for client-go (default 10)",
	)
	runAsLocal = flag.Bool(
		"run-as-local",
		false,
		`When true it enables metac to run by looking up its config file.
		 Metac will no longer be dependent on its CRDs and CRs`,
	)
	metacConfigPath = flag.String(
		"metac-config-path",
		"/etc/config/metac/",
		`Path to metac config file to let metac run as a self contained binary.
		 Needs run-as-local set to true`,
	)
)

// KubeDetails provides kubernetes config & api discovery instance
// based on the kubernetes cluster that gets connected by metac.
//
// These details are helpful when custom controllers import metac &
// in-turn want to invoke kubernetes apis directly without tying
// into metacontrollers' reconciliation process.
//
// This helps the resulting binaries _(read custom controllers)_
// to access copies of kubernetes config as well kubernetes api
// discovery utility.
var KubeDetails *server.KubeDetails

// Start starts this binary
func Start() {
	flag.Parse()

	glog.Infof("Discovery cache refresh interval: %v", *discoveryInterval)
	glog.Infof("API server relist interval i.e. cache flush interval: %v", *informerRelist)
	glog.Infof("Debug http server address: %v", *debugAddr)
	glog.Infof("Run metac locally: %t", *runAsLocal)

	var config *rest.Config
	var err error
	if *clientConfigPath != "" {
		glog.Infof("Using kubeconfig %s", *clientConfigPath)
		config, err = clientcmd.BuildConfigFromFlags("", *clientConfigPath)
	} else if *kubeAPIServerURL != "" {
		glog.Infof("Using kubernetes api server url %s", *kubeAPIServerURL)
		config, err = clientcmd.BuildConfigFromFlags(*kubeAPIServerURL, "")
	} else {
		glog.Info("Using in-cluster kubeconfig")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		glog.Fatal(err)
	}
	config.QPS = float32(*clientGoQPS)
	config.Burst = *clientGoBurst

	// declare the stop server function
	var stopServer func()
	// common server values
	var mserver = &server.Server{
		Config:            config,
		DiscoveryInterval: *discoveryInterval,
		InformerRelist:    *informerRelist,
	}
	// start metac either as config based or CRD based
	if *runAsLocal {
		// run as local implies starting this binary by
		// looking up various MetaController resources as
		// config files
		configServer := &server.ConfigServer{
			Server:     mserver,
			ConfigPath: *metacConfigPath,
		}
		stopServer, err = configServer.Start(*workerCount)
	} else {
		crdServer := &server.CRDServer{
			Server: mserver,
		}
		stopServer, err = crdServer.Start(*workerCount)
	}
	if err != nil {
		glog.Fatal(err)
	}

	// Expose kubernetes information via this global variable.
	// This provides a suitable way for consumers of this library
	// to use the same.
	KubeDetails = mserver.GetKubeDetails()

	exporter, err := prometheus.NewExporter(prometheus.Options{})
	if err != nil {
		glog.Fatalf("Can't create prometheus exporter: %v", err)
	}
	view.RegisterExporter(exporter)

	mux := http.NewServeMux()
	mux.Handle("/metrics", exporter)
	httpServer := &http.Server{
		Addr:    *debugAddr,
		Handler: mux,
	}
	go func() {
		glog.Errorf(
			"Error serving metrics endpoint: %v",
			httpServer.ListenAndServe(),
		)
	}()

	// On SIGTERM, stop all controllers gracefully.
	sigchan := make(chan os.Signal, 2)
	signal.Notify(sigchan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigchan
	glog.Infof("Received %q signal. Shutting down...", sig)

	stopServer()
	httpServer.Shutdown(context.Background())
}
