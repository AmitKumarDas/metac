/*
Copyright 2019 Google Inc.

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
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/klog"

	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	"openebs.io/metac/server"
)

var resourceManager *dynamicdiscovery.APIResourceManager

const installKubectlMsg = `
Cannot find kubectl, cannot run integration tests

Please download kubectl and ensure it is somewhere in the PATH.
See hack/get-kube-binaries.sh

`

// manifestDir is the path from the integration test binary
// working dir to the directory containing manifests to
// install Metacontroller.
const manifestDir = "../../../manifests"

// getKubectlPath returns a path to a kube-apiserver executable.
func getKubectlPath() (string, error) {
	return exec.LookPath("kubectl")
}

// TestMain starts etcd, kube-apiserver, and metacontroller before
// running tests. This is meant to be executed from within a _test.go
// file's TestMain(m *testing.M) function.
//
// Usage:
// 	In some abc_test.go file in package abc
//
// ```go
//	package abc
//
//	import (
//		"openebs.io/metac/test/integration/framework
//	)
//
// 	func TestMain(m *testing.M) {
//		framework.TestMain(m.Run())
//	}
// ````
func TestMain(tests func() int) {
	if err := testMain(tests); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func testMain(tests func() int) error {
	if _, err := getKubectlPath(); err != nil {
		return errors.New(installKubectlMsg)
	}

	stopEtcd, err := startEtcd()
	if err != nil {
		return errors.Wrapf(err, "Can't start etcd")
	}
	defer stopEtcd()

	stopApiserver, err := startApiserver()
	if err != nil {
		return errors.Wrapf(err, "Can't start kube-apiserver")
	}
	defer stopApiserver()

	klog.Info("Waiting for kube-apiserver to be ready")
	start := time.Now()
	for {
		kubectlErr := execKubectl("version")
		if kubectlErr == nil {
			break
		}
		if time.Since(start) > time.Minute {
			return errors.Wrapf(err, "Timed out for kube-apiserver to be ready")
		}
		time.Sleep(time.Second)
	}
	klog.Info("kube-apiserver is ready")

	// Create Metacontroller Namespace.
	err = execKubectl(
		"apply",
		"-f",
		path.Join(manifestDir, "metacontroller-namespace.yaml"),
	)
	if err != nil {
		return errors.Wrapf(err, "Can't install metacontroller namespace")
	}

	// Install Metacontroller RBAC.
	err = execKubectl(
		"apply",
		"-f",
		path.Join(manifestDir, "metacontroller-rbac.yaml"),
	)
	if err != nil {
		return errors.Wrapf(err, "Can't install metacontroller RBAC")
	}

	// Install Metacontroller CRDs.
	err = execKubectl(
		"apply",
		"-f",
		path.Join(manifestDir, "metacontroller.yaml"),
	)
	if err != nil {
		return errors.Wrapf(err, "Can't install metacontroller CRDs")
	}

	// In this integration test environment, there are no Nodes,
	// so the metacontroller StatefulSet will not actually run
	// anything. Instead, we start the Metacontroller server
	// locally inside the test binary, since that's part of the
	// code under test.
	stopServer, err := server.Start(
		ApiserverConfig(), 500*time.Millisecond, 30*time.Minute,
	)
	if err != nil {
		return errors.Wrapf(err, "Can't start metacontroller server")
	}
	defer stopServer()

	// Periodically refresh discovery to pick up newly-installed
	// resources.
	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(ApiserverConfig())
	resourceManager = dynamicdiscovery.NewAPIResourceManager(discoveryClient)

	// We don't care about stopping this cleanly since it has no
	// external effects.
	resourceManager.Start(500 * time.Millisecond)

	// Now run the actual tests
	if exitCode := tests(); exitCode != 0 {
		return errors.Errorf("One or more tests failed: Exit code %d", exitCode)
	}
	return nil
}

func execKubectl(args ...string) error {
	execPath, err := exec.LookPath("kubectl")
	if err != nil {
		return errors.Wrapf(err, "Can't exec kubectl")
	}

	cmdline := append([]string{"--server", ApiserverURL()}, args...)
	cmd := exec.Command(execPath, cmdline...)
	return cmd.Run()
}
