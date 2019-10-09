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
	"k8s.io/apimachinery/pkg/util/json"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/test/integration/framework"
)

// TestApplyDeleteCRD will verify if GenericController can be
// used to implement apply & delete of CustomResourceDefinition.
//
// This function will try to get a CRD installed when a target namespace
// gets installed. GenericController should also automatically uninstall
// this CRD when this target namespace is deleted.
func TestApplyDeleteCRD(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TestApplyDeleteCRD in short mode")
	}

	// namespace to setup GenericController
	ctlNSNamePrefix := "gctl-test"

	// name of the GenericController
	ctlName := "install-un-crd-ctrl"

	// name of the target namespace which is watched by GenericController
	targetNSName := "amitd"

	// name of the target CRD which is reconciled by GenericController
	targetCRDName := "storages.dao.amitd.io"

	f := framework.NewFixture(t)
	defer f.TearDown()

	// create namespace to setup GenericController resources
	ctlNS := f.CreateNamespaceGen(ctlNSNamePrefix)

	// -------------------------------------------------------------------------
	// Define the "reconcile logic" for sync i.e. create/update events of watch
	// -------------------------------------------------------------------------
	//
	// NOTE:
	// 	Sync ensures creation of target CRD via attachments
	sHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if uerr := json.Unmarshal(body, &req); uerr != nil {
			return nil, uerr
		}

		// initialize the hook response
		resp := generic.SyncHookResponse{}

		// we desire this CRD object
		crd := framework.BuildUnstructuredObjFromJSON(
			"apiextensions.k8s.io/v1beta1",
			"CustomResourceDefinition",
			targetCRDName,
			`{
				"spec": {
					"group": "dao.amitd.io",
					"version": "v1alpha1",
					"scope": "Namespaced",
					"names": {
						"plural": "storages",
						"singular": "storage",
						"kind": "Storage",
						"shortNames": ["stor"]
					},
					"additionalPrinterColumns": [
						{
							"JSONPath": ".spec.capacity",
							"name": "Capacity",
							"description": "Capacity of the storage",
							"type": "string"
						},
						{
							"JSONPath": ".spec.nodeName",
							"name": "NodeName",
							"description": "Node where the storage gets attached",
							"type": "string"
						},
						{
							"JSONPath": ".status.phase",
							"name": "Status",
							"description": "Identifies the current status of the storage",
							"type": "string"
						}
					]
				}
			}`,
		)

		// add CRD to attachments to let GenericController
		// sync i.e. create
		resp.Attachments = append(resp.Attachments, crd)

		return json.Marshal(resp)
	})

	// ---------------------------------------------------------------------
	// Define the "reconcile logic" for finalize i.e. delete event of watch
	// ---------------------------------------------------------------------
	//
	// NOTE:
	// 	Finalize ensures deletion of target CRD via attachments
	fHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if uerr := json.Unmarshal(body, &req); uerr != nil {
			return nil, uerr
		}

		// initialize the hook response
		resp := generic.SyncHookResponse{}

		// set attachments to nil to let GenericController
		// finalize i.e. delete CRD
		resp.Attachments = nil

		// finalize hook should be executed till its request
		// has attachments
		if req.Attachments.IsEmpty() {
			// since all attachments are deleted from cluster
			// indicate GenericController to mark completion
			// of finalize hook
			resp.Finalized = true
		} else {
			// if there are still attachments seen in the request
			// keep resyncing the watch
			resp.ResyncAfterSeconds = 2
		}

		t.Logf("Finalize: Req.Attachments.Len=%d", req.Attachments.Len())

		return json.Marshal(resp)
	})

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

		// set 'sync' as well as 'finalize' hooks
		generic.WithWebhookSyncURL(&sHook.URL),
		generic.WithWebhookFinalizeURL(&fHook.URL),

		// Namespace is the watched resource
		generic.WithWatch(
			&v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "namespaces",
				},
				// We are interested only for the target namespace only
				NameSelector: []string{targetNSName},
			},
		),

		// CRDs are the attachments
		//
		// This is done so as to implement create & delete of CRD
		// when above watch resource i.e. namespce is created & deleted.
		generic.WithAttachments(
			[]*v1alpha1.GenericControllerAttachment{
				// We want the target CRD only i.e. storages.dao.amitd.io
				&v1alpha1.GenericControllerAttachment{
					GenericControllerResource: v1alpha1.GenericControllerResource{
						ResourceRule: v1alpha1.ResourceRule{
							APIVersion: "apiextensions.k8s.io/v1beta1",
							Resource:   "customresourcedefinitions",
						},
						NameSelector: []string{targetCRDName},
					},
				},
			},
		),
	)

	var err error

	// ---------------------------------------------------
	// Create the target namespace i.e. target under test
	// ---------------------------------------------------
	//
	// NOTE:
	// 	This triggers reconciliation
	_, err = f.GetTypedClientset().CoreV1().Namespaces().Create(
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNSName,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Need to wait & see if our controller works as expected
	// Make sure the specified attachments i.e. CRD is created
	t.Logf("Wait for creation of CRD %s", targetCRDName)

	err = f.Wait(func() (bool, error) {
		var getErr error

		// ------------------------------------------------
		// verify if target CRD is created i.e. reconciled
		// ------------------------------------------------

		crdObj, getErr :=
			f.GetCRDClient().CustomResourceDefinitions().Get(targetCRDName, metav1.GetOptions{})

		if getErr != nil {
			return false, getErr
		}

		if crdObj == nil {
			return false, errors.Errorf("CRD %s is not created", targetCRDName)
		}

		// condition passed
		return true, nil
	})

	if err != nil {
		t.Fatalf("CRD %s wasn't created: %v", targetCRDName, err)
	}

	// ------------------------------------------------------
	// Trigger reconcile again by deleting the target namespace
	// ------------------------------------------------------

	err =
		f.GetTypedClientset().CoreV1().Namespaces().Delete(targetNSName, &metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Need to wait & see if our controller works as expected
	// Make sure the specified attachments i.e. CRD is deleted
	t.Logf("Wait for deletion of CRD %s", targetCRDName)

	err = f.Wait(func() (bool, error) {
		var getErr error

		// ------------------------------------------------
		// verify if target CRD is deleted i.e. reconciled
		// ------------------------------------------------

		crdObj, getErr :=
			f.GetCRDClient().CustomResourceDefinitions().Get(targetCRDName, metav1.GetOptions{})

		if getErr != nil && !apierrors.IsNotFound(getErr) {
			return false, getErr
		}

		if crdObj != nil && crdObj.GetDeletionTimestamp() == nil {
			return false,
				errors.Errorf("CRD %s is not marked for deletion", targetCRDName)
		}

		// condition passed
		return true, nil
	})

	if err != nil {
		t.Fatalf("CRD %s wasn't deleted: %v", targetCRDName, err)
	}

	t.Logf("Test 'Install Uninstall CRD' passed")
}
