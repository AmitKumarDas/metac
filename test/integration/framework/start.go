/*
Copyright 2020 The MayaData Authors.

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

package framework

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	"openebs.io/metac/server"
	"openebs.io/metac/test/integration/framework/internal"
)

// These global variables are used in test functions to
// invoke various kubernetes APIs
var apiDiscovery *dynamicdiscovery.APIResourceDiscovery
var apiServerConfig *rest.Config

// manifestDir is the path from the integration test binary
// working dir to the directory containing metac manifests.
// These manifests needs to be applied against the kubernetes
// cluster before using metac.
const manifestDir = "../../../manifests"

// StartCRDBasedMetacBinary sets up a kubernetes environment
// by starting kube apiserver & etcd binaries. Once the setup
// is up & running; metac artifacts are applied against this
// cluster. Finally the test functions provided as arguments
// are executed.
func StartCRDBasedMetacBinary(testFns func() int) error {
	klog.V(2).Infof("Will setup k8s")
	cp := &ControlPlane{
		StartTimeout: time.Second * 60,
		StopTimeout:  time.Second * 60,
		// Uncomment below to debug
		//Out: os.Stdout,
		//Err: os.Stderr,
	}
	err := cp.Start()
	if err != nil {
		return err
	}
	defer cp.Stop()
	klog.V(2).Infof("k8s was setup successfully")

	klog.V(2).Infof("Will apply metac artifacts")
	err = cp.KubeCtl().Apply(ApplyConfig{
		Path: manifestDir,
		YAMLFiles: []string{
			"metacontroller-namespace.yaml",
			"metacontroller-rbac.yaml",
			"metacontroller.yaml",
		},
	})
	if err != nil {
		return err
	}
	klog.V(2).Infof("Metac artifacts were applied successfully")

	klog.V(2).Infof("Will start metac binary")
	metacBinPath := internal.BinPathFinder("metac")
	cmd := NewCommand(CommandConfig{
		Err: os.Stderr,
		Out: os.Stdout,
	})
	var allArgs []string
	allArgs = append(
		allArgs,
		flag.Args()...,
	)
	allArgs = append(
		allArgs,
		fmt.Sprintf(
			"-kube-apiserver-url=%s",
			cp.APIURL().String(),
		),
	)
	stop, err := cmd.Start(metacBinPath, allArgs...)
	if err != nil {
		return err
	}
	defer stop()
	klog.V(2).Infof("Metac binary was started successfully")

	// set the config to this global variable which is in turn
	// used by the test functions
	apiServerConfig, err = cp.GetRESTClientConfig()
	if err != nil {
		return err
	}

	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(
		apiServerConfig,
	)

	// set api resource discovery to this global variable which
	// is in turn used by the test functions
	apiDiscovery = dynamicdiscovery.NewAPIResourceDiscoverer(
		discoveryClient,
	)

	// We don't care about stopping this cleanly since it has no
	// external effects.
	apiDiscovery.Start(500 * time.Millisecond)

	// Run the actual tests now since setup is up & running
	if exitCode := testFns(); exitCode != 0 {
		return errors.Errorf(
			"One or more tests failed: Exit code %d",
			exitCode,
		)
	}
	return nil
}

// StartCRDBasedMetacServer sets up a kubernetes environment
// by starting kube apiserver & etcd binaries. Once the setup
// is up & running; metac artifacts are applied against this
// cluster. Finally the test functions provided as arguments
// are executed.
func StartCRDBasedMetacServer(testFns func() int) error {
	klog.V(2).Infof("Will setup k8s")
	cp := &ControlPlane{
		StartTimeout: time.Second * 60,
		StopTimeout:  time.Second * 60,
		// Uncomment below to debug
		//Out: os.Stdout,
		//Err: os.Stderr,
	}
	err := cp.Start()
	if err != nil {
		return err
	}
	defer cp.Stop()
	klog.V(2).Infof("k8s was setup successfully")

	err = cp.KubeCtl().Apply(ApplyConfig{
		Path: manifestDir,
		YAMLFiles: []string{
			"metacontroller-namespace.yaml",
			"metacontroller-rbac.yaml",
			"metacontroller.yaml",
		},
	})
	if err != nil {
		return err
	}

	// set the config to this global variable which
	// is in turn used by the test functions
	apiServerConfig, err = cp.GetRESTClientConfig()
	if err != nil {
		return err
	}

	// In this integration test environment, there are no Nodes,
	// so the metacontroller StatefulSet will not actually run
	// anything. Instead, we start the Metacontroller server
	// locally inside the test binary, since that's part of the
	// code under test.
	metac := &server.CRDServer{
		Server: &server.Server{
			Config:            apiServerConfig,
			DiscoveryInterval: 500 * time.Millisecond,
			InformerRelist:    30 * time.Minute,
		},
	}
	// Start with a single worker
	// One worker should be sufficient for Integration Tests
	stop, err := metac.Start(1)
	if err != nil {
		return errors.Wrapf(
			err,
			"Can't start CRD based metac server",
		)
	}
	defer stop()
	klog.V(2).Info("Started CRD based metac server")

	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(
		apiServerConfig,
	)

	// set api resource discovery to this global variable which
	// is in turn used by the test functions
	apiDiscovery = dynamicdiscovery.NewAPIResourceDiscoverer(
		discoveryClient,
	)

	// We don't care about stopping this cleanly since it has no
	// external effects.
	apiDiscovery.Start(500 * time.Millisecond)

	// Run the actual tests now since setup is up & running
	// successfully
	if exitCode := testFns(); exitCode != 0 {
		return errors.Errorf(
			"One or more tests failed: Exit code %d",
			exitCode,
		)
	}

	return nil
}

// StartConfigBasedMetacServer sets up kubernetes environment by
// starting kube apiserver & etcd binaries. Once the setup is up
// & running; test functions provided as arguments are executed.
func StartConfigBasedMetacServer(testFns func() int) error {
	klog.V(2).Infof("Will setup k8s")
	cp := &ControlPlane{
		StartTimeout: time.Second * 60,
		StopTimeout:  time.Second * 60,

		// Uncomment below to debug setup issues
		//Out: os.Stdout,
		//Err: os.Stderr,
	}
	err := cp.Start()
	if err != nil {
		return err
	}
	defer cp.Stop()
	klog.V(2).Infof("k8s was setup successfully")

	// set the config to this global variable which
	// is in turn used by the test functions
	apiServerConfig, err = cp.GetRESTClientConfig()
	if err != nil {
		return err
	}

	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(
		apiServerConfig,
	)

	// set api resource discovery to this global variable which
	// is in turn used by the test functions
	apiDiscovery = dynamicdiscovery.NewAPIResourceDiscoverer(
		discoveryClient,
	)

	// We don't care about stopping this cleanly since it has no
	// external effects.
	apiDiscovery.Start(500 * time.Millisecond)

	// Run the actual tests now since setup is up & running
	// successfully
	if exitCode := testFns(); exitCode != 0 {
		return errors.Errorf(
			"One or more tests failed: Exit code %d",
			exitCode,
		)
	}
	return nil
}
