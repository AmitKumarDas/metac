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
	"k8s.io/apimachinery/pkg/util/json"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/test/integration/framework"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// TestPostCleanUninstall will verify if GenericController can be
// used to implement clean uninstall requirements when a namespace
// is already marked for deletion but is stuck forever due to some
// dependant resources left with finalizers.
//
// This test verifies the usecase when a GenericController is deployed
// to get this stuck namespace deleted successfully.
//
// A clean uninstall implies when a workload specific Namespace
// is removed from kubernetes cluster, the associated CRDs and CRs
// should get removed from this cluster. This should work even in
// the cases where CRs are set with finalizers and the corresponding
// controllers i.e. pods are no longer available due to the deletion
// of this workload namespace.
func TestStuckToUnStuckDelete(t *testing.T) {
	// namespace to setup GenericController
	ctlNSNamePrefix := "gctl-test"

	// name of the GenericController
	ctlName := "stuck-to-unstuck-gctrl"

	// name of the target namespace which is deleted before
	// starting the metacontroller
	targetNamespaceName := "stuckie-ns"

	// name of the target resource(s) that are created
	// and are expected to get deleted upon deletion
	// of target namespace
	targetResName := "my-stuckie"

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

	// setup some random CRDs, all of which are namespace scoped

	// define a namespace scoped CStorPoolClub CRD & CR with finalizers
	cpcCRD, cpcClient, _ := f.SetupNamespaceCRDAndItsCR(
		"CStorPoolClub",
		targetNamespace.GetName(),
		targetResName,
		framework.SetFinalizers([]string{"protect.abc.io", "protect.def.io"}),
	)

	// define a namespace scoped CStorVolumeRest CRD & CR with finalizers
	cvrCRD, cvrClient, _ := f.SetupNamespaceCRDAndItsCR(
		"CStorVolumeRest",
		targetNamespace.GetName(),
		targetResName,
		framework.SetFinalizers([]string{"protect.xyz.io", "protect.ced.io"}),
	)

	// ------------------------------------------------------
	// Delete target namespace before deploying the controller
	// ------------------------------------------------------
	//
	// Above makes the namespace deletion being stuck due to
	// this namespace scoped custom resources' finalizers.
	err = f.GetTypedClientset().CoreV1().Namespaces().Delete(targetNamespace.GetName(), &metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// ------------------------------------------------------------
	// Define the "reconcile logic" for finalize i.e. delete event
	// ------------------------------------------------------------
	//
	// NOTE:
	// 	This gets triggered upon deletion of target CRD
	//
	// NOTE:
	// 	This is a multi process reconciliation strategy:
	//
	// Stage 1: Remove finalizers from custom resources
	// Stage 2: Delete custom resources that dont have finalizers
	// Stage 3: Delete other custom resource definition(s) if there are no more custom resources
	//
	// FUTURE:
	//	One can report these stages via status of the watch object
	fHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if uerr := json.Unmarshal(body, &req); uerr != nil {
			return nil, uerr
		}

		// initialize the hook response
		resp := generic.SyncHookResponse{}

		// this check i.e. deletion timestamp is not required
		// if this hook is set exclusively as a finalize hook
		if req.Watch.GetDeletionTimestamp() != nil {

			var hasAtleastOneCustomResource bool
			for _, attGroup := range req.Attachments {
				for _, att := range attGroup {
					if att == nil {
						// ignore this attachment
						continue
					}

					// copy the attachment from req to a new instance
					//respAtt := att
					respAtt := &unstructured.Unstructured{}

					if att.GetKind() == "CustomResourceDefinition" {
						// keep the CRD attachment till all its corresponding
						// CRs get deleted
						respAtt.SetUnstructuredContent(att.UnstructuredContent())
						resp.Attachments = append(resp.Attachments, respAtt)
						continue
					} else {
						hasAtleastOneCustomResource = true
					}

					if len(att.GetFinalizers()) == 0 {
						// this is a custom resource & does not have any finalizers
						// then let this be deleted i.e. don't add to response
						continue
					}

					// This is a custom resource with finalizers
					// Hence, re-build the attachment with empty finalizers
					respAtt.SetAPIVersion(att.GetAPIVersion())
					respAtt.SetKind(att.GetKind())
					respAtt.SetName(att.GetName())
					respAtt.SetNamespace(att.GetNamespace())
					// Setting finalizers to empty is a must to
					// let this custom resource get deleted
					respAtt.SetFinalizers([]string{})

					resp.Attachments = append(resp.Attachments, respAtt)
				}
			}

			if !hasAtleastOneCustomResource {
				// If there are no custom resources in attachments then
				// it implies all these custom resources are deleted. We
				// can set the response attachments to nil. This will delete
				// the CRDs.
				resp.Attachments = nil
			}

			// keep executing this finalize hook till its request has attachments
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
		}

		t.Logf(
			"Finalize attachments count: Req %d: Resp %d",
			req.Attachments.Len(), len(resp.Attachments),
		)

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

		// enable controller to delete any attachments
		generic.WithDeleteAny(k8s.BoolPtr(true)),

		// enable controller to update any attachments
		generic.WithUpdateAny(k8s.BoolPtr(true)),

		// set finalize' hook
		generic.WithWebhookFinalizeURL(&fHook.URL),

		// We want CStorPoolClub CRD as our watched resource
		generic.WithWatch(
			&v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "apiextensions.k8s.io/v1beta1",
					Resource:   "customresourcedefinitions",
				},
				// We are interested only for one of our custom resource
				NameSelector: []string{cpcCRD.GetName()},
			},
		),

		// We want the CRs & CRDs as our attachments.
		//
		// This is done so as to implement clean uninstall when
		// above watch resource is deleted. A clean uninstall is
		// successful if these declared attachments get deleted
		// when watch i.e. CRD is deleted.
		generic.WithAttachments(
			[]*v1alpha1.GenericControllerAttachment{
				// We want all CPC custom resources as attachments
				&v1alpha1.GenericControllerAttachment{
					GenericControllerResource: v1alpha1.GenericControllerResource{
						ResourceRule: v1alpha1.ResourceRule{
							APIVersion: cpcCRD.Spec.Group + "/" + cpcCRD.Spec.Versions[0].Name,
							Resource:   cpcCRD.Spec.Names.Plural,
						},
					},
					UpdateStrategy: &v1alpha1.GenericControllerAttachmentUpdateStrategy{
						Method: v1alpha1.ChildUpdateInPlace,
					},
				},
				// We want all CVR custom resources as attachments
				&v1alpha1.GenericControllerAttachment{
					GenericControllerResource: v1alpha1.GenericControllerResource{
						ResourceRule: v1alpha1.ResourceRule{
							APIVersion: cvrCRD.Spec.Group + "/" + cvrCRD.Spec.Versions[0].Name,
							Resource:   cvrCRD.Spec.Names.Plural,
						},
					},
					UpdateStrategy: &v1alpha1.GenericControllerAttachmentUpdateStrategy{
						Method: v1alpha1.ChildUpdateInPlace,
					},
				},
				// We want CRDs to be included as attachments
				// We want the other CRD i.e. CStorVolumeRest only
				&v1alpha1.GenericControllerAttachment{
					GenericControllerResource: v1alpha1.GenericControllerResource{
						ResourceRule: v1alpha1.ResourceRule{
							APIVersion: "apiextensions.k8s.io/v1beta1",
							Resource:   "customresourcedefinitions",
						},
						NameSelector: []string{cvrCRD.GetName()},
					},
					UpdateStrategy: &v1alpha1.GenericControllerAttachmentUpdateStrategy{
						Method: v1alpha1.ChildUpdateInPlace,
					},
				},
			},
		),
	)

	// -------------------------------------------------------
	// Wait for the setup to behave similar to production env
	// -------------------------------------------------------
	//
	// Wait till target CRD is assigned with GenericController's
	// finalizer
	//
	// NOTE:
	//	GenericController automatically updates the watch with
	// its own finalizer if it finds a finalize hook in its
	// specifications.
	readyErr := f.Wait(func() (bool, error) {
		cpcCRDWithF, err :=
			f.GetCRDClient().CustomResourceDefinitions().Get(cpcCRD.GetName(), metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, finalizer := range cpcCRDWithF.GetFinalizers() {
			if finalizer == "protect.gctl.metac.openebs.io/"+ctlNS.GetName()+"-"+ctlName {
				return true, nil
			}
		}
		return false, errors.Errorf("CRD %s is not set with gctl finalizer", cpcCRD.GetName())

	})
	if readyErr != nil {
		// we wait till timeout & panic if condition is not met
		t.Fatal(readyErr)
	}

	// ------------------------------------------------------
	// Delete target CRD to trigger above generic controller
	// ------------------------------------------------------
	//
	// In other words, deleting the generic controller's
	// watch will trigger this controller's finalizer
	delErr := f.GetCRDClient().CustomResourceDefinitions().Delete(cpcCRD.GetName(), &metav1.DeleteOptions{})
	if delErr != nil {
		t.Fatal(delErr)
	}

	// Need to wait & see if our controller works as expected
	// Make sure the specified attachments are deleted
	t.Logf("Waiting for deletion of CRs, CRDs & Namespace")

	err = f.Wait(func() (bool, error) {
		var errs []error

		// -------------------------------------------
		// verify if our custom resources are deleted
		// -------------------------------------------
		cpc, cpcGetErr := cpcClient.Namespace(targetNamespaceName).Get(targetResName, metav1.GetOptions{})
		if cpcGetErr != nil && !apierrors.IsNotFound(cpcGetErr) {
			errs = append(
				errs, errors.Wrapf(cpcGetErr, "Get CPC %s failed", targetResName),
			)
		}
		if cpc != nil {
			errs = append(errs, errors.Errorf("CPC %s is not deleted", targetResName))
		}

		cvr, cvrGetErr := cvrClient.Namespace(targetNamespaceName).Get(targetResName, metav1.GetOptions{})
		if cvrGetErr != nil && !apierrors.IsNotFound(cvrGetErr) {
			errs = append(
				errs,
				errors.Wrapf(cvrGetErr, "Get CVR %s failed", targetResName),
			)
		}
		if cvr != nil {
			errs = append(errs, errors.Errorf("CVR %s is not deleted", targetResName))
		}

		// ------------------------------------------
		// verify if our target namespace is deleted
		// ------------------------------------------
		targetNSAgain, targetNSGetErr := f.GetTypedClientset().CoreV1().Namespaces().
			Get(targetNamespace.GetName(), metav1.GetOptions{})
		if targetNSGetErr != nil && !apierrors.IsNotFound(targetNSGetErr) {
			errs = append(errs, targetNSGetErr)
		}

		if targetNSAgain != nil && targetNSAgain.GetDeletionTimestamp() == nil {
			errs = append(
				errs,
				errors.Errorf(
					"Namespace %s is not marked for deletion", targetNSAgain.GetName(),
				),
			)
		}

		// condition did not pass in case of any errors
		if len(errs) != 0 {
			return false, utilerrors.NewAggregate(errs)
		}

		// condition passed
		return true, nil
	})

	if err != nil {
		t.Fatalf("CRs, CRDs & Namespace deletion failed: %v", err)
	}
	t.Logf("CRDs, CRs & namespace were deleted successfully")
}
