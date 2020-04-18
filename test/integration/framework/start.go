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
	"path"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	"openebs.io/metac/server"
)

var apiResourceDiscovery *dynamicdiscovery.APIResourceDiscovery
var apiServerConfig *rest.Config

// manifestDir is the path from the integration test binary
// working dir to the directory containing manifests to
// install Metacontroller.
const manifestDir = "../../../manifests"

// StartCRDBasedMetac sets up kubernetes environment by starting
// kube apiserver & etcd binaries. Once the setup is done the test
// case functions provided as arguments are executed.
func StartCRDBasedMetac(testFns func() int) error {
	klog.V(2).Infof("Will setup k8s")
	cp := &ControlPlane{
		StartTimeout: time.Second * 40,
		StopTimeout:  time.Second * 40,
		// Uncomment below to debug
		//Out: os.Stdout,
		//Err: os.Stderr,
	}
	err := cp.Start()
	if err != nil {
		return err
	}
	klog.V(2).Infof("k8s was setup successfully")

	// Create Metacontroller Namespace.
	err = cp.KubeCtl().Run(
		"apply",
		"-f",
		path.Join(
			manifestDir,
			"metacontroller-namespace.yaml",
		),
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"Can't install metacontroller namespace",
		)
	}

	// Install Metacontroller RBAC.
	err = cp.KubeCtl().Run(
		"apply",
		"-f",
		path.Join(
			manifestDir,
			"metacontroller-rbac.yaml",
		),
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"Can't install metacontroller RBAC",
		)
	}

	// Install Metacontroller CRDs.
	err = cp.KubeCtl().Run(
		"apply",
		"-f",
		path.Join(
			manifestDir,
			"metacontroller.yaml",
		),
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"Can't install metacontroller CRDs",
		)
	}

	// set the config to the global variable
	apiServerConfig, err = cp.RESTClientConfig()
	if err != nil {
		return err
	}

	// In this integration test environment, there are no Nodes,
	// so the metacontroller StatefulSet will not actually run
	// anything. Instead, we start the Metacontroller server
	// locally inside the test binary, since that's part of the
	// code under test.
	metac := &server.CRDServer{
		Server: server.Server{
			Config:            apiServerConfig,
			DiscoveryInterval: 500 * time.Millisecond,
			InformerRelist:    30 * time.Minute,
		},
	}
	stop, err := metac.Start(5)
	if err != nil {
		return errors.Wrapf(
			err,
			"Can't start CRD based metac server",
		)
	}
	defer stop()
	klog.Info("Started CRD based metac server")

	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(
		apiServerConfig,
	)

	// set api resource discovery to global variable
	apiResourceDiscovery = dynamicdiscovery.NewAPIResourceDiscoverer(
		discoveryClient,
	)

	// We don't care about stopping this cleanly since it has no
	// external effects.
	apiResourceDiscovery.Start(500 * time.Millisecond)

	// Run the actual tests now after above setup was done
	// successfully
	//
	// NOTE:
	//	We ignore the exit code & rely on the using t.Fatal
	// statements if individual test cases fail or error out
	// at runtime
	//
	// NOTE:
	//	This always returns an exit code of 1 & hence a single
	// word FAIL is printed at the end
	testFns()

	return nil
}

// StartConfigBasedMetac sets up kubernetes environment by starting
// kube apiserver & etcd binaries. Once the setup is done the test
// case functions provided as arguments are executed.
func StartConfigBasedMetac(testFns func() int) error {
	klog.V(2).Infof("Will setup k8s")
	cp := &ControlPlane{
		StartTimeout: time.Second * 40,
		StopTimeout:  time.Second * 40,
		// Uncomment below to debug
		//Out: os.Stdout,
		//Err: os.Stderr,
	}
	err := cp.Start()
	if err != nil {
		return err
	}
	klog.V(2).Infof("k8s was setup successfully")

	// set the config to the global variable
	apiServerConfig, err = cp.RESTClientConfig()
	if err != nil {
		return err
	}

	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(
		apiServerConfig,
	)

	// set api resource discovery to the global variable
	apiResourceDiscovery = dynamicdiscovery.NewAPIResourceDiscoverer(
		discoveryClient,
	)

	// We don't care about stopping this cleanly since it has no
	// external effects.
	apiResourceDiscovery.Start(500 * time.Millisecond)

	// Run the actual tests now after above setup was done
	// successfully
	if exitCode := testFns(); exitCode != 0 {
		return errors.Errorf(
			"One or more tests failed: Exit code %d",
			exitCode,
		)
	}

	return nil
}
