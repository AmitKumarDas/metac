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

package generic

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"openebs.io/metac/apis/metacontroller/v1alpha1"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
)

func TestMakeSelectorKeyFromAVK(t *testing.T) {
	var tests = map[string]struct {
		apiVersion string
		kind       string
		expect     string
	}{
		"valid values": {
			apiVersion: "v1",
			kind:       "Pod",
			expect:     "Pod.v1",
		},
		"missing apiversion": {
			apiVersion: "",
			kind:       "Pod",
			expect:     "Pod.",
		},
		"missing kind": {
			apiVersion: "v1",
			kind:       "",
			expect:     ".v1",
		},
		"missing both": {
			apiVersion: "",
			kind:       "",
			expect:     ".",
		},
	}
	for name, mock := range tests {
		name := name
		mock := mock
		t.Run(name, func(t *testing.T) {
			got := makeSelectorKeyFromAVK(mock.apiVersion, mock.kind)
			if got != mock.expect {
				t.Fatalf("Expected %q got %q", mock.expect, got)
			}
		})
	}
}

func TestSelectorMatchesLAN(t *testing.T) {
	var tests = map[string]struct {
		controllerResource v1alpha1.GenericControllerResource
		apiResource        *dynamicdiscovery.APIResource
		target             *unstructured.Unstructured
		isMatch            bool
		isMatchErr         bool
		isErr              bool
	}{
		"empty controller resource": {
			controllerResource: v1alpha1.GenericControllerResource{},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			isMatch: false,
			isErr:   true,
		},
		"no controller resource matchers": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"nil target": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"app": "metac",
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target:     nil,
			isMatch:    false,
			isMatchErr: true,
			isErr:      false,
		},
		"nil api resource": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"app": "metac",
					},
				},
			},
			apiResource: nil,
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			isMatch: false,
			isErr:   true,
		},
		"match annotations": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"app": "metac",
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"match annotation expressions": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"metac"},
						},
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"no match annotations": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"apps": "metac",
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
		"match labels": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "metac",
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"no match labels": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"apps": "metac",
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
		"match label expressions": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"metac"},
						},
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"match name": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod"}),
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Pod",
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"no match name": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod"}),
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Podie",
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
		"match name && labels": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod"}),
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app":        "metac",
						"controller": "declarative",
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Pod",
						"labels": map[string]interface{}{
							"app":        "metac",
							"controller": "declarative",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"match name && no match labels": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod"}),
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app":  "metac",
						"ctrl": "declarative",
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Pod",
						"labels": map[string]interface{}{
							"app":        "metac",
							"controller": "declarative",
						},
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
		"match name && annotations": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod"}),
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"app":        "metac",
						"controller": "declarative",
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Pod",
						"annotations": map[string]interface{}{
							"app":        "metac",
							"controller": "declarative",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"match name && no match annotations": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod"}),
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"app":  "metac",
						"meta": "controller",
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Pod",
						"annotations": map[string]interface{}{
							"app":        "metac",
							"controller": "declarative",
						},
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
		"match name, labels && annotations": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod"}),
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpExists,
						},
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"metac"},
						},
						metav1.LabelSelectorRequirement{
							Key:      "appie",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
					MatchLabels: map[string]string{
						"app":  "metac",
						"meta": "controller",
					},
				},
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"app":  "metac",
						"meta": "controller",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpExists,
						},
						metav1.LabelSelectorRequirement{
							Key:      "appy",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"none"},
						},
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"metac"},
						},
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Pod",
						"annotations": map[string]interface{}{
							"app":  "metac",
							"meta": "controller",
						},
						"labels": map[string]interface{}{
							"app":  "metac",
							"meta": "controller",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"match name && labels && no match annotations": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod"}),
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpExists,
						},
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"metac"},
						},
					},
					MatchLabels: map[string]string{
						"meta": "controller",
					},
				},
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"app":  "metac",
						"meta": "controller",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpExists,
						},
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"none"},
						},
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Pod",
						"annotations": map[string]interface{}{
							"app":  "metac",
							"meta": "controller",
						},
						"labels": map[string]interface{}{
							"app":  "metac",
							"meta": "controller",
						},
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
		"match name && annotations && no match labels": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod"}),
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpExists,
						},
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"none"},
						},
					},
					MatchLabels: map[string]string{
						"meta": "controller",
					},
				},
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"app":  "metac",
						"meta": "controller",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpExists,
						},
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"none"},
						},
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Pod",
						"annotations": map[string]interface{}{
							"app":  "metac",
							"meta": "controller",
						},
						"labels": map[string]interface{}{
							"app":  "metac",
							"meta": "controller",
						},
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
		"match labels && annotations && no match name": {
			controllerResource: v1alpha1.GenericControllerResource{
				ResourceRule: v1alpha1.ResourceRule{
					APIVersion: "v1",
					Resource:   "pods",
				},
				NameSelector: v1alpha1.NameSelector([]string{"My-Pod-ies"}),
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpExists,
						},
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"none"},
						},
					},
					MatchLabels: map[string]string{
						"meta": "controller",
					},
				},
				AnnotationSelector: &v1alpha1.AnnotationSelector{
					MatchAnnotations: map[string]string{
						"app":  "metac",
						"meta": "controller",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpExists,
						},
						metav1.LabelSelectorRequirement{
							Key:      "app",
							Operator: metav1.LabelSelectorOpNotIn,
							Values:   []string{"none"},
						},
					},
				},
			},
			apiResource: &dynamicdiscovery.APIResource{
				APIVersion: "v1",
				APIResource: metav1.APIResource{
					Kind: "Pod",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "My-Pod",
						"annotations": map[string]interface{}{
							"app":  "metac",
							"meta": "controller",
						},
						"labels": map[string]interface{}{
							"app":  "metac",
							"meta": "controller",
						},
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
	}
	for name, mock := range tests {
		name := name
		mock := mock
		t.Run(name, func(t *testing.T) {
			mgr := &dynamicdiscovery.APIResourceManager{
				GetByResourceFn: func(apiVer, resource string) *dynamicdiscovery.APIResource {
					return mock.apiResource
				},
			}
			s, err := NewSelectorForWatch(mgr, mock.controllerResource)
			if mock.isErr && err == nil {
				t.Fatalf("Expected error got nil")
			}
			if !mock.isErr && err != nil {
				t.Fatalf("Expected no error got [%+v]", err)
			}
			if err == nil {
				match, err := s.MatchLAN(mock.target)
				if mock.isMatchErr && err == nil {
					t.Fatalf("Expected match error got none")
				}
				if !mock.isMatchErr && err != nil {
					t.Fatalf("Expected no match error got [%+v]", err)
				}
				if !mock.isMatchErr && match != mock.isMatch {
					t.Fatalf("Expected match %t got %t", mock.isMatch, match)
				}
			}
		})
	}
}
