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

	"openebs.io/metac/controller/decorator"
	"openebs.io/metac/test/integration/framework"
	"openebs.io/metac/third_party/kubernetes"
)

// TestCustomResourceOwnsAPodViaDctl will verify if
// DecoratorController can be used to create & delete
// pod based on presence & absence of a custom resource
func TestCustomResourceOwnsAPodViaDctl(t *testing.T) {

	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	namespaceName := "ns-cropvdctl"

	// ------------------------------------------------------------
	// Define "reconcile logic" for parent
	// ------------------------------------------------------------
	//
	// NOTE:
	// 	This gets triggered when parent resource is created
	syncHook := f.ServeWebhook(func(request []byte) ([]byte, error) {
		req := decorator.SyncHookRequest{}
		// unmarshal http request into decorator controller hook request
		if uerr := json.Unmarshal(request, &req); uerr != nil {
			return nil, uerr
		}
		// return pod as the desired state
		resp := decorator.SyncHookResponse{}
		resp.Attachments = append(
			resp.Attachments,
			&unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Pod",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name":      "my-pod",
						"namespace": namespaceName,
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "busybox",
								"image": "busybox",
							},
						},
					},
				},
			},
		)
		return json.Marshal(resp)
	})

	// ------------------------------------------------------------
	// Define "finalize logic" for parent
	// ------------------------------------------------------------
	//
	// NOTE:
	// 	This gets triggered when parent resource is deleted
	finalizeHook := f.ServeWebhook(func(request []byte) ([]byte, error) {
		req := decorator.SyncHookRequest{}
		// unmarshal http request into decorator controller hook request
		if uerr := json.Unmarshal(request, &req); uerr != nil {
			return nil, uerr
		}
		resp := decorator.SyncHookResponse{}
		if req.Attachments.IsEmpty() {
			// set finalize to true if attachments are not observed
			// in the cluster
			resp.Finalized = true
		}
		return json.Marshal(resp)
	})

	// TestSteps are executed in their defined order
	result, err := f.Test(
		[]framework.TestStep{
			framework.TestStep{
				Name: "create-test-namespace",
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
				Name: "create-parent-crd",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "cropdctls.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "CROPDCtl",
									"listKind": "CROPDCtlList",
									"singular": "cropdctl",
									"plural":   "cropdctls",
									"shortNames": []interface{}{
										"cropdctl",
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
				Name: "create-decorator-controller",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "DecoratorController",
							"apiVersion": "metac.openebs.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name": "cr-owns-pod-via-dctl",
							},
							"spec": map[string]interface{}{
								"resources": []interface{}{
									map[string]interface{}{
										"apiVersion": "integration.test.io/v1",
										"resource":   "cropdctls",
									},
								},
								"attachments": []interface{}{
									map[string]interface{}{
										"apiVersion": "v1",
										"resource":   "pods",
									},
								},
								"hooks": map[string]interface{}{
									"sync": map[string]interface{}{
										"webhook": map[string]interface{}{
											"url": kubernetes.StringPtr(syncHook.URL),
										},
									},
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
				Name: "create-CROPDCtl-resource",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CROPDCtl",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "my-cr-that-owns-a-pod",
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-pod-creation-via-dctl",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Pod",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      "my-pod",
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "delete-CROPDCtl-resource",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CROPDCtl",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "my-cr-that-owns-a-pod",
								"namespace": namespaceName,
							},
							"spec": nil, // implies delete
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-pod-deletion-via-dctl",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Pod",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      "my-pod",
								"namespace": namespaceName,
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
