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

package genericlocal

import (
	"testing"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/test/integration/framework"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// TestLocalSetStatusOnCR will verify if GenericController can be
// used to implement setting of status against the watched
// resource
func TestLocalSetStatusOnCR(t *testing.T) {

	// name of the GenericController
	ctlName := "set-status-on-cr-localgctrl"

	// name of the target namespace
	targetNamespaceName := "my-watch-cr"

	// name of the target resource(s) that are created
	// and are expected to get deleted upon deletion
	// of target namespace
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
			"conditions": []string{
				"GenericController",
				"InlineHookCall",
				"RunFromLocalConfig",
			},
		}
		// there are no attachments to be reconciled
		resp.SkipReconcile = true
		return nil
	}

	// Add this sync hook implementation to inline hook registry
	var inlineHookName = "test/gctl-local-set-status-on-cr"
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

		// We want LocalNerd resource as our watched resource
		generic.WithWatch(
			&v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "test.metac.openebs.io/v1",
					Resource:   "localnerds",
				},
				// We are interested only for our custom resource
				NameSelector: []string{targetResName},
			},
		),
	)

	// start metac that uses above GenericController instance as a config
	stopMetac := f.StartMetacFromGenericControllerConfig(
		func() ([]*v1alpha1.GenericController, error) {
			return []*v1alpha1.GenericController{gctlConfig}, nil
		},
	)
	defer stopMetac()

	// ---------------------------------------------------
	// Create the target namespace i.e. target under test
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
	_, lnClient, _ := f.SetupNamespaceCRDAndItsCR(
		"LocalNerd",
		targetNamespace.GetName(),
		targetResName,
		framework.SetFinalizers([]string{"protect.abc.io", "protect.def.io"}),
	)

	// Need to wait & see if our controller works as expected
	klog.Infof("Will wait to verify LocalNerd status")

	waitCondErr := f.Wait(func() (bool, error) {
		var errs []error

		// -------------------------------------------
		// verify if our custom resources are set with status
		// -------------------------------------------
		lnObj, getLNErr := lnClient.
			Namespace(targetNamespaceName).
			Get(
				targetResName,
				metav1.GetOptions{},
			)
		if getLNErr != nil {
			errs = append(
				errs,
				errors.Wrapf(
					getLNErr,
					"Get LocalNerd %s failed",
					targetResName,
				),
			)
		}

		// verify phase
		phase, _, _ := unstructured.NestedString(
			lnObj.UnstructuredContent(),
			"status",
			"phase",
		)
		if phase != "Active" {
			errs = append(
				errs,
				errors.Errorf("LocalNerd status is not 'Active'"),
			)
		}

		// verify conditions
		conditions, _, _ := unstructured.NestedStringSlice(
			lnObj.UnstructuredContent(),
			"status",
			"conditions",
		)
		if len(conditions) != 3 {
			errs = append(
				errs,
				errors.Errorf("LocalNerd conditions count is not 3"),
			)
		}

		// condition did not pass in case of any errors
		if len(errs) != 0 {
			return false, utilerrors.NewAggregate(errs)
		}

		// condition passed
		return true, nil
	})

	if waitCondErr != nil {
		t.Fatalf(
			"Failed to set LocalNerd status: %v",
			waitCondErr,
		)
	}
	klog.Infof("LocalNerd status was set successfully")
}
