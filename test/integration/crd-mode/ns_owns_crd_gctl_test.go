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

func TestNamespaceOwnsCRDViaGctl(t *testing.T) {
	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	// NOTE:
	// Keep the namespaces as unique as possible
	namespaceName := "ns-nocrdvg"

	// namespace that will be under watch
	targetNamespaceName := "tgt-nocrdvg"

	// ------------------------------------------------------------
	// Define the "reconcile logic" for sync i.e. create/update/noop event
	// ------------------------------------------------------------
	syncHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if uerr := json.Unmarshal(body, &req); uerr != nil {
			return nil, uerr
		}
		// initialize the hook response
		resp := generic.SyncHookResponse{}
		// add CRDs as desired states
		resp.Attachments = append(
			resp.Attachments,
			&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1beta1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name": "nocrdones.integration.test.io",
					},
					"spec": map[string]interface{}{
						"version": "v1alpha1",
						"group":   "integration.test.io",
						"scope":   "Cluster",
						"names": map[string]interface{}{
							"kind":     "NOCRDOne",
							"listKind": "NOCRDOneList",
							"singular": "nocrdone",
							"plural":   "nocrdones",
							"shortNames": []interface{}{
								"nocrdone",
							},
						},
						"versions": []interface{}{
							map[string]interface{}{
								"name":    "v1alpha1",
								"served":  true,
								"storage": true,
							},
						},
					},
				},
			},
		)
		resp.Attachments = append(
			resp.Attachments,
			&unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apiextensions.k8s.io/v1beta1",
					"kind":       "CustomResourceDefinition",
					"metadata": map[string]interface{}{
						"name": "nocrdtwos.integration.test.io",
					},
					"spec": map[string]interface{}{
						"version": "v1",
						"group":   "integration.test.io",
						"scope":   "Cluster",
						"names": map[string]interface{}{
							"kind":     "NOCRDTwo",
							"listKind": "NOCRDTwoList",
							"singular": "nocrdtwo",
							"plural":   "nocrdtwos",
							"shortNames": []interface{}{
								"nocrdtwo",
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
		)
		return json.Marshal(resp)
	})

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
		for _, att := range req.Attachments.List() {
			if len(att.GetFinalizers()) == 0 {
				// This CRD has no finalizers
				// Don't add to response to let Metac delete
				// the same from the cluster
				continue
			}
			att.SetFinalizers([]string{})
			resp.Attachments = append(
				resp.Attachments,
				att,
			)
		}
		// Check for presence of attachments in request
		if req.Attachments.IsEmpty() {
			// Mark this finalize hook as completed since
			// all attachments are deleted from cluster
			klog.V(2).Infof("Finalize completed")
			resp.Finalized = true
		} else {
			// Keep resyncing the watch since attachments are still
			// observed in the cluster
			klog.V(2).Infof(
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
				Name: "create-generic-controller",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "GenericController",
							"apiVersion": "metac.openebs.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      "ns-owns-crd-gctl",
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
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
											"nocrdones.integration.test.io",
											"nocrdtwos.integration.test.io",
										},
										"updateStrategy": map[string]interface{}{
											"method": "InPlace",
										},
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
				Name: "assert-presence-of-gctl-finalizer-in-target-namespace",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": targetNamespaceName,
								"finalizers": []interface{}{
									"protect.gctl.metac.openebs.io/ns-nocrdvg-ns-owns-crd-gctl",
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-presence-of-nocrdone-crd",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CustomResourceDefinition",
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"metadata": map[string]interface{}{
								"name": "nocrdones.integration.test.io",
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-presence-of-nocrdtwo-crd",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CustomResourceDefinition",
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"metadata": map[string]interface{}{
								"name": "nocrdtwos.integration.test.io",
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
				Name: "assert-non-presence-of-nocrdone-crd",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CustomResourceDefinition",
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"metadata": map[string]interface{}{
								"name": "nocrdones.integration.test.io",
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorNotFound,
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-presence-of-nocrdtwo-crd",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CustomResourceDefinition",
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"metadata": map[string]interface{}{
								"name": "nocrdtwos.integration.test.io",
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
