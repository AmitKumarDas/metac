/*
Copyright 2017 Google Inc.
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

package main

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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"openebs.io/metac/server"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
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
	clientConfigPath = flag.String(
		"client-config-path",
		"",
		`Path to kubeconfig file (same format as used by kubectl); 
		if not specified, uses in-cluster config`,
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
		`When true enables metac to run by looking up its config file;
		 Metac will no longer be dependent on its CRDs and CRs`,
	)
	metacConfigPath = flag.String(
		"metac-config-path",
		"/etc/config/metac/",
		`Path to metac config file to let metac run as a self contained binary;
		 Needs run-as-local set to true`,
	)
)

func main() {
	flag.Parse()

	glog.Infof("Discovery cache refresh interval: %v", *discoveryInterval)
	glog.Infof("API server relist interval i.e. cache flush interval: %v", *informerRelist)
	glog.Infof("Debug http server address: %v", *debugAddr)
	glog.Infof("Run metac locally: %t", *runAsLocal)

	var config *rest.Config
	var err error
	if *clientConfigPath != "" {
		glog.Infof("Using current context from kubeconfig file: %v", *clientConfigPath)
		config, err = clientcmd.BuildConfigFromFlags("", *clientConfigPath)
	} else {
		glog.Info("No kubeconfig file specified: Trying in-cluster auto-config")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		glog.Fatal(err)
	}
	config.QPS = float32(*clientGoQPS)
	config.Burst = *clientGoBurst

	var stopServer func()
	var mserver = server.Server{
		Config:            config,
		DiscoveryInterval: *discoveryInterval,
		InformerRelist:    *informerRelist,
	}
	// start metac either as config based or CRD based
	if *runAsLocal {
		localServer := &server.ConfigBasedServer{
			Server:          mserver,
			MetacConfigPath: *metacConfigPath,
		}
		stopServer, err = localServer.Start(*workerCount)
	} else {
		crdServer := &server.CRDBasedServer{Server: mserver}
		stopServer, err = crdServer.Start(*workerCount)
	}

	if err != nil {
		glog.Fatal(err)
	}

	exporter, err := prometheus.NewExporter(prometheus.Options{})
	if err != nil {
		glog.Fatalf("Can't create prometheus exporter: %v", err)
	}
	view.RegisterExporter(exporter)

	mux := http.NewServeMux()
	mux.Handle("/metrics", exporter)
	srv := &http.Server{
		Addr:    *debugAddr,
		Handler: mux,
	}
	go func() {
		glog.Errorf("Error serving debug endpoint: %v", srv.ListenAndServe())
	}()

	// On SIGTERM, stop all controllers gracefully.
	sigchan := make(chan os.Signal, 2)
	signal.Notify(sigchan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigchan
	glog.Infof("Received %q signal. Shutting down...", sig)

	stopServer()
	srv.Shutdown(context.Background())
}
