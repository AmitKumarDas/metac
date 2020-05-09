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

// TestDeleteNamespaceDeletesCRDAndCRViaGctl will verify if
// GenericController can be used to implement clean uninstall
// requirements.
//
// Use Case: When a workload Namespace is removed from kubernetes
// cluster, its associated CRDs and CRs should get removed from
// this cluster. This should work even in the cases where CRs are
// set with finalizers.
func TestDeleteNamespaceDeletesCRDAndCRViaGctl(t *testing.T) {
	f := framework.NewIntegrationTester(t)
	defer f.TearDown()

	// NOTE:
	// Keep the namespaces as unique as possible

	// GenericController namespace which needs to be different
	// than target namespace
	gctlNamespaceName := "gctl-ns-dndcacvg"

	// name of the target namespace which is watched by GenericController
	targetNamespaceName := "target-ns-dndcacvg"

	// name of attachments managed by GenericController
	nsDeployCRName := "ns-deploy"
	nsServiceCRName := "ns-service"

	// ------------------------------------------------------------
	// Define the "reconcile logic" for finalize i.e. delete event
	// ------------------------------------------------------------
	//
	// NOTE:
	// 	This gets triggered upon deletion of target namespace
	//
	// NOTE:
	// 	This is a multi process reconciliation strategy:
	//		Stage 1: Remove finalizers from custom resources
	//      Stage 2: Delete custom resources that dont have finalizers
	//		Stage 3: Delete CustomResourceDefinition(s) when there are no custom resource(s)
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
				// keep CRD till all its corresponding custom resources get deleted
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
				Name: "create-controller-namespace",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": gctlNamespaceName,
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
				Name: "create-nsdeploy-crd-as-namespace-scoped",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "nsdeploys.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1alpha1",
								"group":   "integration.test.io",
								"scope":   "Namespaced",
								"names": map[string]interface{}{
									"kind":     "NsDeploy",
									"listKind": "DeployList",
									"singular": "nsdeploy",
									"plural":   "nsdeploys",
									"shortNames": []interface{}{
										"nsdeploy",
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
				Name: "create-nsservice-crd-as-cluster-scope",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"kind":       "CustomResourceDefinition",
							"metadata": map[string]interface{}{
								"name": "nsservices.integration.test.io",
							},
							"spec": map[string]interface{}{
								"version": "v1alpha1",
								"group":   "integration.test.io",
								"scope":   "Cluster",
								"names": map[string]interface{}{
									"kind":     "NsService",
									"listKind": "NsServiceList",
									"singular": "nsservice",
									"plural":   "nsservices",
									"shortNames": []interface{}{
										"nssvc",
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
				Name: "create-nsdeploy-resource",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "NsDeploy",
							"apiVersion": "integration.test.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name":      nsDeployCRName,
								"namespace": targetNamespaceName,
								"finalizers": []interface{}{
									"protect.nsdeploy.io",
									"protect.nsdeploy.io",
								},
							},
						},
					},
				},
			},
			framework.TestStep{
				Name: "create-nsservice-resource",
				Apply: framework.Apply{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "NsService",
							"apiVersion": "integration.test.io/v1alpha1",
							"metadata": map[string]interface{}{
								"name": nsServiceCRName,
								"finalizers": []interface{}{
									"protect.nsservice.io",
									"protect.nsservice.io",
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
								"name":      "delete-ns-deletes-crd-cr-gctl",
								"namespace": gctlNamespaceName,
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
										"apiVersion": "integration.test.io/v1alpha1",
										"resource":   "nsdeploys",
										"updateStrategy": map[string]interface{}{
											"method": "InPlace",
										},
									},
									map[string]interface{}{
										"apiVersion": "integration.test.io/v1alpha1",
										"resource":   "nsservices",
										"updateStrategy": map[string]interface{}{
											"method": "InPlace",
										},
									},
									map[string]interface{}{
										"apiVersion": "apiextensions.k8s.io/v1beta1",
										"resource":   "customresourcedefinitions",
										"nameSelector": []interface{}{
											"nsdeploys.integration.test.io",
											"nsservices.integration.test.io",
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
				Name: "assert-presence-of-gctl-finalizer-in-target-namespace",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "Namespace",
							"apiVersion": "v1",
							"metadata": map[string]interface{}{
								"name": targetNamespaceName,
								"finalizers": []interface{}{
									"protect.gctl.metac.openebs.io/gctl-ns-dndcacvg-delete-ns-deletes-crd-cr-gctl",
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
				Name: "assert-non-presence-of-nsdeploy-crd",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CustomResourceDefinition",
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"metadata": map[string]interface{}{
								"name": "nsdeploys.integration.test.io",
							},
						},
					},
					StateCheck: &framework.StateCheck{
						Operator: framework.StateCheckOperatorNotFound,
					},
				},
			},
			framework.TestStep{
				Name: "assert-non-presence-of-nsservice-crd",
				Assert: &framework.Assert{
					State: &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "CustomResourceDefinition",
							"apiVersion": "apiextensions.k8s.io/v1beta1",
							"metadata": map[string]interface{}{
								"name": "nsservices.integration.test.io",
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
