/*
Copyright 2019 The MayaData Authors.

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

package common

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/watch"
	"openebs.io/metac/apis/metacontroller/v1alpha1"
	dynamicapply "openebs.io/metac/dynamic/apply"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	"openebs.io/metac/third_party/kubernetes"
)

type NoopResourceOperation struct{}

func (n NoopResourceOperation) Create(obj *unstructured.Unstructured, options metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (n NoopResourceOperation) Update(obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (n NoopResourceOperation) UpdateStatus(obj *unstructured.Unstructured, options metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (n NoopResourceOperation) Delete(name string, options *metav1.DeleteOptions, subresources ...string) error {
	return nil
}
func (n NoopResourceOperation) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}
func (n NoopResourceOperation) Get(name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}
func (n NoopResourceOperation) List(opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return nil, nil
}
func (n NoopResourceOperation) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}
func (n NoopResourceOperation) Patch(name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func TestAttachmentResourcesExecutorUpdate(t *testing.T) {
	lastAppliedKey := "test-watch-uid/gctl-last-applied"

	mockAttachmentExecutor := &ResourceStatesController{
		ClusterStatesControllerBase: ClusterStatesControllerBase{
			GetChildUpdateStrategyByGK: func(group, kind string) v1alpha1.ChildUpdateMethod {
				return v1alpha1.ChildUpdateInPlace
			},
			Watch: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"uid": "test-watch-uid",
					},
				},
			},
			UpdateAny: kubernetes.BoolPtr(true),
		},
		DynamicClient: &dynamicclientset.ResourceClient{
			ResourceInterface: &NoopResourceOperation{},
			APIResource:       &dynamicdiscovery.APIResource{},
		},
	}

	var tests = map[string]struct {
		observedJSON string
		desiredJSON  string

		// set either lastApplied or isRunAsPatch
		// Both of them can be used to execute merge either as:
		// 	1/ Patch operation or,
		//	2/ 3-way merge operation
		lastApplied  string
		isRunAsPatch bool

		isUpdate bool
	}{
		//
		// test spec
		//
		"with updates": {
			observedJSON: `{
				"metadata": {
					"meta": "old value"
				},
				"spec": "old value"
			}`,
			desiredJSON: `{
				"metadata": {
					"meta": "new value"
				},
				"spec": "new value"
			}`,
			isUpdate: true,
		},
		"with no changes": {
			observedJSON: `{
				"metadata": {
					"orig": "hello"
				},
				"spec": "who-am-i"
			}`,
			desiredJSON: `{
				"metadata": {
					"orig": "hello"
				},
				"spec": "who-am-i"
			}`,
			isUpdate: false,
		},
		"with additions": {
			observedJSON: `{
				"metadata": {
					"field1": "value1"
				},
				"spec": {
					"hi": "there"
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"field1": "value1",
					"field2": "value2"
				},
				"spec": {
					"hi": "there",
					"ping": "pong"
				}
			}`,
			isUpdate: true,
		},
		"with removals": {
			observedJSON: `{
				"metadata": {
					"field1": "value1",
					"field2": "value2"
				},
				"spec": {
					"hi": "there",
					"ping": "pong"
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"field1": "value1"
				},
				"spec": {
					"hi": "there"
				}
			}`,
			isUpdate: false,
		},
		"with removals via patch": {
			observedJSON: `{
				"metadata": {
					"field1": "value1",
					"field2": "value2"
				},
				"spec": {
					"hi": "there",
					"ping": "pong"
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"field1": "value1"
				},
				"spec": {
					"hi": "there"
				}
			}`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		//
		// test labels
		//
		"no changes to labels": {
			observedJSON: `{
				"metadata": {
					"labels": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"labels": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			isUpdate: false,
		},
		"new additions to existing labels": {
			observedJSON: `{
				"metadata": {
					"labels": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"labels": {
						"app": "openebs",
						"type": "storage",
						"ssd": "true"
					}
				}
			}`,
			isUpdate: true,
		},
		"remove all existing & add new labels": {
			observedJSON: `{
				"metadata": {
					"labels": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"labels": {
						"ssd": "true"
					}
				}
			}`,
			isUpdate: true,
		},
		"remove some labels from existing": {
			observedJSON: `{
				"metadata": {
					"labels": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"labels": {
						"type": "storage"
					}
				}
			}`,
			isUpdate: false,
		},
		"remove some labels from existing via patch": {
			observedJSON: `{
				"metadata": {
					"labels": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"labels": {
						"type": "storage"
					}
				}
			}`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		"empty the labels": {
			observedJSON: `{
		 		"metadata": {
		 			"labels": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
		 			"labels": {}
		 		}
		 	}`,
			isUpdate: false,
		},
		"empty the labels via patch": {
			observedJSON: `{
		 		"metadata": {
		 			"labels": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
		 			"labels": {}
		 		}
			 }`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		"nullify the labels": {
			observedJSON: `{
		 		"metadata": {
		 			"labels": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {}
		 	}`,
			isUpdate: false,
		},
		"nullify the labels via patch": {
			observedJSON: `{
		 		"metadata": {
		 			"labels": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {}
			 }`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		//
		// test annotations
		//
		"remove all existing & add new annotations": {
			observedJSON: `{
				"metadata": {
					"annotations": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"annotations": {
						"ssd": "true"
					}
				}
			}`,
			isUpdate: true,
		},
		"add new annotations to existing": {
			observedJSON: `{
				"metadata": {
					"annotations": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"annotations": {
						"app": "openebs",
						"type": "storage",
						"ssd": "true"
					}
				}
			}`,
			isUpdate: true,
		},
		"remove some annotations from existing": {
			observedJSON: `{
				"metadata": {
					"annotations": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"annotations": {
						"type": "storage"
					}
				}
			}`,
			isUpdate: false,
		},
		"remove some annotations from existing via patch": {
			observedJSON: `{
				"metadata": {
					"annotations": {
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"annotations": {
						"type": "storage"
					}
				}
			}`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		"nullify the annotations": {
			observedJSON: `{
		 		"metadata": {
		 			"annotations": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {}
		 	}`,
			isUpdate: false,
		},
		"nullify the annotations via patch": {
			observedJSON: `{
		 		"metadata": {
		 			"annotations": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {}
			 }`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		"empty the annotations": {
			observedJSON: `{
		 		"metadata": {
		 			"annotations": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
		 			"annotations": {}
		 		}
		 	}`,
			isUpdate: false,
		},
		"empty the annotations via patch": {
			observedJSON: `{
		 		"metadata": {
		 			"annotations": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
		 			"annotations": {}
		 		}
			 }`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		//
		// test finalizers
		//
		"remove all existing & add new finalizers": {
			observedJSON: `{
				"metadata": {
					"finalizers": [
						"openebs-protect",
						"storage-protect"
					]
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"finalizers": [
						"zone-protect"
					]
				}
			}`,
			isUpdate: true,
		},
		"add new finalizers to existing": {
			observedJSON: `{
				"metadata": {
					"finalizers": [
						"zone-protect"
					]
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"finalizers": [
						"zone-protect",
						"storage-protect"
					]
				}
			}`,
			isUpdate: true,
		},
		"remove some finalizers from existing": {
			observedJSON: `{
				"metadata": {
					"finalizers": [
						"zone-protect",
						"storage-protect"
					]
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"finalizers": [
						"storage-protect"
					]
				}
			}`,
			isUpdate: true,
		},
		"remove some finalizers from existing via patch": {
			observedJSON: `{
				"metadata": {
					"finalizers": [
						"zone-protect",
						"storage-protect"
					]
				}
			}`,
			desiredJSON: `{
				"metadata": {
					"finalizers": [
						"storage-protect"
					]
				}
			}`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		"nullify the finalizers": {
			observedJSON: `{
		 		"metadata": {
					"finalizers": [
						"storage-protect",
						"zone-protect"
					]
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {}
		 	}`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		"nullify the finalizers via patch": {
			observedJSON: `{
		 		"metadata": {
					"finalizers": [
						"storage-protect",
						"zone-protect"
					]
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {}
			 }`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		"empty the finalizers": {
			observedJSON: `{
		 		"metadata": {
					"finalizers": [
						"storage-protect",
						"zone-protect"
					]
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
		 			"finalizers": []
		 		}
		 	}`,
			isUpdate: true,
		},
		"empty the finalizers via patch": {
			observedJSON: `{
		 		"metadata": {
					"finalizers": [
						"storage-protect",
						"zone-protect"
					]
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
		 			"finalizers": []
		 		}
			 }`,
			isRunAsPatch: true,
			isUpdate:     true,
		},
		//
		// test labels with last applied state
		//
		"remove some labels from existing with last applied": {
			observedJSON: `{
		 		"metadata": {
		 			"labels": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
					"labels": {
						"app": "openebs"
					}
				}
			}`,
			lastApplied: `{
				"metadata":{
					"labels":{
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			isUpdate: true,
		},
		"nullify the labels with last applied": {
			observedJSON: `{
		 		"metadata": {
		 			"labels": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {}
			}`,
			lastApplied: `{
				"metadata":{
					"labels":{
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			isUpdate: true,
		},
		"empty the labels with last applied": {
			observedJSON: `{
		 		"metadata": {
		 			"labels": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
					"labels": {}
				}
			}`,
			lastApplied: `{
				"metadata":{
					"labels":{
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			isUpdate: true,
		},
		//
		// test annotations with last applied state
		//
		"remove some annotations from existing with last applied": {
			observedJSON: `{
		 		"metadata": {
		 			"annotations": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
					"annotations": {
						"app": "openebs"
					}
				}
			}`,
			lastApplied: `{
				"metadata":{
					"annotations":{
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			isUpdate: true,
		},
		"nullify the annotations with last applied": {
			observedJSON: `{
		 		"metadata": {
		 			"annotations": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {}
			}`,
			lastApplied: `{
				"metadata":{
					"annotations":{
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			isUpdate: true,
		},
		"empty the annotations with last applied": {
			observedJSON: `{
		 		"metadata": {
		 			"annotations": {
		 				"app": "openebs",
		 				"type": "storage"
		 			}
		 		}
		 	}`,
			desiredJSON: `{
		 		"metadata": {
					"annotations": {}
				}
			}`,
			lastApplied: `{
				"metadata":{
					"annotations":{
						"app": "openebs",
						"type": "storage"
					}
				}
			}`,
			isUpdate: true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			observed := make(map[string]interface{})
			if err := json.Unmarshal([]byte(test.observedJSON), &observed); err != nil {
				t.Fatalf("Can't unmarshal observed: %v", err)
			}
			desired := make(map[string]interface{})
			if err := json.Unmarshal([]byte(test.desiredJSON), &desired); err != nil {
				t.Fatalf("Can't unmarshal desired: %v", err)
			}
			observedUnstruct := &unstructured.Unstructured{Object: observed}
			desiredUnstruct := &unstructured.Unstructured{Object: desired}

			// init IsPatchByGK func to return bool that in turn executes
			// the merge function either as:
			//	1/ a patch operation if true or
			//	2/ 3-way merge operation if false
			mockAttachmentExecutor.IsPatchByGK = func(group, kind string) bool {
				return test.isRunAsPatch
			}

			// if test has specified the last applied then use it
			//
			// NOTE:
			//	If last applied state matches the observed instance's
			// content then the 3-way merge will be same as a patch
			// operation
			lastApplied := make(map[string]interface{})
			if test.lastApplied != "" {
				// Use last applied state from the test case if specified
				if err := json.Unmarshal(
					[]byte(test.lastApplied), &lastApplied,
				); err != nil {
					t.Fatalf("can't unmarshal lastApplied: %+v", err)
				}
			}
			if len(lastApplied) != 0 {
				err := dynamicapply.SetLastAppliedByAnnKey(
					observedUnstruct,
					lastApplied,
					lastAppliedKey,
				)
				if err != nil {
					t.Fatalf("can't set last applied to observed: %+v", err)
				}
			}

			// this is the **function under test**
			isUpdate, err := mockAttachmentExecutor.update(observedUnstruct, desiredUnstruct)
			if err != nil {
				t.Fatalf("Test %s error: %v", name, err)
			}
			if isUpdate != test.isUpdate {
				t.Fatalf("Test %s failed: got %t want %t", name, isUpdate, test.isUpdate)
			}
		})
	}
}
