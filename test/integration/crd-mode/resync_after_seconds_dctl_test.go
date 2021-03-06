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
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog"

	"openebs.io/metac/controller/decorator"
	"openebs.io/metac/test/integration/framework"
	"openebs.io/metac/third_party/kubernetes"
)

func TestResyncAfterSecondsViaDctl(t *testing.T) {
	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	watchName := "my-watch"
	namespaceName := "ns-rasvdctl"

	var lastSync time.Time
	done := false

	// define "reconcile logic" in this hook
	syncHook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := decorator.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		resp := decorator.SyncHookResponse{}
		if req.Object.Object["status"] == nil {
			// 1st sync
			//
			// If status hasn't been set yet, set it.
			// This must be the first ever sync attempt.
			resp.Status = map[string]interface{}{}
		} else if lastSync.IsZero() {
			// 2nd sync
			//
			// Do nothing except request a resync.
			lastSync = time.Now()
			resp.ResyncAfterSeconds = 0.1
		} else if !done {
			// 3rd sync
			done = true
			// Report how much time elapsed in a custom status field
			resp.Status = map[string]interface{}{
				"elapsedSeconds": time.Since(lastSync).Seconds(),
			}
		} else {
			// 4th & subsequent syncs
			//
			// If we're done, just **freeze** the status.
			// In other words set the response with watch's current status.
			watchStatus := req.Object.Object["status"]
			resp.Status = watchStatus.(map[string]interface{})
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
				Name: "create-watch-crd-as-namespace-scoped",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "rasdctlwatches.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "RASDCtlWatch",
									"listKind": "RASDCtlWatchList",
									"singular": "rasdctlwatch",
									"plural":   "rasdctlwatches",
									"shortNames": []interface{}{
										"rasdctlwatch",
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
							"kind":       "RASDCtlWatch",
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
				Name: "create-decorator-controller",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "DecoratorController",
							"apiVersion": "metac.openebs.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      "resync-after-seconds-dctl",
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								"resources": []interface{}{
									map[string]interface{}{
										"apiVersion": "integration.test.io/v1",
										"resource":   "rasdctlwatches",
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
				Name: "assert-watch-status-has-elapsedseconds",
				Assert: &framework.Assert{
					PathCheck: &framework.PathCheck{
						Operator: framework.PathCheckOperatorExists,
						Path:     "status.elapsedSeconds",
					},
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "RASDCtlWatch",
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
				Name: "assert-watch-status-has-elapsedseconds-lte-1.0",
				Assert: &framework.Assert{
					PathCheck: &framework.PathCheck{
						Operator: framework.PathCheckOperatorLTE,
						DataType: framework.PathValueDataTypeFloat64,
						Path:     "status.elapsedSeconds",
						Value:    1.0,
					},
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "RASDCtlWatch",
							"apiVersion": "integration.test.io/v1",
							"metadata": map[string]interface{}{
								"name":      watchName,
								"namespace": namespaceName,
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
