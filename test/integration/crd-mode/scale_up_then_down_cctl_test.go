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
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog"

	"openebs.io/metac/controller/composite"
	"openebs.io/metac/test/integration/framework"
	"openebs.io/metac/third_party/kubernetes"
)

func TestScaleUpThenDownViaCctl(t *testing.T) {
	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	watchName := "watch-sutdvcctl"
	namespaceName := "ns-sutdvcctl"
	attachmentName := "attachment-sutdvcctl"

	// define "reconcile logic" in this hook
	syncHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := composite.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}
		replicas, _, err := unstructured.NestedInt64(
			req.Parent.Object,
			"spec",
			"replicas",
		)
		if err != nil {
			return nil, err
		}
		var children []*unstructured.Unstructured
		var i int64
		for i = 0; i < replicas; i++ {
			child := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "ScaleUpThenDownCCtlAttachment",
					"apiVersion": "integration.test.io/v1",
					"metadata": map[string]interface{}{
						"name":      fmt.Sprintf("%s-%d", attachmentName, i),
						"namespace": req.Parent.GetNamespace(),
						"labels": map[string]interface{}{
							"watch-name": req.Parent.GetName(),
						},
					},
				},
			}
			children = append(children, child)
		}

		resp := composite.SyncHookResponse{
			Children: children,
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
								"name": "scaleupthendowncctlwatches.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "ScaleUpThenDownCCtlWatch",
									"listKind": "ScaleUpThenDownCCtlWatchList",
									"singular": "scaleupthendowncctlwatch",
									"plural":   "scaleupthendowncctlwatches",
									"shortNames": []interface{}{
										"sutdcctlwatch",
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
								"name": "scaleupthendowncctlattachments.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "ScaleUpThenDownCCtlAttachment",
									"listKind": "ScaleUpThenDownCCtlAttachmentList",
									"singular": "scaleupthendowncctlattachment",
									"plural":   "scaleupthendowncctlattachments",
									"shortNames": []interface{}{
										"sutdcctlattachment",
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
							"kind":       "ScaleUpThenDownCCtlWatch",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      watchName,
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								"replicas": int64(5),
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-composite-controller",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CompositeController",
							"apiVersion": "metac.openebs.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      "scale-up-then-down-cctl",
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								"generateSelector": kubernetes.BoolPtr(true),
								"parentResource": map[string]interface{}{
									"apiVersion": "integration.test.io/v1",
									"resource":   "scaleupthendowncctlwatches",
								},
								"childResources": []interface{}{
									map[string]interface{}{
										"apiVersion": "integration.test.io/v1",
										"resource":   "scaleupthendowncctlattachments",
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
				Name: "assert-presence-of-5-attachments",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ScaleUpThenDownCCtlAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"namespace": namespaceName,
								"labels": map[string]interface{}{
									"watch-name": watchName,
								},
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorListCountEquals,
						Count:    kubernetes.IntPtr(5),
					},
				},
			},
			framework.TestStep{
				Name: "update-watch-to-0-replicas",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ScaleUpThenDownCCtlWatch",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      watchName,
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								"replicas": int64(0),
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-presence-of-0-attachments",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "ScaleUpThenDownCCtlAttachment",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"namespace": namespaceName,
								"labels": map[string]interface{}{
									"watch-name": watchName,
								},
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorListCountEquals,
						Count:    kubernetes.IntPtr(0),
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
