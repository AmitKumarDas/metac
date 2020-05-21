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

package configmode

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/server"
	"openebs.io/metac/test/integration/framework"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// TestSetStatusOnCRThatExistsLater will verify if GenericController can be
// used to implement setting of status against the watched
// resource
func TestSetStatusOnCRThatExistsLater(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode.")
	}
	// name of the GenericController
	ctlName := "lgctl-ssoctel"

	// name of the target namespace
	targetNamespaceName := "ns-ssoctel"

	// name of the target resource
	targetResName := "my-cr"

	f := framework.NewFixture(t)
	defer f.TearDown()

	// ------------------------------------------------------------
	// Define the "reconcile logic" for sync i.e. create/update event
	// ------------------------------------------------------------
	//
	// NOTE:
	// 	This makes use of inline function as hook
	sHook := func(req *generic.SyncHookRequest, resp *generic.SyncHookResponse) error {
		if resp == nil {
			resp = &generic.SyncHookResponse{}
		}
		resp.Status = map[string]interface{}{
			"phase": "Active",
		}
		// there are no attachments to be reconciled
		resp.SkipReconcile = true
		return nil
	}

	// Add this sync hook implementation to inline hook registry
	var inlineHookName = "test/gctl-set-status-on-cr-that-exists-later"
	generic.AddToInlineRegistry(inlineHookName, sHook)

	// ---------------------------------------------------------
	// Define & Apply a GenericController i.e. a Meta Controller
	// ---------------------------------------------------------

	// This is one of the meta controller that is defined as
	// a Kubernetes custom resource. It listens to the resource
	// specified in the watch field and acts against the resources
	// specified in the attachments field.
	gctlConfig := f.CreateGenericControllerAsMetacConfig(
		ctlName,

		// set sync hook
		generic.WithInlinehookSyncFunc(k8s.StringPtr(inlineHookName)),

		// custom resource is the watched resource
		generic.WithWatch(
			&v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "test.metac.openebs.io/v1",
					Resource:   "crthatexistslaters",
				},
				// We are interested only for our custom resource
				NameSelector: []string{targetResName},
			},
		),
	)

	// start metac that uses above GenericController instance as a config
	var mserver = &server.Server{
		Config:            framework.APIServerConfig,
		DiscoveryInterval: 500 * time.Millisecond,
		InformerRelist:    30 * time.Minute,
	}
	metacServer := &server.ConfigServer{
		Server: mserver,
		GenericControllerConfigLoadFn: func() ([]*v1alpha1.GenericController, error) {
			return []*v1alpha1.GenericController{gctlConfig}, nil
		},
		RetryIndefinitelyForStart: k8s.BoolPtr(true), // target under test
	}
	stopMetac, err := metacServer.Start(5)
	if err != nil {
		t.Fatal(err)
	}
	defer stopMetac()

	// we wait for more than a minute & then create the CRD & CR
	time.Sleep(1 * time.Minute)

	// ---------------------------------------------------
	// Create the target namespace
	// ---------------------------------------------------
	//
	// NOTE:
	// 	Targeted CustomResources will be set in this namespace
	targetNamespace, nsCreateErr := f.GetTypedClientset().
		CoreV1().
		Namespaces().
		Create(
			&v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: targetNamespaceName,
				},
			},
		)
	if nsCreateErr != nil {
		t.Fatal(nsCreateErr)
	}

	// ------------------------------------------------------
	// Create target CRD & CR to trigger above generic controller
	// ------------------------------------------------------
	//
	// Create a namespace scoped CoolNerd CRD & CR with finalizers
	_, crClient, _ := f.SetupNamespaceCRDAndItsCR(
		"CRThatExistsLater",
		targetNamespace.GetName(),
		targetResName,
	)

	// Need to wait & see if our controller works as expected
	klog.Infof("Wait to verify CRThatExistsLater status")

	err = f.Wait(func() (bool, error) {
		// -------------------------------------------
		// verify if custom resource is set with status
		// -------------------------------------------
		crObj, err := crClient.
			Namespace(targetNamespaceName).
			Get(
				targetResName,
				metav1.GetOptions{},
			)
		if err != nil {
			return false, errors.Wrapf(
				err,
				"Failed to get CRThatExistsLater %s",
				targetResName,
			)

		}
		// verify phase
		phase, _, _ := unstructured.NestedString(
			crObj.UnstructuredContent(),
			"status",
			"phase",
		)
		if phase != "Active" {
			return false, errors.Errorf(
				"CRThatExistsLater %s status is not 'Active'",
				targetResName,
			)
		}
		// condition passed
		return true, nil
	})

	if err != nil {
		t.Fatalf(
			"Failed to set CRThatExistsLater status: %v",
			err,
		)
	}
	klog.Infof("CRThatExistsLater status was set successfully")
}
