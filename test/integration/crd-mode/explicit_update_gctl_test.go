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

func TestExplicitUpdateViaGctl(t *testing.T) {
	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	namespaceName := "ns-euolvg"

	// define "reconcile logic" in this hook
	syncHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		owned := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "ExplicitUpdateAttachment",
				"apiVersion": "integration.test.io/v1",
				"metadata": map[string]interface{}{
					"name":      "owned-attachment",
					"namespace": namespaceName,
					"labels": map[string]interface{}{
						"app":      "metac",
						"explicit": "true",
					},
					"annotations": map[string]interface{}{
						"app":      "metac",
						"explicit": "true",
					},
				},
			},
		}
		nonOwned := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind":       "ExplicitUpdateAttachment",
				"apiVersion": "integration.test.io/v1",
				"metadata": map[string]interface{}{
					"name":      "non-owned-attachment",
					"namespace": namespaceName,
					"labels": map[string]interface{}{
						"explicit": "true",
					},
					"annotations": map[string]interface{}{
						"explicit": "true",
					},
				},
			},
		}
		resp := generic.SyncHookResponse{
			Attachments:     []*unstructured.Unstructured{owned},
			ExplicitUpdates: []*unstructured.Unstructured{nonOwned},
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
				Name: "create-watch-crd",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "explicitupdatewatches.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "ExplicitUpdateWatch",
									"listKind": "ExplicitUpdateWatchList",
									"singular": "explicitupdatewatch",
									"plural":   "explicitupdatewatches",
									"shortNames": []interface{}{
										"euwatch",
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
								"name": "explicitupdateattachments.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "ExplicitUpdateAttachment",
									"listKind": "ExplicitUpdateAttachmentList",
									"singular": "explicitupdateattachment",
									"plural":   "explicitupdateattachments",
									"shortNames": []interface{}{
										"euattachment",
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
							"kind":       "ExplicitUpdateWatch",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "my-watch",
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-external-attachment-resource",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitUpdateAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "non-owned-attachment",
								"namespace": namespaceName,
								"labels": map[string]interface{}{
									"app": "non-metac",
								},
								"annotations": map[string]interface{}{
									"app": "non-metac",
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-external-configmap",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ConfigMap",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      "my-read-only-config",
								"namespace": namespaceName,
								"labels": map[string]interface{}{
									"app": "non-metac",
								},
								"annotations": map[string]interface{}{
									"app": "non-metac",
								},
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
								"name":      "explicit-update-via-gctl",
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								"watch": map[string]interface{}{
									"apiVersion": "integration.test.io/v1",
									"resource":   "explicitupdatewatches",
								},
								"attachments": []interface{}{
									map[string]interface{}{
										"apiVersion": "integration.test.io/v1",
										"resource":   "explicitupdateattachments",
										"updateStrategy": map[string]interface{}{
											"method": "InPlace",
										},
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
				Name: "assert-presence-of-owned-attachment",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitUpdateAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "owned-attachment",
								"namespace": namespaceName,
								"labels": map[string]interface{}{
									"app":      "metac",
									"explicit": "true",
								},
								"annotations": map[string]interface{}{
									"app":      "metac",
									"explicit": "true",
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-presence-of-non-owned-attachment",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitUpdateAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "non-owned-attachment",
								"namespace": namespaceName,
								"labels": map[string]interface{}{
									"app":      "non-metac",
									"explicit": "true",
								},
								"annotations": map[string]interface{}{
									"app":      "non-metac",
									"explicit": "true",
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-presence-of-external-configmap",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ConfigMap",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      "my-read-only-config",
								"namespace": namespaceName,
								"labels": map[string]interface{}{
									"app": "non-metac",
								},
								"annotations": map[string]interface{}{
									"app": "non-metac",
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "update-non-owned-attachment",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitUpdateAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "non-owned-attachment",
								"namespace": namespaceName,
								"labels": map[string]interface{}{
									"app":       nil,
									"new-stuff": "true",
								},
								"annotations": map[string]interface{}{
									"app":       nil,
									"new-stuff": "true",
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-owned-attachment-with-new-updates",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ExplicitUpdateAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      "non-owned-attachment",
								"namespace": namespaceName,
								"labels": map[string]interface{}{
									"app":       "",
									"new-stuff": "true",
									"explicit":  "true",
								},
								"annotations": map[string]interface{}{
									"app":       "",
									"new-stuff": "true",
									"explicit":  "true",
								},
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
