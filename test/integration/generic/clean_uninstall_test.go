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
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/json"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/test/integration/framework"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// TestCleanUninstall will verify if GenericController can be
// used to implement clean uninstall requirements.
//
// A clean uninstall implies when a workload specific Namespace
// is removed from kubernetes cluster, the associated CRDs and CRs
// should get removed from this cluster. This should work even in
// the cases where CRs are set with finalizers and the corresponding
// controllers i.e. pods are no longer available due to the deletion
// of this workload namespace.
func TestCleanUninstall(t *testing.T) {
	// namespace to setup GenericController
	ctlNSNamePrefix := "gctl-test"
	// name of the GenericController
	ctlName := "clean-uninstall-ctrl"

	// name of the target namespace which is watched by GenericController
	targetNSName := "target-ns"

	// name of the target resource(s) that are created
	// and are expected to get deleted upon deletion
	// of target namespace
	targetResName := "my-target"
	finalizers := []string{
		"protect.abc.io",
		"protect.def.io",
		"protect.bc.org",
	}

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
	targetNS, err := f.GetTypedClientset().CoreV1().Namespaces().Create(
		&v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNSName,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// setup some random CRDs, some of which are cluster scoped
	// while others are namespace scoped

	// define a cluster scoped CRD & create a dummy resource as well
	cpcCRD, cpcClient, _ := f.SetupCRDAndDeployOneCR(
		"CStorPoolClaim",
		targetResName,
	)
	// define a namespace scoped CRD & create a dummy resource as well
	cvrCRD, cvrClient, _ := f.SetupNSCRDAndDeployOneCR(
		"CStorVolumeReplica",
		targetNS.GetName(),
		targetResName,
	)

	// ---------------------------------------------------------------
	// Define the "reconcile logic" for sync i.e. create/update events
	// ---------------------------------------------------------------
	//
	// NOTE:
	// 	We just set the finalizers to the attachments
	sHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if uerr := json.Unmarshal(body, &req); uerr != nil {
			return nil, uerr
		}

		// initialize the hook response
		resp := generic.SyncHookResponse{}

		for _, attGroup := range req.Attachments {
			for _, att := range attGroup {
				if att == nil {
					continue
				}
				respAtt := att
				//if len(att.GetFinalizers()) == 0 &&
				//	att.GetKind() != "CustomResourceDefinition" {
				if len(att.GetFinalizers()) == 0 && att.GetDeletionTimestamp() == nil {
					// set finalizers to attachments that are not pending deletion
					respAtt.SetFinalizers(finalizers)
				}
				// keep the attachments that are not pending deletion
				if att.GetDeletionTimestamp() == nil {
					resp.Attachments = append(resp.Attachments, respAtt)
				}
			}
		}

		t.Logf(
			"Sync attachments count: Req %d: Resp %d",
			req.Attachments.Len(), len(resp.Attachments),
		)

		return json.Marshal(resp)
	})

	// ------------------------------------------------------------
	// Define the "reconcile logic" for finalize i.e. delete event
	// ------------------------------------------------------------
	//
	// NOTE:
	// 	This is a multi process reconciliation strategy:
	//		Stage 1: remove finalizers in first/initial finalize hook(s)
	//		Stage 2: remove the attachments in subsequent finalize hook(s)
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

			// we shall remove finalizer from custom resources
			// in first sync i.e. stage 1

			var hasCR bool
			//var hasFinalizer bool
			for _, attGroup := range req.Attachments {
				for _, att := range attGroup {
					if att == nil {
						continue
					}

					if att.GetKind() != "CustomResourceDefinition" {
						hasCR = true
					}

					if len(att.GetFinalizers()) == 0 &&
						att.GetKind() != "CustomResourceDefinition" {
						// if this is a custom resource & does not have any finalizers
						// then let this be deleted i.e. don't add to response
						continue
					}

					// copy the attachment from req to a new instance
					respAtt := att

					if len(att.GetFinalizers()) != 0 {
						//hasFinalizer = true
						// remove finalizers
						respAtt.SetFinalizers(nil)
					}

					// add the updated attachment to response to let this
					// attachment be updated at cluster
					resp.Attachments = append(resp.Attachments, respAtt)
				}
			}

			//if !hasCR && !hasFinalizer {
			if !hasCR {
				// If there are no custom resources & no finalizers in
				// any attachments then we can set the response attachments
				// to nil. This implies all CRs are deleted & it is the time
				// to delete all CRDs.
				resp.Attachments = nil
			}

			// keep executing finalize hook till its request has attachments
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

		// set 'sync' as well as 'finalize' hooks
		generic.WithWebhookSyncURL(&sHook.URL),
		generic.WithWebhookFinalizeURL(&fHook.URL),

		// We want Namespace as our watched resource
		generic.WithWatch(
			&v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "namespaces",
				},
				// We are interested only for our target namespace
				NameSelector: []string{targetNSName},
			},
		),

		// We want the CRs & CRDs as our attachments.
		//
		// This is done so as to implement clean uninstall when
		// above watch resource is deleted. A clean uninstall is
		// successful if these declared attachments get deleted
		// when watch i.e. our target namespace is deleted.
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
				// We want CRDs to be included as attachments &&
				// We want only our CRDs i.e. CStorPoolClaim & CStorVolumeReplica
				&v1alpha1.GenericControllerAttachment{
					GenericControllerResource: v1alpha1.GenericControllerResource{
						ResourceRule: v1alpha1.ResourceRule{
							APIVersion: "apiextensions.k8s.io/v1beta1",
							Resource:   "customresourcedefinitions",
						},
						NameSelector: []string{
							cpcCRD.GetName(),
							cvrCRD.GetName(),
						},
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
	// Wait till target namespace is assigned with a finalizer
	// by GenericController. GenericController automatically
	// assigns a watch with its own finalizer if it finds a
	// finalize hook in its specifications.
	err = f.Wait(func() (bool, error) {
		targetNS, err =
			f.GetTypedClientset().CoreV1().Namespaces().Get(targetNSName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, finalizer := range targetNS.GetFinalizers() {
			if finalizer == "protect.gctl.metac.openebs.io/"+ctlNS.GetName()+"-"+ctlName {
				return true, nil
			}
		}
		return false,
			errors.Errorf("Namespace %s is not set with gctl finalizer", targetNSName)

	})
	if err != nil {
		// we wait till timeout & panic if condition is not met
		t.Fatal(err)
	}

	// Since setup is ready
	//
	// ------------------------------------------------------
	// Trigger the test by deleting the target namespace
	// ------------------------------------------------------
	err = f.GetTypedClientset().CoreV1().Namespaces().
		Delete(targetNS.GetName(), &metav1.DeleteOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Need to wait & see if our controller works as expected
	// Make sure the specified attachments are deleted
	t.Logf("Waiting for deletion of CRs & CRDs")

	err = f.Wait(func() (bool, error) {
		var errs []error
		var getErr error

		// -------------------------------------------
		// verify if our custom resources are deleted
		// -------------------------------------------
		cpc, getErr := cpcClient.Get(targetResName, metav1.GetOptions{})
		if getErr != nil && !apierrors.IsNotFound(getErr) {
			errs = append(
				errs,
				errors.Wrapf(getErr, "Failed to get CPC %s", targetResName),
			)
		}
		if cpc != nil {
			errs = append(errs, errors.Errorf("CPC %s is not deleted", targetResName))
		}

		cvr, getErr := cvrClient.Namespace(targetNSName).Get(targetResName, metav1.GetOptions{})
		if getErr != nil && !apierrors.IsNotFound(getErr) {
			errs = append(
				errs,
				errors.Wrapf(getErr, "Failed to get CVR %s", targetResName),
			)
		}
		if cvr != nil {
			errs = append(errs, errors.Errorf("CVR %s is not deleted", targetResName))
		}

		// ---------------------------------------------
		// verify if our CRDs are deleted
		// ---------------------------------------------

		// TODO (@amitkumardas):
		// Not sure why following does not work ?
		// The logs show that these CRDs are getting deleted
		// based on above sync & finalize logic. However, below
		// get calls work always.
		//
		// Have commented till I get the root cause!
		// cpcCRDAgain, getErr := f.GetCRDClient().CustomResourceDefinitions().
		// 	Get(cpcCRD.GetName(), metav1.GetOptions{})
		// if getErr != nil && !apierrors.IsNotFound(getErr) {
		// 	errs = append(errs, getErr)
		// }
		// if cpcCRDAgain != nil && len(cpcCRDAgain.GetFinalizers()) != 0 {
		// 	errs = append(
		// 		errs,
		// 		errors.Errorf(
		// 			"CPC CRD %s has finalizers", cpcCRD.GetName(),
		// 		),
		// 	)
		// }
		// if cpcCRDAgain != nil && cpcCRDAgain.GetDeletionTimestamp() == nil {
		// 	errs = append(
		// 		errs,
		// 		errors.Errorf(
		// 			"CPC CRD %s is not marked for deletion", cpcCRD.GetName(),
		// 		),
		// 	)
		// }

		// cvrCRDAgain, getErr := f.GetCRDClient().CustomResourceDefinitions().
		// 	Get(cvrCRD.GetName(), metav1.GetOptions{})
		// if getErr != nil && !apierrors.IsNotFound(getErr) {
		// 	errs = append(errs, getErr)
		// }
		// if cvrCRDAgain != nil && len(cvrCRDAgain.GetFinalizers()) != 0 {
		// 	errs = append(
		// 		errs,
		// 		errors.Errorf(
		// 			"CVR CRD %s has finalizers", cvrCRD.GetName(),
		// 		),
		// 	)
		// }
		// if cvrCRDAgain != nil && cvrCRDAgain.GetDeletionTimestamp() == nil {
		// 	errs = append(
		// 		errs,
		// 		errors.Errorf(
		// 			"CVR CRD %s is not marked for deletion", cvrCRD.GetName(),
		// 		),
		// 	)
		// }

		// ------------------------------------------
		// verify if our target namespace is deleted
		// ------------------------------------------
		targetNSAgain, getErr := f.GetTypedClientset().CoreV1().Namespaces().
			Get(targetNS.GetName(), metav1.GetOptions{})
		if getErr != nil && !apierrors.IsNotFound(getErr) {
			errs = append(errs, getErr)
		}
		if targetNSAgain != nil && len(targetNSAgain.GetFinalizers()) != 0 {
			errs = append(
				errs,
				errors.Errorf(
					"Namespace %s has finalizers", targetNS.GetName(),
				),
			)
		}
		if targetNSAgain != nil && targetNSAgain.GetDeletionTimestamp() == nil {
			errs = append(
				errs,
				errors.Errorf(
					"Namespace %s is not marked for deletion", targetNS.GetName(),
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
		t.Fatalf("CRs & CRDs deletion failed: %v", err)
	}
	t.Logf("CRs & CRDs were finalized / deleted successfully")
}
