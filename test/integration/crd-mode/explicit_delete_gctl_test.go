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

func TestExplicitDeleteViaGctl(t *testing.T) {
	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	namespaceName := "ns-edvg"

	// define "reconcile logic" in this hook
	syncHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		deleteOne := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "ExplicitDeleteAttachment",
				"apiVersion": "integration.test.io/v1",
				"metadata": map[string]interface{}{
					"name":      "attachment-one",
					"namespace": namespaceName,
				},
			},
		}
		deleteTwo := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "ExplicitDeleteAttachment",
				"apiVersion": "integration.test.io/v1",
				"metadata": map[string]interface{}{
					"name":      "attachment-two",
					"namespace": namespaceName,
				},
			},
		}
		deleteThree := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "ConfigMap",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name":      "attachment-three",
					"namespace": namespaceName,
				},
			},
		}
		resp := generic.SyncHookResponse{
			ExplicitDeletes: []*unstructured.Unstructured{
				deleteOne,
				deleteTwo,
				deleteThree,
			},
		}
		resp.ResyncAfterSeconds = 1.0
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
				Name: "create-watch-crd",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "explicitdeletewatches.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "ExplicitDeleteWatch",
									"listKind": "ExplicitDeleteWatchList",
									"singular": "explicitdeletewatch",
									"plural":   "explicitdeletewatches",
									"shortNames": []interface{}{
										"edwatch",
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
				Name: "create-attachment-crd",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "explicitdeleteattachments.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "ExplicitDeleteAttachment",
									"listKind": "ExplicitDeleteAttachmentList",
									"singular": "explicitdeleteattachment",
									"plural":   "explicitdeleteattachments",
									"shortNames": []interface{}{
										"edattachment",
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
				Name: "create-watch-resource",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitDeleteWatch",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "delete-all",
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-external-attachment-one",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitDeleteAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-one",
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-external-attachment-two",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitDeleteAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-two",
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
								"name":      "explicit-delete-gctl",
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								"watch": map[string]interface{}{
									"apiVersion": "integration.test.io/v1",
									"resource":   "explicitdeletewatches",
								},
								"attachments": []interface{}{
									map[string]interface{}{
										"apiVersion": "integration.test.io/v1",
										"resource":   "explicitdeleteattachments",
									},
									map[string]interface{}{
										"apiVersion": "v1",
										"resource":   "configmaps",
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
				Name: "create-external-attachment-three",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ConfigMap",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-three",
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-presence-of-attachment-one",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitDeleteAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-one",
								"namespace": namespaceName,
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorNotFound,
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-presence-of-attachment-two",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitDeleteAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-two",
								"namespace": namespaceName,
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorNotFound,
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-presence-of-attachment-three",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ConfigMap",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-three",
								"namespace": namespaceName,
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorNotFound,
					},
				},
			},
			framework.TestStep{
				Name: "create-attachment-one-again",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitDeleteAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-one",
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-attachment-three-again",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ConfigMap",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-three",
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-presence-of-attachment-three-again",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ConfigMap",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-three",
								"namespace": namespaceName,
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorNotFound,
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-presence-of-attachment-one-again",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitDeleteAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "attachment-one",
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
