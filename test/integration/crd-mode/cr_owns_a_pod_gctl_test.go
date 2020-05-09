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

// TestCustomResourceOwnsAPodViaGctl will verify if GenericController
// can be used to create & delete pod based on presence &
// absence of a custom resource
func TestCustomResourceOwnsAPodViaGctl(t *testing.T) {

	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	namespaceName := "my-ns"
	customResourceName := "my-deploy"
	podName := "my-pod"

	// ------------------------------------------------------------
	// Define "reconcile logic" for MyDeploy
	// ------------------------------------------------------------
	//
	// NOTE:
	// 	This gets triggered when MyDeploy resource is created
	syncHook := f.ServeWebhook(func(request []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		// unmarshal http request into generic hook request
		if uerr := json.Unmarshal(request, &req); uerr != nil {
			return nil, uerr
		}
		// watch UID will be set as annotation against the desired pod
		watchUID := req.Watch.GetUID()
		// build hook response that includes a pod as the desired state
		resp := generic.SyncHookResponse{}
		resp.Attachments = append(
			resp.Attachments,
			&unstructured.Unstructured{
				Object: map[string]interface{}{
					"kind":       "Pod",
					"apiVersion": "v1",
					"metadata": map[string]interface{}{
						"name":      podName,
						"namespace": namespaceName,
						"annotations": map[string]interface{}{
							"watch-uid": watchUID,
						},
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
	// Define "finalize logic" for MyDeploy
	// ------------------------------------------------------------
	//
	// NOTE:
	// 	This gets triggered when MyDeploy resource is deleted
	finalizeHook := f.ServeWebhook(func(request []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		// unmarshal http request into generic hook request
		if uerr := json.Unmarshal(request, &req); uerr != nil {
			return nil, uerr
		}
		resp := generic.SyncHookResponse{}
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
				Name: "create-namespace",
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
				Name: "create-my-deploy-crd",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "mydeploys.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1alpha1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "MyDeploy",
									"listKind": "MyDeployList",
									"singular": "mydeploy",
									"plural":   "mydeploys",
									"shortNames": []interface{}{
										"mdeploy",
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
				Name: "create-a-generic-controller",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "GenericController",
							"apiVersion": "metac.openebs.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      "my-generic-controller",
								"namespace": namespaceName,
							},
							"spec": map[string]interface{}{
								//"resyncPeriodSeconds": kubernetes.Int32Ptr(3600),
								"watch": map[string]interface{}{
									"apiVersion": "integration.test.io/v1alpha1",
									"resource":   "mydeploys",
								},
								"attachments": []interface{}{
									map[string]interface{}{
										"apiVersion": "v1",
										"resource":   "pods",
										"advancedSelector": map[string]interface{}{
											"selectorTerms": []interface{}{
												map[string]interface{}{
													"matchReferenceExpressions": []interface{}{
														map[string]interface{}{
															"key":    "metadata.annotations.watch-uid",
															"refKey": "metadata.uid",
														},
													},
												},
											},
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
				Name: "create-mydeploy-resource",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "MyDeploy",
							"apiVersion": "integration.test.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      customResourceName,
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-pod-creation",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Pod",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      podName,
								"namespace": namespaceName,
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "delete-mydeploy-resource",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "MyDeploy",
							"apiVersion": "integration.test.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      customResourceName,
								"namespace": namespaceName,
							},
							"spec": nil, // implies delete
						},
					},
				},
			},
			framework.TestStep{
				Name: "assert-pod-deletion",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Pod",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name":      podName,
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
