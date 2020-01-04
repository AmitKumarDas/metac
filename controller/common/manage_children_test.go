package common

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	dynamicapply "openebs.io/metac/dynamic/apply"
)

func TestApplyMerge(t *testing.T) {
	lastAppliedKey := "last-applied-state"

	tests := map[string]struct {
		observed                string
		desired                 string
		want                    string
		isSetLastAppliedIfEmpty bool
		lastApplied             string
		isDiff                  bool
		repeatTestCount         int
	}{
		//
		// test the finalizers
		//
		"no changes to finalizers": {
			observed: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  false,
			repeatTestCount:         5,
		},
		"add new finalizers to empty slice": {
			observed: `{
				"metadata": {
					"finalizers": []
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"add new finalizers to nil slice": {
			observed: `{
				"metadata": {}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"add new finalizers to existing": {
			observed: `{
				"metadata": {
					"finalizers": [
						"storage-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"remove some finalizers from existing": {
			observed: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"storage-protection"
					]
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": [
						"storage-protection"
					]
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"add & remove finalizers from existing": {
			observed: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"ssd-protection"
					]
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"ssd-protection"
					]
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"empty the finalizers": {
			observed: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": []
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": []
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"nullify the finalizers": {
			observed: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {}
			}`,
			want: `{
				"metadata": {}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"remove all existing finalizers & add new ones": {
			observed: `{
				"metadata": {
					"finalizers": [
						"storage-protection",
						"openebs-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		//
		// test finalizers with last applied state
		//
		"remove all existing & add new finalizers with different last applied": {
			observed: `{
				"metadata": {
					"finalizers": [
						"openebs-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			lastApplied: `{
				"metadata": {}
			}`,
			isDiff:          true,
			repeatTestCount: 5,
		},
		"no changes to finalizers with different last applied": {
			observed: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			lastApplied: `{
				"metadata": {}
			}`,
			isDiff:          false,
			repeatTestCount: 5,
		},
		"remove all existing finalizers with different last applied": {
			observed: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": []
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": []
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			lastApplied: `{
				"metadata": {}
			}`,
			isDiff:          true,
			repeatTestCount: 5,
		},
		"remove all existing finalizers with observed content as last applied": {
			observed: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": []
				}
			}`,
			want: `{
				"metadata": {
					"finalizers": []
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			lastApplied: `{
				"metadata": {
					"finalizers": [
						"zone-protection"
					]
				}
			}`,
			isDiff:          true,
			repeatTestCount: 5,
		},
		//
		// test the labels
		//
		"no changes to labels": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs",
						"ssd": "true"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs",
						"ssd": "true"
					}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs",
						"ssd": "true"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  false,
			repeatTestCount:         5,
		},
		"add new labels to empty map": {
			observed: `{
				"metadata": {
					"labels": {}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"add new labels to nil": {
			observed: `{
				"metadata": {}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"add new labels to existing": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"remove labels from existing": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"app": "storage"
					}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"app": "storage"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"add & remove labels from existing": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"ssd": "true"
					}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"ssd": "true"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"empty the labels": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"nullify the labels": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {}
			}`,
			want: `{
				"metadata": {}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"remove all existing labels & add new ones": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"ssd": "true",
						"zone": "east"
					}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"ssd": "true",
						"zone": "east"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		//
		// test labels with observed last applied state
		//
		"remove all existing & add new labels with different last applied": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"ssd": "true",
						"zone": "east"
					}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs",
						"ssd": "true",
						"zone": "east"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			lastApplied: `{
				"metadata": {
					"labels": {}
				}
			}`,
			isDiff:          true,
			repeatTestCount: 5,
		},
		"update all labels with different last applied": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"app": "stor",
						"type": "none"
					}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"app": "stor",
						"type": "none"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			lastApplied: `{
				"metadata": {
					"labels": {}
				}
			}`,
			isDiff:          true,
			repeatTestCount: 5,
		},
		"remove all existing labels with different last applied": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			lastApplied: `{
				"metadata": {}
			}`,
			isDiff:          false,
			repeatTestCount: 5,
		},
		"remove all existing labels with observed content as last applied": {
			observed: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {}
				}
			}`,
			want: `{
				"metadata": {
					"labels": {}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			lastApplied: `{
				"metadata": {
					"labels": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			isDiff:          true,
			repeatTestCount: 5,
		},
		//
		// test the annotations
		//
		"no changes to annotations": {
			observed: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs",
						"ssd": "true"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs",
						"ssd": "true"
					}
				}
			}`,
			want: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs",
						"ssd": "true"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  false,
			repeatTestCount:         5,
		},
		"add new annotations to empty": {
			observed: `{
				"metadata": {
					"annotations": {}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			want: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"add new annotations to nil": {
			observed: `{
				"metadata": {}
			}`,
			desired: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			want: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"add new annotations to existing": {
			observed: `{
				"metadata": {
					"annotations": {
						"app": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			want: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"remove annotations from existing": {
			observed: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {
						"app": "storage"
					}
				}
			}`,
			want: `{
				"metadata": {
					"annotations": {
						"app": "storage"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"add & remove annotations from existing": {
			observed: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"ssd": "true"
					}
				}
			}`,
			want: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"ssd": "true"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"empty the annotations": {
			observed: `{
				"metadata": {
					"annotations": {
						"app": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {}
				}
			}`,
			want: `{
				"metadata": {
					"annotations": {}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"nullify the annotations": {
			observed: `{
				"metadata": {
					"annotations": {
						"app": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {}
			}`,
			want: `{
				"metadata": {}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
		"remove all existing annotations & add new ones": {
			observed: `{
				"metadata": {
					"annotations": {
						"app": "storage",
						"type": "openebs"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {
						"ssd": "true",
						"zone": "east"
					}
				}
			}`,
			want: `{
				"metadata": {
					"annotations": {
						"ssd": "true",
						"zone": "east"
					}
				}
			}`,
			isSetLastAppliedIfEmpty: true,
			isDiff:                  true,
			repeatTestCount:         5,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			observedObj := make(map[string]interface{})
			if err := json.Unmarshal([]byte(test.observed), &observedObj); err != nil {
				t.Fatalf("can't unmarshal observed: %+v", err)
			}
			desiredObj := make(map[string]interface{})
			if err := json.Unmarshal([]byte(test.desired), &desiredObj); err != nil {
				t.Fatalf("can't unmarshal desired: %+v", err)
			}
			wantObj := make(map[string]interface{})
			if err := json.Unmarshal([]byte(test.want), &wantObj); err != nil {
				t.Fatalf("can't unmarshal want: %+v", err)
			}

			// Prepare the observed instance as an unstructured instance
			observedUn := &unstructured.Unstructured{
				Object: observedObj,
			}
			// Below is a nested if else logic that tries to set last applied
			// state against the observed instance
			//
			// NOTE:
			//	This is only done for first test iteration only
			if test.isSetLastAppliedIfEmpty {
				// NOTE:
				// 	This simulates the behaviour of GenericController's Update
				//
				// NOTE:
				//	A missing last applied state in observed instance's annotations
				// is expected to happen only once in its entire lifecycle
				// i.e. in its very first reconcile attempt
				observedAnns := observedUn.GetAnnotations()
				if observedAnns == nil || observedAnns[lastAppliedKey] == "" {
					lastApplied := make(map[string]interface{})
					if test.lastApplied != "" {
						// Use last applied state from the test case if specified
						if err := json.Unmarshal(
							[]byte(test.lastApplied), &lastApplied,
						); err != nil {
							t.Fatalf("can't unmarshal lastApplied: %+v", err)
						}
					}
					if len(lastApplied) == 0 {
						// Use observed instance's content as the last applied state
						// if last applied state was not specified in the test case
						lastApplied = observedUn.UnstructuredContent()
					}
					err := dynamicapply.SetLastAppliedByAnnKey(
						observedUn,
						lastApplied,
						lastAppliedKey,
					)
					if err != nil {
						t.Fatalf("can't set last applied to observed: %+v", err)
					}
				}
			}

			// Prepare the desired instance as an unstructured instance
			desiredUn := &unstructured.Unstructured{
				Object: desiredObj,
			}

			// Prepare the want instance as an unstructured instance
			wantUn := &unstructured.Unstructured{
				Object: wantObj,
			}
			// NOTE:
			//	We need to simulate the behaviour of Merge() method
			// to make 'want' same as merge's output i.e. 'got'
			err := dynamicapply.SetLastAppliedByAnnKey(
				wantUn, desiredUn.UnstructuredContent(), lastAppliedKey,
			)
			if err != nil {
				t.Fatalf("can't set last applied to want: %+v", err)
			}

			// Repeat this test case at-least once
			if test.repeatTestCount == 0 {
				test.repeatTestCount = 1
			}
			for count := 0; count < test.repeatTestCount; count++ {
				a := NewApplyFromAnnKey(lastAppliedKey)
				// This is the method under test
				got, err := a.Merge(observedUn, desiredUn)
				if err != nil {
					t.Fatalf("iter %d: can't merge: %+v", count, err)
				}
				if !reflect.DeepEqual(got, wantUn) {
					t.Fatalf(
						"iter %d: got & want are different: a=got, b=want:\n%s",
						count, cmp.Diff(got, wantUn),
					)
				}
				if isDiff, _ := a.HasMergeDiff(); isDiff != test.isDiff && count == 0 {
					// First test iteration can only be verified for differences
					// between observed & desired states. Rest of the iterations
					// should not have any difference due to reconciliation i.e.
					// merge operation.
					t.Fatalf("iter %d: expected diff %t got %t", count, test.isDiff, isDiff)
				}
				// Resulting state i.e. 'got' from this test iteration becomes
				// the new observed state
				//
				// NOTE:
				//	When repeatTestCount is greater than 1; it simulates a
				// continuous reconcile behaviour
				//
				// NOTE:
				//	Desired state is never changed since it should be
				// idempotent for every reconcile
				observedUn = got
			}
		})
	}
}

func TestRevertObjectMetaSystemFields(t *testing.T) {
	observedJSON := `{
		"metadata": {
			"origMeta": "should stay gone",
			"otherMeta": "should change value",
			"creationTimestamp": "should restore orig value",
			"deletionTimestamp": "should restore orig value",
			"uid": "should bring back removed value"
		},
		"other": "should change value"
	}`
	mergedJSON := `{
		"metadata": {
			"creationTimestamp": null,
			"deletionTimestamp": "new value",
			"newMeta": "new value",
			"otherMeta": "new value",
			"selfLink": "should be removed"
		},
		"other": "new value"
	}`
	wantJSON := `{
		"metadata": {
			"otherMeta": "new value",
			"newMeta": "new value",
			"creationTimestamp": "should restore orig value",
			"deletionTimestamp": "should restore orig value",
			"uid": "should bring back removed value"
		},
		"other": "new value"
	}`

	observed := make(map[string]interface{})
	if err := json.Unmarshal([]byte(observedJSON), &observed); err != nil {
		t.Fatalf("can't unmarshal orig: %v", err)
	}
	merged := make(map[string]interface{})
	if err := json.Unmarshal([]byte(mergedJSON), &merged); err != nil {
		t.Fatalf("can't unmarshal newObj: %v", err)
	}
	want := make(map[string]interface{})
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatalf("can't unmarshal want: %v", err)
	}

	err := revertObjectMetaSystemFields(&unstructured.Unstructured{Object: merged}, &unstructured.Unstructured{Object: observed})
	if err != nil {
		t.Fatalf("revertObjectMetaSystemFields error: %v", err)
	}

	if got := merged; !reflect.DeepEqual(got, want) {
		t.Logf("reflect diff: a=got, b=want:\n%s", cmp.Diff(got, want))
		t.Fatalf("revertObjectMetaSystemFields() = %#v, want %#v", got, want)
	}
}
