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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"openebs.io/metac/apis/metacontroller/v1alpha1"
)

func TestNewReferenceSelector(t *testing.T) {
	var tests = map[string]struct {
		config                     ReferenceSelectorConfig
		expectOperatorMappingCount int
	}{
		"no reference select terms": {
			config:                     ReferenceSelectorConfig{},
			expectOperatorMappingCount: 6,
		},
	}
	for name, mock := range tests {
		name := name
		mock := mock
		t.Run(name, func(t *testing.T) {
			s := NewReferenceSelector(mock.config)
			if len(s.operatorMapping) != mock.expectOperatorMappingCount {
				t.Fatalf(
					"Expected operator map count %d got %d",
					mock.expectOperatorMappingCount,
					len(s.operatorMapping),
				)
			}
		})
	}
}

func TestReferenceSelectorMatch(t *testing.T) {
	var tests = map[string]struct {
		config    ReferenceSelectorConfig
		target    *unstructured.Unstructured
		reference *unstructured.Unstructured
		isMatch   bool
		isErr     bool
	}{
		"no select terms + nil target + nil ref": {
			config:    ReferenceSelectorConfig{},
			target:    nil,
			reference: nil,
			isMatch:   false,
			isErr:     true,
		},
		"no select terms + not nil target + nil ref": {
			config: ReferenceSelectorConfig{},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			reference: nil,
			isMatch:   false,
			isErr:     true,
		},
		"no select terms + not nil target + not nil ref": {
			config: ReferenceSelectorConfig{},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			isMatch: true,
			isErr:   false,
		},
		"matching label by MatchReference": {
			config: ReferenceSelectorConfig{
				MatchReference: []string{
					"metadata.labels.app",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
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
		"matching special labels by MatchReference": {
			config: ReferenceSelectorConfig{
				MatchReference: []string{
					`metadata.labels.app\.metac\.io/name`,
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"matching special labels by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key: `metadata.labels.app\.metac\.io/name`,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"matching annotations by MatchReference": {
			config: ReferenceSelectorConfig{
				MatchReference: []string{
					"metadata.annotations.app",
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app": "metac",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
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
		"matching special annotations by MatchReference": {
			config: ReferenceSelectorConfig{
				MatchReference: []string{
					`metadata.annotations.app\.metac\.io/name`,
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"matching special annotations by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key: `metadata.annotations.app\.metac\.io/name`,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"matching special labels to annotations by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:    `metadata.labels.app\.metac\.io/name`,
						RefKey: `metadata.annotations.app\.metac\.io/name`,
					},
				},
			},
			target: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"app.metac.io/name": "metac",
						},
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"matching watch spec to attachment name by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						RefKey: "spec.watchName", // watch
						Key:    "metadata.name",  // attachment
					},
				},
			},
			target: &unstructured.Unstructured{ // attachment
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "secret-101",
					},
				},
			},
			reference: &unstructured.Unstructured{ // watch
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"watchName": "secret-101",
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"non matching watch spec to attachment name by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						RefKey:   "spec.watchName", // watch
						Key:      "metadata.name",  // attachment
						Operator: v1alpha1.ReferenceSelectorOpNotEquals,
					},
				},
			},
			target: &unstructured.Unstructured{ // attachment
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "secret-102",
					},
				},
			},
			reference: &unstructured.Unstructured{ // watch
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"watchName": "secret-101",
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"matching watch name to attachment name by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.name", // attachment
						Operator: v1alpha1.ReferenceSelectorOpEqualsName,
					},
				},
			},
			target: &unstructured.Unstructured{ // attachment
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "secret-102",
					},
				},
			},
			reference: &unstructured.Unstructured{ // watch
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "secret-102",
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"non matching watch name to attachment name by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.name", // attachment
						Operator: v1alpha1.ReferenceSelectorOpEqualsName,
					},
				},
			},
			target: &unstructured.Unstructured{ // attachment
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "secret-100",
					},
				},
			},
			reference: &unstructured.Unstructured{ // watch
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "secret-102",
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
		"matching watch namespace to attachment namespace by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.namespace", // attachment
						Operator: v1alpha1.ReferenceSelectorOpEqualsNamespace,
					},
				},
			},
			target: &unstructured.Unstructured{ // attachment
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"namespace": "some",
					},
				},
			},
			reference: &unstructured.Unstructured{ // watch
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"namespace": "some",
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"non matching watch namespace to attachment namespace by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.namespace", // attachment
						Operator: v1alpha1.ReferenceSelectorOpEqualsNamespace,
					},
				},
			},
			target: &unstructured.Unstructured{ // attachment
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"namespace": "something",
					},
				},
			},
			reference: &unstructured.Unstructured{ // watch
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"namespace": "some",
					},
				},
			},
			isMatch: false,
			isErr:   false,
		},
		"matching watch uid to attachment annotations by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.annotations.watch-uid", // attachment
						Operator: v1alpha1.ReferenceSelectorOpEqualsUID,
					},
				},
			},
			target: &unstructured.Unstructured{ // attachment
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"watch-uid": "abc-10101-abd-101911",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{ // watch
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"uid": "abc-10101-abd-101911",
					},
				},
			},
			isMatch: true,
			isErr:   false,
		},
		"non matching watch uid to attachment annotations by MatchReferenceExpressions": {
			config: ReferenceSelectorConfig{
				MatchReferenceExpressions: []v1alpha1.ReferenceSelectorRequirement{
					v1alpha1.ReferenceSelectorRequirement{
						Key:      "metadata.annotations.watch-uid", // attachment
						Operator: v1alpha1.ReferenceSelectorOpEqualsUID,
					},
				},
			},
			target: &unstructured.Unstructured{ // attachment
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"watch-uid": "xxxx-10101-xxx-101911",
						},
					},
				},
			},
			reference: &unstructured.Unstructured{ // watch
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"uid": "abc-10101-abd-101911",
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
			ok, err := NewReferenceSelector(mock.config).Match(
				mock.target,
				mock.reference,
			)
			if mock.isErr && err == nil {
				t.Fatalf("Expected error got none")
			}
			if !mock.isErr && err != nil {
				t.Fatalf("Expected no error got [%+v]", err)
			}
			if ok != mock.isMatch {
				t.Fatalf("Expected match %t got %t", mock.isMatch, ok)
			}
		})
	}
}
