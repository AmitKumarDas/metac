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

package generic

import (
	"testing"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/test/integration/framework"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// TestSetStatusOnCR will verify if GenericController can be
// used to implement setting of status against the watched
// resource
func TestSetStatusOnCR(t *testing.T) {
	// namespace to setup GenericController
	ctlNSNamePrefix := "gctl-test"

	// name of the GenericController
	ctlName := "set-watch-status-gctrl"

	// name of the target namespace which is deleted before
	// starting the metacontroller
	targetNamespaceName := "watch-status-ns"

	// name of the target resource(s) that are created
	// and are expected to get deleted upon deletion
	// of target namespace
	targetResName := "my-watch-stat"

	f := framework.NewFixture(t)
	defer f.TearDown()

	// create namespace to setup GenericController resources
	ctlNS := f.CreateNamespaceGen(ctlNSNamePrefix)

	var err error

	// ---------------------------------------------------
	// Create the target namespace i.e. target under test
	// ---------------------------------------------------
	//
	// NOTE:
	// 	Targeted CustomResources will be set in this namespace
	targetNamespace, err := f.GetTypedClientset().CoreV1().Namespaces().Create(
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNamespaceName,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// ------------------------------------------------------------
	// Define the "reconcile logic" for sync i.e. create/update event
	// ------------------------------------------------------------
	//
	// NOTE:
	// 	This makes use of inline function as hook
	sHook := func(req *generic.SyncHookRequest, resp *generic.SyncHookResponse) error {
		if req == nil || req.Watch == nil {
			t.Logf("Request does not have watch")
			return nil
		}

		t.Logf("Request has watch: %v", req.Watch.GetName())
		if resp == nil {
			resp = &generic.SyncHookResponse{}
		}
		resp.Status = map[string]interface{}{
			"phase": "Active",
			"conditions": []string{
				"GenericController",
				"InlineHookCall",
			},
		}

		t.Logf("Sending response status %v", resp.Status)
		return nil
	}

	// Add this sync hook implementation to inline hook registry
	var testWatchStatusFuncName = "test/watch-status"
	generic.AddToInlineRegistry(testWatchStatusFuncName, sHook)

	// ---------------------------------------------------------
	// Define & Apply a GenericController i.e. a Meta Controller
	// ---------------------------------------------------------

	// This is one of the meta controller that is defined as
	// a Kubernetes custom resource. It listens to the resource
	// specified in the watch field and acts against the resources
	// specified in the attachments field.
	f.CreateGenericController(
		ctlName,
		ctlNS.Name,

		// set sync hook
		generic.WithInlinehookSyncFunc(k8s.StringPtr(testWatchStatusFuncName)),

		// We want CoolNerd resource as our watched resource
		generic.WithWatch(
			&v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "test.metac.openebs.io/v1",
					Resource:   "coolnerds",
				},
				// We are interested only for our custom resource
				NameSelector: []string{targetResName},
			},
		),
	)

	// ------------------------------------------------------
	// Create target CRD & CR to trigger above generic controller
	// ------------------------------------------------------
	//
	// Create a namespace scoped CoolNerd CRD & CR with finalizers
	_, cnClient, _ := f.SetupNamespaceCRDAndItsCR(
		"CoolNerd",
		targetNamespace.GetName(),
		targetResName,
		framework.SetFinalizers([]string{"protect.abc.io", "protect.def.io"}),
	)

	// Need to wait & see if our controller works as expected
	t.Logf("Waiting for verification of CoolNerd resource status")

	err = f.Wait(func() (bool, error) {
		var errs []error

		// -------------------------------------------
		// verify if our custom resources are deleted
		// -------------------------------------------
		cnObj, cpcGetErr := cnClient.Namespace(targetNamespaceName).Get(targetResName, metav1.GetOptions{})
		if cpcGetErr != nil && !apierrors.IsNotFound(cpcGetErr) {
			errs = append(
				errs, errors.Wrapf(cpcGetErr, "Get CoolNerd %s failed", targetResName),
			)
		}
		phase, _, _ :=
			unstructured.NestedString(cnObj.UnstructuredContent(), "status", "phase")
		if cnObj != nil && phase != "Active" {
			errs = append(errs, errors.Errorf("CoolNerd status is not 'Active'"))
		}
		conditions, _, _ :=
			unstructured.NestedStringSlice(cnObj.UnstructuredContent(), "status", "conditions")
		if cnObj != nil && len(conditions) != 2 {
			errs = append(errs, errors.Errorf("CoolNerd conditions count is not 2"))
		}

		// condition did not pass in case of any errors
		if len(errs) != 0 {
			return false, utilerrors.NewAggregate(errs)
		}

		// condition passed
		return true, nil
	})

	if err != nil {
		t.Fatalf("Setting CoolNerd resource status failed: %v", err)
	}
	t.Logf("Setting CoolNerd resource status was successful")
}
