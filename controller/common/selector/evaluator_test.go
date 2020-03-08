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

package selector

import (
	"testing"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"openebs.io/metac/apis/metacontroller/v1alpha1"
)

func TestEvalIsAnnotationMatch(t *testing.T) {
	tests := map[string]struct {
		selector v1alpha1.SelectorTerm
		target   *unstructured.Unstructured
		isError  bool
		isMatch  bool
	}{
		"Empty selector": {
			selector: v1alpha1.SelectorTerm{},
			target:   &unstructured.Unstructured{},
			isError:  false,
			isMatch:  true,
		},
		"Empty selector && nil target": {
			selector: v1alpha1.SelectorTerm{},
			target:   nil,
			isError:  false,
			isMatch:  true,
		},
		"MatchAnnotations selector && nil target": {
			selector: v1alpha1.SelectorTerm{
				MatchAnnotations: map[string]string{
					"app": "trial",
				},
			},
			target:  nil,
			isError: true,
			isMatch: false,
		},
		"MatchAnnotations selector && matching annotations": {
			selector: v1alpha1.SelectorTerm{
				MatchAnnotations: map[string]string{
					"app": "trial",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match && Expression annotation selector && matching annotations - 1": {
			selector: v1alpha1.SelectorTerm{
				MatchAnnotations: map[string]string{
					"app": "trial",
				},
				MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"it"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match && Expression annotation selector + matching annotations + exhaustive": {
			selector: v1alpha1.SelectorTerm{
				MatchAnnotations: map[string]string{
					"do":  "it",
					"app": "trial",
				},
				MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "nope",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "hulla",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "hulla",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh", "it", "trial"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh", "it"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh", "it", "trial"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match && Expression annotation selector + matching annotations + exhaustive + json": {
			selector: v1alpha1.SelectorTerm{
				MatchAnnotations: map[string]string{
					"do":  "it",
					"app": "trial",
				},
				MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "nope",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "hulla",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "hulla",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh", "it", "trial"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh", "it"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh", "it", "trial"},
					},
				},
			},
			target: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"annotations": {
							"app": "trial",
							"do": "it"
						}
					}
				}
			`),
			isError: false,
			isMatch: true,
		},
		"MatchAnnotations selector && non matching annotations": {
			selector: v1alpha1.SelectorTerm{
				MatchAnnotations: map[string]string{
					"donot": "doit",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Invalid MatchAnnotationExpression selector && matching annotations": {
			selector: v1alpha1.SelectorTerm{
				MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"it"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"MatchAnnotationExpression selector && match & non match annotations": {
			selector: v1alpha1.SelectorTerm{
				MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "hulla",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
	}
	for name, mock := range tests {
		name := name // pin it
		mock := mock // pin it
		t.Run(name, func(t *testing.T) {
			e := &Evaluation{
				Target: mock.target,
			}
			match, err := e.isAnnotationMatch(mock.selector)
			if mock.isError && err == nil {
				t.Fatalf("%s: Expected error: Got none", name)
			}
			if mock.isMatch && !match {
				t.Fatalf("%s: Expected match: Got no match", name)
			}

		})
	}
}

func TestEvalIsLabelMatch(t *testing.T) {
	tests := map[string]struct {
		selector v1alpha1.SelectorTerm
		target   *unstructured.Unstructured
		isError  bool
		isMatch  bool
	}{
		"Empty selector": {
			selector: v1alpha1.SelectorTerm{},
			target:   &unstructured.Unstructured{},
			isError:  false,
			isMatch:  true,
		},
		"Empty selector && nil target": {
			selector: v1alpha1.SelectorTerm{},
			target:   nil,
			isError:  false,
			isMatch:  true,
		},
		"MatchLabels selector && nil target": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"app": "trial",
				},
			},
			target:  nil,
			isError: true,
			isMatch: false,
		},
		"MatchLabels selector && matching labels": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"app": "trial",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchLabels selector && matching labels && value has -": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"app": "trial-1",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial-1",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchLabels selector && matching labels && special char .": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"app.io": "trial",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.io": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchLabels selector && matching labels && special char /": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"app/io": "trial",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app/io": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchLabels selector && matching labels && special char / and .": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"app.io/type": "trial",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.io/type": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match && Expression label selector && matching labels": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"app": "trial",
				},
				MatchLabelExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"it"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match && Expression label selector + matching labels + exhaustive": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"do":  "it",
					"app": "trial",
				},
				MatchLabelExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "nope",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "hulla",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh", "it"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh", "trial"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match && Expression label selector + matching labels + exhaustive + json": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"do":  "it",
					"app": "trial",
				},
				MatchLabelExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "nope",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "hulla",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh", "it"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh", "trial"},
					},
				},
			},
			target: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"labels": {
							"app": "trial",
							"do":  "it"
						}
					}
				}
			`),
			isError: false,
			isMatch: true,
		},
		"MatchLabels selector && non matching labels": {
			selector: v1alpha1.SelectorTerm{
				MatchLabels: map[string]string{
					"donot": "doit",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Invalid MatchLabelExpressions selector && matching labels": {
			selector: v1alpha1.SelectorTerm{
				MatchLabelExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "do",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"it"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"MatchLabelExpressions selector && match & non match labels": {
			selector: v1alpha1.SelectorTerm{
				MatchLabelExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "hulla",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"MatchLabelExpressions selector + match & non match labels + json": {
			selector: v1alpha1.SelectorTerm{
				MatchLabelExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "app",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "hulla",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh"},
					},
				},
			},
			target: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"labels": {
							"app": "trial",
							"do":  "it"
						}
					}
				}
			`),
			isError: false,
			isMatch: false,
		},
	}
	for name, mock := range tests {
		name := name // pin it
		mock := mock // pin it
		t.Run(name, func(t *testing.T) {
			e := &Evaluation{
				Target: mock.target,
			}
			match, err := e.isLabelMatch(mock.selector)
			if mock.isError && err == nil {
				t.Fatalf("%s: Expected error: Got none", name)
			}
			if !mock.isError && err != nil {
				t.Fatalf("%s: Expected no error: Got %v", name, err)
			}
			if mock.isMatch && !match {
				t.Fatalf("%s: Expected match: Got no match", name)
			}
			if !mock.isMatch && match {
				t.Fatalf("%s: Expected no match: Got match", name)
			}
		})
	}
}

func TestEvalIsSliceMatchFinalizers(t *testing.T) {
	tests := map[string]struct {
		selector v1alpha1.SelectorTerm
		target   *unstructured.Unstructured
		isError  bool
		isMatch  bool
	}{
		"Match two finalizers of three": {
			selector: v1alpha1.SelectorTerm{
				MatchSlice: map[string][]string{
					"metadata.finalizers": []string{
						"pvc-protect",
						"app-protect",
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"finalizers": []interface{}{
							"pvc-protect",
							"storage-protect",
							"app-protect",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Match one finalizer of three": {
			selector: v1alpha1.SelectorTerm{
				MatchSlice: map[string][]string{
					"metadata.finalizers": []string{
						"pvc-protect",
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"finalizers": []interface{}{
							"pvc-protect",
							"storage-protect",
							"app-protect",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Match all finalizers": {
			selector: v1alpha1.SelectorTerm{
				MatchSlice: map[string][]string{
					"metadata.finalizers": []string{
						"pvc-protect",
						"storage-protect",
						"app-protect",
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"finalizers": []interface{}{
							"pvc-protect",
							"storage-protect",
							"app-protect",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match all finalizers + In operator": {
			selector: v1alpha1.SelectorTerm{
				MatchSliceExpressions: []v1alpha1.SliceSelectorRequirement{
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpIn,
						Values:   []string{"pvc-protect", "storage-protect", "app-protect"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"finalizers": []interface{}{
							"pvc-protect",
							"storage-protect",
							"app-protect",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match all finalizers + Equals operator": {
			selector: v1alpha1.SelectorTerm{
				MatchSliceExpressions: []v1alpha1.SliceSelectorRequirement{
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpEquals,
						Values:   []string{"pvc-protect", "storage-protect", "app-protect"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"finalizers": []interface{}{
							"pvc-protect",
							"storage-protect",
							"app-protect",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match no finalizers + NotIn operator": {
			selector: v1alpha1.SelectorTerm{
				MatchSliceExpressions: []v1alpha1.SliceSelectorRequirement{
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotIn,
						Values:   []string{"invalid-protect"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"finalizers": []interface{}{
							"pvc-protect",
							"storage-protect",
							"app-protect",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match no finalizers + NotEquals operator": {
			selector: v1alpha1.SelectorTerm{
				MatchSliceExpressions: []v1alpha1.SliceSelectorRequirement{
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotEquals,
						Values:   []string{"invalid-protect"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"finalizers": []interface{}{
							"pvc-protect",
							"storage-protect",
							"app-protect",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match all finalizers + All operators": {
			selector: v1alpha1.SelectorTerm{
				MatchSliceExpressions: []v1alpha1.SliceSelectorRequirement{
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpEquals,
						Values:   []string{"pvc-protect", "storage-protect", "app-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotEquals,
						Values:   []string{"unknown", "pvc-protect", "storage-protect", "app-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpIn,
						Values:   []string{"pvc-protect", "storage-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpIn,
						Values:   []string{"pvc-protect", "storage-protect", "app-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotIn,
						Values:   []string{"unknown-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotEquals,
						Values:   []string{"unknown-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotIn,
						Values:   []string{"unknown-protect", "storage-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotEquals,
						Values:   []string{"unknown-protect", "storage-protect"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"finalizers": []interface{}{
							"pvc-protect",
							"storage-protect",
							"app-protect",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match all finalizers + all operators + json": {
			selector: v1alpha1.SelectorTerm{
				MatchSliceExpressions: []v1alpha1.SliceSelectorRequirement{
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpEquals,
						Values:   []string{"pvc-protect", "storage-protect", "app-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotEquals,
						Values:   []string{"unknown", "pvc-protect", "storage-protect", "app-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpIn,
						Values:   []string{"pvc-protect", "storage-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpIn,
						Values:   []string{"pvc-protect", "storage-protect", "app-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotIn,
						Values:   []string{"unknown-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotEquals,
						Values:   []string{"unknown-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotIn,
						Values:   []string{"unknown-protect", "storage-protect"},
					},
					v1alpha1.SliceSelectorRequirement{
						Key:      "metadata.finalizers",
						Operator: v1alpha1.SliceSelectorOpNotEquals,
						Values:   []string{"unknown-protect", "storage-protect"},
					},
				},
			},
			target: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"finalizers": [
							"pvc-protect",
							"storage-protect",
							"app-protect"
						]
					}
				}
			`),
			isError: false,
			isMatch: true,
		},
	}
	for name, mock := range tests {
		name := name // pin it
		mock := mock // pin it
		t.Run(name, func(t *testing.T) {
			e := &Evaluation{
				Target: mock.target,
			}
			match, err := e.isSliceMatch(mock.selector)
			if mock.isError && err == nil {
				t.Fatalf("%s: Expected error: Got none", name)
			}
			if !mock.isError && err != nil {
				t.Fatalf("%s: Expected no error: Got %v", name, err)
			}
			if mock.isError {
				return
			}
			if mock.isMatch && !match {
				t.Fatalf("%s: Expected match: Got no match", name)
			}
			if !mock.isMatch && match {
				t.Fatalf("%s: Expected no match: Got match", name)
			}
		})
	}
}

func TestEvalIsFieldMatch(t *testing.T) {
	tests := map[string]struct {
		selector v1alpha1.SelectorTerm
		target   *unstructured.Unstructured
		isError  bool
		isMatch  bool
	}{
		"Empty selector": {
			selector: v1alpha1.SelectorTerm{},
			target:   &unstructured.Unstructured{},
			isError:  false,
			isMatch:  true,
		},
		"Empty selector && nil target": {
			selector: v1alpha1.SelectorTerm{},
			target:   nil,
			isError:  false,
			isMatch:  true,
		},
		"MatchFields selector && nil target": {
			selector: v1alpha1.SelectorTerm{
				MatchFields: map[string]string{
					"metadata.annotations.app": "trial",
				},
			},
			target:  nil,
			isError: true,
			isMatch: false,
		},
		"MatchFields selector && matching target": {
			selector: v1alpha1.SelectorTerm{
				MatchFields: map[string]string{
					"spec.path": "my-path",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"path": "my-path",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields selector && matching target && value with /": {
			selector: v1alpha1.SelectorTerm{
				MatchFields: map[string]string{
					"spec.path": "/dev/sdb",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"path": "/dev/sdb",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields selector && matching target && value with numbers": {
			selector: v1alpha1.SelectorTerm{
				MatchFields: map[string]string{
					"spec.path": "1234",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"path": "1234",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields selector && matching target && value is like uuid": {
			selector: v1alpha1.SelectorTerm{
				MatchFields: map[string]string{
					"spec.path": "1234-1232-asdfssed-12esee-212",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"path": "1234-1232-asdfssed-12esee-212",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields selector && matching fields": {
			selector: v1alpha1.SelectorTerm{
				MatchFields: map[string]string{
					"metadata.labels.app":     "trial",
					"metadata.annotations.do": "it",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
						"annotations": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match && Expression field selector && matching fields - 1": {
			selector: v1alpha1.SelectorTerm{
				MatchFields: map[string]string{
					"metadata.labels.app": "trial",
				},
				MatchFieldExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.app",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"it"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Match && Expression field selector && matching fields - Exhaustive": {
			selector: v1alpha1.SelectorTerm{
				MatchFields: map[string]string{
					"metadata.labels.do":  "it",
					"metadata.labels.app": "trial",
				},
				MatchFieldExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.app",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: metav1.LabelSelectorOpExists,
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.nope",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.hulla",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"huh"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"trial"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"trial", "it", "itsss"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.app",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"trialsss"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"it"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"it", "trial", "trialsss"},
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"ittt"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields selector && non matching fields": {
			selector: v1alpha1.SelectorTerm{
				MatchFields: map[string]string{
					"metadata.labels.donot": "doit",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Invalid MatchFieldExpressions selector && matching fields": {
			selector: v1alpha1.SelectorTerm{
				MatchFieldExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"it"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"MatchFieldExpressions selector && match & non match fields": {
			selector: v1alpha1.SelectorTerm{
				MatchFieldExpressions: []metav1.LabelSelectorRequirement{
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.app",
						Operator: metav1.LabelSelectorOpDoesNotExist,
					},
					metav1.LabelSelectorRequirement{
						Key:      "metadata.labels.hulla",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"huh"},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
	}
	for name, mock := range tests {
		name := name // pin it
		mock := mock // pin it
		t.Run(name, func(t *testing.T) {
			e := &Evaluation{
				Target: mock.target,
			}
			match, err := e.isFieldMatch(mock.selector)
			if mock.isError && err == nil {
				t.Fatalf("%s: Expected error: Got none", name)
			}
			if !mock.isError && err != nil {
				t.Fatalf("%s: Expected no error: Got %v", name, err)
			}
			if mock.isMatch && !match {
				t.Fatalf("%s: Expected match: Got no match", name)
			}
			if !mock.isMatch && match {
				t.Fatalf("%s: Expected no match: Got match", name)
			}
		})
	}
}

func TestEvalIsReferenceMatch(t *testing.T) {
	tests := map[string]struct {
		selector  v1alpha1.SelectorTerm
		target    *unstructured.Unstructured
		reference *unstructured.Unstructured
		isError   bool
		isMatch   bool
	}{
		"Empty selector": {
			selector:  v1alpha1.SelectorTerm{},
			target:    &unstructured.Unstructured{},
			reference: &unstructured.Unstructured{},
			isError:   false,
			isMatch:   true,
		},
		"Empty selector + nil target + not nil reference": {
			selector:  v1alpha1.SelectorTerm{},
			target:    nil,
			reference: &unstructured.Unstructured{},
			isError:   false,
			isMatch:   true,
		},
		"Empty selector + not nil target + nil reference": {
			selector:  v1alpha1.SelectorTerm{},
			target:    &unstructured.Unstructured{},
			reference: nil,
			isError:   false,
			isMatch:   true,
		},
		"Not nil selector + nil target + nil reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.name", "metadata.namespace",
				},
			},
			target:    nil,
			reference: nil,
			isError:   true,
			isMatch:   false,
		},
		"Not nil selector + not nil target + nil reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.name", "metadata.namespace",
				},
			},
			target:    &unstructured.Unstructured{},
			reference: nil,
			isError:   true,
			isMatch:   false,
		},
		"Not nil selector + nil target + not nil reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.name", "metadata.namespace",
				},
			},
			target:    nil,
			reference: &unstructured.Unstructured{},
			isError:   true,
			isMatch:   false,
		},
		"Selector + matching target + matching reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.app",
					"metadata.annotations.do",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
						"annotations": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
						"annotations": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector + matching target + matching reference + json": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.app",
					"metadata.annotations.do",
				},
			},
			target: unstructuredTestObjFromJSONStr(`
			{
				"metadata": {
				  "labels": {
					  "app": "trial"
				  },
				  "annotations": {
					  "do": "it"
				  }
				}
			}
			`),
			reference: unstructuredTestObjFromJSONStr(`
			{
				"metadata": {
				  "labels": {
					  "app": "trial"
				  },
				  "annotations": {
					  "do": "it"
				  }
				}
			}
			`),
			isError: false,
			isMatch: true,
		},
		"Selector + matching target + non matching reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.app",
					"metadata.annotations.do",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
						"annotations": map[string]interface{}{
							"do": "it",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
						"annotations": map[string]interface{}{
							"do": "it-nah",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Selector expressions + matching target + matching reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.app",
				},
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: v1alpha1.ReferenceSelectorOpEquals,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector expressions + not matching target + matching reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.app",
				},
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: v1alpha1.ReferenceSelectorOpEquals,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trialsss",
							"do":  "it",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Selector expressions + matching target + not matching reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.app",
				},
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: v1alpha1.ReferenceSelectorOpEquals,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it-nah",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Negative selector expressions + matching target + matching reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.app",
				},
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: v1alpha1.ReferenceSelectorOpEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.dont",
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector reference + non matching name": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.name",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "i-am-target",
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "i-am-reference",
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Selector reference + matching name": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.name",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "my-name",
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "my-name",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector reference + matching name + cross": {
			selector: v1alpha1.SelectorTerm{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "spec.refer-name",
						Operator: v1alpha1.ReferenceSelectorOpEqualsName,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "my-name",
					},
					"spec": map[string]interface{}{
						"refer-name": "ref-name",
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "ref-name",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector reference + matching namespace + cross": {
			selector: v1alpha1.SelectorTerm{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "spec.refer-namespace",
						Operator: v1alpha1.ReferenceSelectorOpEqualsNamespace,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "my-name",
					},
					"spec": map[string]interface{}{
						"refer-namespace": "defaulte",
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "ref-name",
						"namespace": "defaulte",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector reference + matching uid + cross": {
			selector: v1alpha1.SelectorTerm{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "spec.refer-uid",
						Operator: v1alpha1.ReferenceSelectorOpEqualsUID,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "my-name",
					},
					"spec": map[string]interface{}{
						"refer-uid": "uid-001-001",
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "ref-name",
						"uid":  "uid-001-001",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector reference + non matching namespace": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.namespace",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"namespace": "i-am-target",
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"namespace": "i-am-reference",
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Selector reference + matching namespace": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.namespace",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"namespace": "my-ns",
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"namespace": "my-ns",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector reference + non matching uid": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.uid",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"uid": "target-001",
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"uid": "reference-001",
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"Selector reference + matching uid": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.uid",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"uid": "name-001",
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"uid": "name-001",
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"All selector expressions + matching target with extras + matching reference": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.app",
				},
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: v1alpha1.ReferenceSelectorOpEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.dont",
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app":   "trial",
							"do":    "it",
							"hello": "howdy",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"All selector expressions + matching target + matching reference with extras": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.app",
				},
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.do",
						Operator: v1alpha1.ReferenceSelectorOpEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.dont",
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app":   "trial",
							"do":    "it",
							"hello": "howdy",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector reference + matching target + matching reference + Exhaustive": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.do",
					"metadata.labels.app",
					"metadata.annotations.kool",
					"metadata.name",
					"metadata.namespace",
					"metadata.uid",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test",
						"uid":       "test-10101-10101",
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
						"annotations": map[string]interface{}{
							"kool": "aid",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test",
						"uid":       "test-10101-10101",
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
						"annotations": map[string]interface{}{
							"kool": "aid",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"Selector expressions + matching target + matching reference + exhaustive": {
			selector: v1alpha1.SelectorTerm{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.app",
						Operator: v1alpha1.ReferenceSelectorOpEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.noapp",
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.annotations.noapp",
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.name",
						Operator: v1alpha1.ReferenceSelectorOpEqualsName,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.namespace",
						Operator: v1alpha1.ReferenceSelectorOpEqualsNamespace,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.uid",
						Operator: v1alpha1.ReferenceSelectorOpEqualsUID,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test",
						"uid":       "test-10101-10101",
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
						"annotations": map[string]interface{}{
							"kool": "aid",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test",
						"uid":       "test-10101-10101",
						"labels": map[string]interface{}{
							"app": "trial",
							"do":  "it",
						},
						"annotations": map[string]interface{}{
							"kool": "aid",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"All selector + matching target + matching reference + exhaustive": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.do",
					"metadata.labels.app",
					"metadata.annotations.kool",
					"metadata.name",
					"metadata.namespace",
					"metadata.uid",
				},
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.app",
						Operator: v1alpha1.ReferenceSelectorOpEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.noapp",
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.annotations.noapp",
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.name",
						Operator: v1alpha1.ReferenceSelectorOpEqualsName,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.namespace",
						Operator: v1alpha1.ReferenceSelectorOpEqualsNamespace,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.uid",
						Operator: v1alpha1.ReferenceSelectorOpEqualsUID,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test",
						"uid":       "test-10101-10101",
						"labels": map[string]interface{}{
							"app":   "trial",
							"do":    "it",
							"extra": "i-am-target",
						},
						"annotations": map[string]interface{}{
							"kool":  "aid",
							"extra": "i-am-target",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "test",
						"namespace": "test",
						"uid":       "test-10101-10101",
						"labels": map[string]interface{}{
							"app":   "trial",
							"do":    "it",
							"extra": "i-am-reference",
						},
						"annotations": map[string]interface{}{
							"kool":  "aid",
							"extra": "i-am-reference",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"All selector + matching target + matching reference + exhaustive + json": {
			selector: v1alpha1.SelectorTerm{
				MatchReference: []string{
					"metadata.labels.do",
					"metadata.labels.app",
					"metadata.annotations.kool",
					"metadata.name",
					"metadata.namespace",
					"metadata.uid",
				},
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.app",
						Operator: v1alpha1.ReferenceSelectorOpEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.labels.noapp",
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.annotations.noapp",
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.name",
						Operator: v1alpha1.ReferenceSelectorOpEqualsName,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.namespace",
						Operator: v1alpha1.ReferenceSelectorOpEqualsNamespace,
					},
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.uid",
						Operator: v1alpha1.ReferenceSelectorOpEqualsUID,
					},
				},
			},
			target: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"name": "test",
						"namespace": "test",
						"uid": "test-10101-10101",
						"labels": {
							"app":   "trial",
							"do":    "it",
							"extra": "i-am-target"
						},
						"annotations": {
							"kool":  "aid",
							"extra": "i-am-target"
						}
					}
				}
			`),
			reference: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"name": "test",
						"namespace": "test",
						"uid": "test-10101-10101",
						"labels": {
							"app":   "trial",
							"do":    "it",
							"extra": "i-am-target"
						},
						"annotations": {
							"kool":  "aid",
							"extra": "i-am-target"
						}
					}
				}
			`),
			isError: false,
			isMatch: true,
		},
	}
	for name, mock := range tests {
		name := name // pin it
		mock := mock // pin it
		t.Run(name, func(t *testing.T) {
			e := &Evaluation{
				Target:    mock.target,
				Reference: mock.reference,
			}
			match, err := e.isReferenceMatch(mock.selector)
			if mock.isError && err == nil {
				t.Fatalf("%s: Expected error: Got none", name)
			}
			if !mock.isError && err != nil {
				t.Fatalf("%s: Expected no error: Got %v", name, err)
			}
			if mock.isMatch && !match {
				t.Fatalf("%s: Expected match: Got no match", name)
			}
			if !mock.isMatch && match {
				t.Fatalf("%s: Expected no match: Got match", name)
			}
		})
	}
}

func TestEvalIsMatch(t *testing.T) {
	tests := map[string]struct {
		terms     []*v1alpha1.SelectorTerm
		target    *unstructured.Unstructured
		reference *unstructured.Unstructured
		isError   bool
		isMatch   bool
	}{
		"Empty selector": {
			terms:   []*v1alpha1.SelectorTerm{},
			target:  &unstructured.Unstructured{},
			isError: false,
			isMatch: true,
		},
		"Empty selector && nil target": {
			terms:   []*v1alpha1.SelectorTerm{},
			target:  nil,
			isError: false,
			isMatch: true,
		},
		"MatchAnnotations selector && nil target": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchAnnotations: map[string]string{
						"app": "trial",
					},
				},
			},
			target:  nil,
			isError: true,
			isMatch: false,
		},
		"MatchLabels selector && nil target": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trial",
					},
				},
			},
			target:  nil,
			isError: true,
			isMatch: false,
		},
		"MatchFields selector && nil target": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trial",
					},
				},
			},
			target:  nil,
			isError: true,
			isMatch: false,
		},
		"MatchAnnotations selector && matching annotations": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchAnnotations: map[string]string{
						"app": "trial",
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchLabels selector && matching annotations": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trial",
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields selector && matching annotations": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trial",
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields (T) || MatchLabels (F) selector": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trial",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trialss",
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields (F) || MatchLabels (T) selector": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trial",
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields (F) || MatchLabels (F) || MatchAnnotations (T) selector": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotations: map[string]string{
						"app": "trial",
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
						"annotations": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields (All F) || MatchLabels (All F) || MatchAnnotations (One T) selector": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabelExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotations: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"trial"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trial"},
						},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
						"annotations": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: true,
		},
		"MatchFields (All F) || MatchLabels (All F) || MatchAnnotations (All F) selector": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabelExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotations: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"trial"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "jumbo",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "trial",
						},
						"annotations": map[string]interface{}{
							"app": "trial",
						},
					},
				},
			},
			isError: false,
			isMatch: false,
		},
		"MatchFields (All F) + MatchLabels (All F) + MatchAnnotations (All F) + json": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabelExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotations: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"trial"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "jumbo",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
			},
			target: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"labels": {
							"app": "trial"
						},
						"annotations": {
							"app": "trial"
						}
					}
				}
			`),
			isError: false,
			isMatch: false,
		},
		"MatchFields (All F) + MatchLabels (All F) + MatchAnnotations (All T) + json": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabelExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchAnnotations: map[string]string{
						"app": "trial",
					},
					MatchAnnotationExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trial"},
						},
						metav1.LabelSelectorRequirement{
							Key:      "jumbo",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
			},
			target: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"labels": {
							"app": "trial"
						},
						"annotations": {
							"app": "trial"
						}
					}
				}
			`),
			isError: false,
			isMatch: true,
		},
		"MatchFields (All F) + MatchLabels (All F) + MatchSlice (All T) + json": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchFields: map[string]string{
						"metadata.labels.app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchFieldExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabelExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchSlice: map[string][]string{
						"metadata.finalizers": []string{
							"abc-protect",
							"def-protect",
							"xyz-protect",
						},
					},
					MatchSliceExpressions: []v1alpha1.SliceSelectorRequirement{
						v1alpha1.SliceSelectorRequirement{
							Key:      "metadata.finalizers",
							Operator: v1alpha1.SliceSelectorOpIn,
							Values:   []string{"abc-protect", "xyz-protect"},
						},
						v1alpha1.SliceSelectorRequirement{
							Key:      "jumbo",
							Operator: v1alpha1.SliceSelectorOpNotIn,
							Values:   []string{"dunno-protect", "who-protect"},
						},
						v1alpha1.SliceSelectorRequirement{
							Key:      "metadata.finalizers",
							Operator: v1alpha1.SliceSelectorOpNotIn,
							Values:   []string{"abc-protect", "xyz-protect", "def-protect", "dunno-protect"},
						},
					},
				},
			},
			target: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"labels": {
							"app": "trial"
						},
						"annotations": {
							"app": "trial"
						},
						"finalizers": [
							"abc-protect",
							"def-protect",
							"xyz-protect"
						]
					}
				}
			`),
			isError: false,
			isMatch: true,
		},
		"MatchLabels (All F) + MatchSlice (All F) + MatchReference (All T) + json": {
			terms: []*v1alpha1.SelectorTerm{
				&v1alpha1.SelectorTerm{
					MatchLabels: map[string]string{
						"app": "trialss",
					},
				},
				&v1alpha1.SelectorTerm{
					MatchLabelExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"trialss"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchSlice: map[string][]string{
						"metadata.finalizers": []string{
							"abc-protect",
							"def-protect",
							"xyz-protectsss",
						},
					},
					MatchSliceExpressions: []v1alpha1.SliceSelectorRequirement{
						v1alpha1.SliceSelectorRequirement{
							Key:      "metadata.finalizers",
							Operator: v1alpha1.SliceSelectorOpEquals,
							Values:   []string{"abc-protect", "xyz-protect"},
						},
						v1alpha1.SliceSelectorRequirement{
							Key:      "jumbo",
							Operator: v1alpha1.SliceSelectorOpIn,
							Values:   []string{"dunno-protect", "who-protect"},
						},
						v1alpha1.SliceSelectorRequirement{
							Key:      "metadata.finalizers",
							Operator: v1alpha1.SliceSelectorOpNotIn,
							Values:   []string{"xyz-protect", "def-protect", "dunno-protect"},
						},
					},
				},
				&v1alpha1.SelectorTerm{
					MatchReference: []string{
						"metadata.name",
						"metadata.namespace",
						"metadata.uid",
						"metadata.labels.app",
						"metadata.annotations.app",
					},
					MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
						v1alpha1.ReferenceSelectorRequirement{
							Key:      "metadata.name",
							Operator: v1alpha1.ReferenceSelectorOpEquals,
						},
						v1alpha1.ReferenceSelectorRequirement{
							Key:      "metadata.namespace",
							Operator: v1alpha1.ReferenceSelectorOpEquals,
						},
						v1alpha1.ReferenceSelectorRequirement{
							Key:      "metadata.uid",
							Operator: v1alpha1.ReferenceSelectorOpEquals,
						},
						v1alpha1.ReferenceSelectorRequirement{
							Key:      "metadata.labels.app",
							Operator: v1alpha1.ReferenceSelectorOpEquals,
						},
						v1alpha1.ReferenceSelectorRequirement{
							Key:      "metadata.annotations.app",
							Operator: v1alpha1.ReferenceSelectorOpEquals,
						},
					},
				},
			},
			target: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"name": "my-name",
						"namespace": "my-namespace",
						"uid": "my-001",
						"labels": {
							"app": "trial"
						},
						"annotations": {
							"app": "trial"
						},
						"finalizers": [
							"abc-protect",
							"def-protect",
							"xyz-protect"
						]
					}
				}
			`),
			reference: unstructuredTestObjFromJSONStr(`
				{
					"metadata": {
						"name": "my-name",
						"namespace": "my-namespace",
						"uid": "my-001",
						"labels": {
							"app": "trial"
						},
						"annotations": {
							"app": "trial"
						},
						"finalizers": [
							"abc-protect",
							"def-protect",
							"xyz-protect"
						]
					}
				}
			`),
			isError: false,
			isMatch: true,
		},
	}
	for name, mock := range tests {
		name := name // pin it
		mock := mock // pin it
		t.Run(name, func(t *testing.T) {
			e := Evaluation{
				Terms:     mock.terms,
				Target:    mock.target,
				Reference: mock.reference,
			}
			match, err := e.RunMatch()
			if mock.isError && err == nil {
				t.Fatalf("%s: Expected error: Got none", name)
			}
			if !mock.isError && err != nil {
				t.Fatalf("%s: Expected no error: Got %v", name, err)
			}
			if mock.isMatch && !match {
				t.Fatalf("%s: Expected match: Got no match", name)
			}
			if !mock.isMatch && match {
				t.Fatalf("%s: Expected no match: Got match", name)
			}
		})
	}
}

// unstructuredTestObjFromJSONStr creates a new Unstructured instance
// from the given JSON string.
//
// NOTE:
// 	It panics on a decode error because it's meant for use with hard-coded
// test data.
//
// NOTE:
//	This is meant to be used in _test.go file(s) only
func unstructuredTestObjFromJSONStr(jsonStr string) *unstructured.Unstructured {
	obj := map[string]interface{}{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
		panic(errors.Wrapf(err, "Unmarshal failed: Json string %s", jsonStr))
	}
	u := &unstructured.Unstructured{Object: obj}
	return u
}
