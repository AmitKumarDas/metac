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

package crdmode

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog"

	"openebs.io/metac/controller/generic"
	"openebs.io/metac/test/integration/framework"
	"openebs.io/metac/third_party/kubernetes"
)

// TestDeleteStuckNamespaceViaGctl will verify if GenericController
// can be used to uninstall a namespace that is already marked for
// deletion but is stuck forever due to some dependant resources
// with finalizers.
func TestDeleteStuckNamespaceViaGctl(t *testing.T) {
	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	// NOTE:
	// Keep the namespaces as unique as possible
	namespaceName := "ns-dsnvg"

	// namespace that will be under watch
	targetNamespaceName := "ns-stuck-dsnvg"

	// ------------------------------------------------------------
	// Define the "reconcile logic" for finalize i.e. delete event
	// ------------------------------------------------------------
	finalizeHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if uerr := json.Unmarshal(body, &req); uerr != nil {
			return nil, uerr
		}

		// initialize the hook response
		resp := generic.SyncHookResponse{}

		var isCustomResourceObserved bool
		var keep []*unstructured.Unstructured
		for _, att := range req.Attachments.List() {
			// Check for CRD
			if att.GetKind() == "CustomResourceDefinition" {
				// keep CRD till all its corresponding custom resources
				// are deleted
				keep = append(keep, att)
				continue
			}
			isCustomResourceObserved = true
			// This is a custom resource
			if len(att.GetFinalizers()) == 0 {
				// This is a custom resource with no finalizers.
				// Hence, let this be deleted i.e. don't add to response
				continue
			}
			att.SetFinalizers([]string{})
			keep = append(keep, att)
		}

		if isCustomResourceObserved {
			// If there are no custom resources in attachments then
			// it implies all these custom resources are deleted. We
			// can set the response attachments to nil to delete
			// the CRDs.
			resp.Attachments = append(
				resp.Attachments,
				keep...,
			)
		}
		// Check for presence of attachments in request
		if req.Attachments.IsEmpty() {
			// Mark this finalize hook as completed since all attachments
			// are deleted from cluster
			klog.Infof("Finalize completed")
			resp.Finalized = true
		} else {
			// Keep resyncing the watch since attachments are observed
			// in the cluster
			klog.Infof(
				"Finalize in-progress: req %d: resp %d",
				req.Attachments.Len(),
				len(resp.Attachments),
			)
			resp.ResyncAfterSeconds = 1
		}
		return json.Marshal(resp)
	})

	// Run the testcase here
	//
	// NOTE:
	// 	TestSteps are executed in their defined order
	result, err := f.Test(
		[]framework.TestStep{
			framework.TestStep{
				Name: "create-gctl-namespace",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-target-namespace",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": targetNamespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-crd-as-namespace-scoped",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "stuckdeploys.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "StuckDeploy",
									"listKind": "StuckDeployList",
									"singular": "stuckdeploy",
									"plural":   "stuckdeploys",
									"shortNames": []interface{}{
										"stuckdeploy",
									},
								},
								"versions": []interface{}{
									map[string]interface{}{
										"name":    "v1",
										"served":  true,
										"storage": true,
									},
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-stuckdeploy-resource-with-finalizers",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "StuckDeploy",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "my-deploy",
								"namespace": targetNamespaceName,
								"finalizers": []interface{}{
									"protect.deploy.io",
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "delete-target-namespace",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": targetNamespaceName,
							},
						},
					},
					Replicas: kubernetes.IntPtr(0), // implies delete
				},
			},
			framework.TestStep{
				Name: "assert-target-namespace-is-stuck-in-deletion",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": targetNamespaceName,
							},
							"status": map[string]interface{}{
								"phase": "Terminating",
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-generic-controller",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "GenericController",
							"apiVersion": "metac.openebs.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      "delete-stuck-ns-gctl",
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								"deleteAny": kubernetes.BoolPtr(true),
								"updateAny": kubernetes.BoolPtr(true),
								"watch": map[string]interface{}{
									"apiVersion": "v1",
									"resource":   "namespaces",
									"nameSelector": []interface{}{
										targetNamespaceName,
									},
								},
								"attachments": []interface{}{
									map[string]interface{}{
										"apiVersion": "apiextensions.k8s.io/v1beta1",
										"resource":   "customresourcedefinitions",
										"nameSelector": []interface{}{
											"stuckdeploys.integration.test.io",
										},
									},
									map[string]interface{}{
										"apiVersion": "integration.test.io/v1",
										"resource":   "stuckdeploys",
										"updateStrategy": map[string]interface{}{
											"method": "InPlace",
										},
									},
								},
								"hooks": map[string]interface{}{
									"finalize": map[string]interface{}{
										"webhook": map[string]interface{}{
											"url": kubernetes.StringPtr(finalizeHook.URL),
										},
									},
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-presence-of-stuckdeploys-crd",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CustomResourceDefinition",
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"metadata": map[string]interface{}{
								"name": "stuckdeploys.integration.test.io",
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorNotFound,
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-presence-of-target-namespace",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": targetNamespaceName,
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorNotFound,
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("Test failed: %+v", err)
	}
	if result.Phase == framework.TestStepResultFailed {
		t.Fatalf("Test failed:\n%s", result)
	}
	klog.Infof("Test passed:\n%s", result)
}
