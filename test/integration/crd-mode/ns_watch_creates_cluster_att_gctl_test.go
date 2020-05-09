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

func TestNamespacedWatchCreatesClusterScopedAttachmentViaGctl(t *testing.T) {
	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	watchName := "my-watch"
	namespaceName := "test-nwccsavg"
	attachmentName := "my-attachment-nwccsavg"

	// define "reconcile logic" in this hook
	syncHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		child := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "MyCAttachment",
				"apiVersion": "integration.test.io/v1alpha1",
				"metadata": map[string]interface{}{
					"name": attachmentName,
					"labels": map[string]interface{}{
						"do-not-change": "lbl-101",
					},
				},
			},
		}
		resp := generic.SyncHookResponse{
			Attachments: []*unstructured.Unstructured{child},
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
				Name: "create-testcase-namespace",
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
				Name: "create-mywatch-crd-as-namespace-scoped",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "mynswatches.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1alpha1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "MyNsWatch",
									"listKind": "MyNsWatchList",
									"singular": "mynswatch",
									"plural":   "mynswatches",
									"shortNames": []interface{}{
										"mynswatch",
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
				},
			},
			framework.TestStep{
				Name: "create-myattachment-crd-as-cluster-scope",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "mycattachments.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1alpha1",
								"group":   "integration.test.io",
								"scope":   "Cluster",
								"names": map[string]interface{}{
									"kind":     "MyCAttachment",
									"listKind": "MyCAttachmentList",
									"singular": "mycattachment",
									"plural":   "mycattachments",
									"shortNames": []interface{}{
										"mycattachment",
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
				},
			},
			framework.TestStep{
				Name: "create-mywatch-resource",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "MyNsWatch",
							"apiVersion": "integration.test.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      watchName,
								"namespace": namespaceName,
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
								"name":      "ns-watch-creates-cluster-att-gctl",
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								"watch": map[string]interface{}{
									"apiVersion": "integration.test.io/v1alpha1",
									"resource":   "mynswatches",
								},
								"attachments": []interface{}{
									map[string]interface{}{
										"apiVersion": "integration.test.io/v1alpha1",
										"resource":   "mycattachments",
									},
								},
								"hooks": map[string]interface{}{
									"sync": map[string]interface{}{
										"webhook": map[string]interface{}{
											"url": kubernetes.StringPtr(syncHook.URL),
										},
									},
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-presence-of-myattachment",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "MyCAttachment",
							"apiVersion": "integration.test.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name": attachmentName,
							},
						},
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
