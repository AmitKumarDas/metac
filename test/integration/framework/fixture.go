/*
Copyright 2019 Google Inc.
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

package framework

import (
	"fmt"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metaclientset "openebs.io/metac/client/generated/clientset/versioned"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
)

const (
	// testing involves some amount of wait & retry i.e. polling
	// these variables are tunables used during polling
	defaultWaitTimeout  = 60 * time.Second
	defaultWaitInterval = 200 * time.Millisecond
)

// Fixture is the base structure that various test cases will make use
// to build integration test logic. Individual test logic use this
// to handle respective teardown as well as their common logic.
type Fixture struct {
	t *testing.T

	teardownFuncs []func() error

	// clientsets to invoke CRUD operations using:
	// -- dynamic approach
	dynamicClientset *dynamicclientset.Clientset

	// clientsets to invoke CRUD operations using:
	// -- kubernetes typed approach
	typedClientset kubernetes.Interface

	// crdClient is based on k8s.io/apiextensions-apiserver &
	// is all about invoking CRUD ops against CRDs
	crdClient apiextensionsclientset.ApiextensionsV1beta1Interface

	// meta clientset to invoke CRUD operations against various
	// meta controllers i.e. Metac's custom resources
	metaClientset metaclientset.Interface
}

// NewFixture returns a new instance of Fixture
func NewFixture(t *testing.T) *Fixture {
	// get the config that is created just for the purposes
	// of integration testing of this project
	config := ApiserverConfig()

	crdClient, err := apiextensionsclientset.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	dynamicClientset, err := dynamicclientset.New(config, resourceManager)
	if err != nil {
		t.Fatal(err)
	}
	metaClientset, err := metaclientset.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	typedClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	return &Fixture{
		t:                t,
		dynamicClientset: dynamicClientset,
		typedClientset:   typedClientset,
		crdClient:        crdClient,
		metaClientset:    metaClientset,
	}
}

// GetTypedClientset returns the Kubernetes clientset that is
// used to invoke CRUD operations against Kubernetes native
// resources.
func (f *Fixture) GetTypedClientset() kubernetes.Interface {
	return f.typedClientset
}

// GetCRDClient returns the CRD client that can be used to invoke
// CRUD operations against CRDs itself
func (f *Fixture) GetCRDClient() apiextensionsclientset.ApiextensionsV1beta1Interface {
	return f.crdClient
}

// CreateNamespace creates a namespace that will be deleted
// when this fixture's teardown is invoked.
func (f *Fixture) CreateNamespace(namespace string) *v1.Namespace {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	ns, err := f.typedClientset.CoreV1().Namespaces().Create(ns)
	if err != nil {
		f.t.Fatal(err)
	}

	// add this to teardown that gets executed during cleanup
	f.addToTeardown(func() error {
		_, err := f.typedClientset.CoreV1().Namespaces().Get(ns.GetName(), metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		return f.typedClientset.CoreV1().Namespaces().Delete(ns.GetName(), nil)
	})
	return ns
}

// CreateNamespaceGen creates a namespace with its name prefixed
// with the provided name. This namespace gets deleted when this
// fixture's teardown is invoked
func (f *Fixture) CreateNamespaceGen(name string) *v1.Namespace {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name + "-",
		},
	}
	ns, err := f.typedClientset.CoreV1().Namespaces().Create(ns)
	if err != nil {
		f.t.Fatal(err)
	}

	// add this to teardown that gets executed during cleanup
	f.addToTeardown(func() error {
		_, err := f.typedClientset.CoreV1().Namespaces().Get(ns.GetName(), metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		return f.typedClientset.CoreV1().Namespaces().Delete(ns.GetName(), nil)
	})
	return ns
}

// TearDown cleans up resources created through this instance
// of the test fixture.
func (f *Fixture) TearDown() {
	// cleanup in descending order
	for i := len(f.teardownFuncs) - 1; i >= 0; i-- {
		teardown := f.teardownFuncs[i]
		err := teardown()
		if err != nil && !apierrors.IsNotFound(err) {
			f.t.Logf("Teardown %d failed: %v", i, err)
			// Mark the test as failed, but continue trying to tear down.
			f.t.Fail()
		}
	}
}

// Wait polls the condition until it's true, with a default interval
// and timeout. This is meant for use in integration tests, so frequent
// polling is fine.
//
// The condition function returns a bool indicating whether it is satisfied,
// as well as an error which should be non-nil if and only if the function was
// unable to determine whether or not the condition is satisfied (for example
// if the check involves calling a remote server and the request failed).
//
// If the condition function returns a non-nil error, Wait will log the error
// and continue retrying until the timeout.
func (f *Fixture) Wait(condition func() (bool, error)) error {
	// mark the start time
	start := time.Now()
	for {
		done, err := condition()
		if err == nil && done {
			f.t.Logf("Wait condition succeeded")
			return nil
		}
		if time.Since(start) > defaultWaitTimeout {
			return fmt.Errorf(
				"Wait condition timed out %s: %v", defaultWaitTimeout, err,
			)
		}
		if err != nil {
			// Log error, but keep trying until timeout.
			f.t.Logf("Wait condition failed: Will retry: %v", err)
		} else {
			f.t.Logf("Waiting for condition to succeed: Will retry")
		}
		time.Sleep(defaultWaitInterval)
	}
}

// addToTeardown adds the given teardown func to the list of
// teardown functions
func (f *Fixture) addToTeardown(teardown func() error) {
	f.teardownFuncs = append(f.teardownFuncs, teardown)
}
