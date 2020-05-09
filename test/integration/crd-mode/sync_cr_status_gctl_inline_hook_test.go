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
	"k8s.io/klog"

	"openebs.io/metac/controller/generic"
	"openebs.io/metac/test/integration/framework"
)

func TestSyncCRStatusViaGctlInlineHook(t *testing.T) {
	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	watchName := "my-watch"
	namespaceName := "ns-scrsvgih"

	// define "reconcile logic" as an inline hook
	syncHook := func(req *generic.SyncHookRequest, resp *generic.SyncHookResponse) error {
		resp.Status = map[string]interface{}{
			"phase": "Active",
			"conditions": []string{
				"GenericController",
				"InlineHookCall",
			},
		}
		return nil
	}

	// Add this sync hook implementation to inline hook registry
	var inlineHookName = "sync/cr-status"
	generic.AddToInlineRegistry(inlineHookName, syncHook)

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
				Name: "create-watch-crd-as-namespace-scoped",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "scrswatches.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "SCRSWatch",
									"listKind": "SCRSWatchList",
									"singular": "scrswatch",
									"plural":   "scrswatches",
									"shortNames": []interface{}{
										"scrswatch",
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
				Name: "create-generic-controller",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "GenericController",
							"apiVersion": "metac.openebs.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      "sync-cr-status-gctl-inline-hook",
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								"watch": map[string]interface{}{
									"apiVersion": "integration.test.io/v1",
									"resource":   "scrswatches",
								},
								"hooks": map[string]interface{}{
									"sync": map[string]interface{}{
										"inline": map[string]interface{}{
											"funcName": inlineHookName,
										},
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
							"kind":       "SCRSWatch",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      watchName,
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-watch-status",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "SCRSWatch",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      watchName,
								"namespace": namespaceName,
							},
							"status": map[string]interface{}{
								"phase": "Active",
								"conditions": []interface{}{
									"GenericController",
									"InlineHookCall",
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
