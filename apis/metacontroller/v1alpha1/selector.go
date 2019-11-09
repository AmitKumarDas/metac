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

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ResourceSelector represents the union of the results of one or more
// queries over a set of selector terms i.e., it represents the
// **OR** of the selector terms.
type ResourceSelector struct {
	// A list of selector terms. This list of terms are ORed.
	SelectorTerms []*SelectorTerm `json:"selectorTerms"`
}

// A SelectorTerm is a query over various match representations.
// The result of match(-es) are ANDed.
type SelectorTerm struct {
	// MatchSlice is a map i.e. key value pairs based slice selector.
	// A single {key,value} in the MatchSlice map is equivalent to
	// an element of matchSliceExpressions, whose key field is "key",
	// the operator is "In", and the value contains array of string
	// values.
	//
	// A key should represent the nested field path of the target under
	// match separated by dot(s) i.e. '.'
	//
	// A MatchSlice is converted into a list of SliceSelectorRequirement
	// that are AND-ed to determine if the selector matches its target or
	// not.
	//
	// This is optional
	MatchSlice map[string][]string `json:"matchSlice"`

	// MatchSliceExpressions is a list of slice selector requirements.
	// These requirements are AND-ed to determine if the selector matches
	// its target or not.
	//
	// The slice selector requirement key should represent the nested
	// field path of the target under match separated by dot(s) i.e. '.'
	//
	// This is optional
	MatchSliceExpressions []SliceSelectorRequirement `json:"matchSliceExpressions"`

	// MatchFields is a map i.e. key value pairs based field selector.
	// A single {key,value} in the MatchFields map is equivalent to an
	// element of matchFieldExpressions, whose key field is "key", the
	// operator is "In", and the value contains only a string value.
	//
	// A key should represent the nested field path of the target under
	// match separated by dot(s) i.e. '.'
	//
	// A MatchFields is converted into a list of LabelSelectorRequirement
	// that are AND-ed to determine if the selector matches its target or
	// not.
	//
	// This is optional
	MatchFields map[string]string `json:"matchFields"`

	// MatchFieldExpressions is a list of field selector requirements.
	// The requirements are AND-ed.
	//
	// The label selector requirement key should represent the nested
	// field path of the target under match separated by dot(s) i.e. '.'
	//
	// This is optional
	MatchFieldExpressions []metav1.LabelSelectorRequirement `json:"matchFieldExpressions"`

	// MatchReference is a list of keys where each key holds the path to a
	// field present in target resource as well as the reference resource.
	// A single key in the MatchReference list is equivalent to an
	// element of MatchReferenceExpressions, whose key field is "key", and
	// the operator is "Equals".
	//
	// A key should represent the nested field path of the target as well
	// watch separated by dot(s) e.g. 'metadata.name'
	//
	// A MatchReference is converted into a list of LabelSelectorRequirement
	// that are AND-ed to determine if the selector marks its target as
	// a match or no match.
	//
	// This is optional
	MatchReference []string `json:"matchWatch"`

	// MatchReferenceExpressions is a list of field selector requirements.
	// The requirements are AND-ed.
	//
	// A label selector requirement key should represent the nested
	// field path of the target separated by dot(s) e.g. 'metadata.uid'
	//
	// This result of each item in this list of LabelSelectorRequirements
	// is AND-ed to determine if the selector marks its target as
	// a match or no match.
	//
	// This is optional
	MatchReferenceExpressions []ReferenceSelectorRequirement `json:"matchWatchExpressions"`

	// MatchLabels is a map of {key,value} pairs. A single {key,value}
	// in the MatchLabels map is equivalent to an element of
	// MatchLabelExpressions, whose key field is "key", the operator is "In",
	// and the value contains a string value. The requirements are AND-ed.
	//
	// The key as well value is matched against the target's labels.
	//
	// This is optional
	MatchLabels map[string]string `json:"matchLabels"`

	// MatchLabelExpressions is a list of label selector requirements.
	// The requirements are ANDed.
	//
	// The label selector requirement's key as well value is matched
	// against the target's labels.
	//
	// This is optional
	MatchLabelExpressions []metav1.LabelSelectorRequirement `json:"matchLabelExpressions"`

	// MatchAnnotations is a map of {key,value} pairs. A single {key,value}
	// in the MatchAnnotations map is equivalent to an element of
	// MatchAnnotationExpressions, whose key field is "key", the operator is
	// "In", and the value contains a string value. The requirements are ANDed.
	//
	// The key as well value is matched against the target's annotations.
	//
	// This is optional
	MatchAnnotations map[string]string `json:"matchAnnotations"`

	// MatchAnnotationExpressions is a list of label selector requirements.
	// The requirements are ANDed.
	//
	// The key as well value is matched against the target's annotations.
	//
	// This is optional
	MatchAnnotationExpressions []metav1.LabelSelectorRequirement `json:"matchAnnotationExpressions"`
}

// SliceSelectorRequirement contains values, a key, and an operator that
// relates the key and values. The zero value of Requirement is invalid.
//
// NOTE:
// 	Requirement implements both set based match and exact match.
//
// NOTE:
// 	Requirement should be initialized via appropriate constructors
// for creating a valid SliceSelectorRequirement.
type SliceSelectorRequirement struct {
	// Key is the target's nested path that the selector applies to
	Key string `json:"key"`

	// Operator represents the key's relationship to a set of values
	Operator SliceSelectorOperator `json:"operator"`

	// Values is an array of string values corresponding to the key
	Values []string `json:"values"`
}

// SliceSelectorOperator is a set of supported operators that is used by
// SliceSelectorRequirement
type SliceSelectorOperator string

const (
	// SliceSelectorOpEquals does a strict equals check
	SliceSelectorOpEquals SliceSelectorOperator = "Equals"

	// SliceSelectorOpNotEquals does a not equals check
	SliceSelectorOpNotEquals SliceSelectorOperator = "NotEquals"

	// SliceSelectorOpIn does a contains check
	SliceSelectorOpIn SliceSelectorOperator = "In"

	// SliceSelectorOpNotIn does a not contains check
	SliceSelectorOpNotIn SliceSelectorOperator = "NotIn"
)

// ReferenceSelectorRequirement contains a key and an operator.
// Operator performs match related operations against key and
// corresponding values. Values are derived from the target
// object and the reference object.
//
// NOTE:
//	Target refers to any arbitrary resource instance whereas
// reference resource refers to the parent / watch resource in
// various meta controllers.
type ReferenceSelectorRequirement struct {
	// Key is the target's nested path that the selector applies to.
	// The nested path is separated by dot(s) e.g. 'metadata.namespace'
	Key string `json:"key"`

	// Operator represents the key's relationship to a string value
	Operator ReferenceSelectorOperator `json:"operator"`
}

// ReferenceSelectorOperator is a set of supported operators that is
// used by ReferenceSelectorRequirement
type ReferenceSelectorOperator string

const (
	// ReferenceSelectorOpEquals does a strict equals check of the
	// target's value against the reference's value. In this case
	// value is derived from the nested path specified in the key.
	ReferenceSelectorOpEquals ReferenceSelectorOperator = "Equals"

	// ReferenceSelectorOpNotEquals does a not equals check of the
	// target's value against the reference's value. In this case
	// value is derived from the nested path specified in the key.
	ReferenceSelectorOpNotEquals ReferenceSelectorOperator = "NotEquals"

	// ReferenceSelectorOpEqualsUID does a strict equals check of
	// the value derived from key against the reference's UID
	ReferenceSelectorOpEqualsUID ReferenceSelectorOperator = "EqualsWatchUID"

	// ReferenceSelectorOpEqualsName does a strict equals check of
	// the value derived from the key against the reference's Name
	ReferenceSelectorOpEqualsName ReferenceSelectorOperator = "EqualsWatchName"

	// ReferenceSelectorOpEqualsNamespace does a strict equals check
	// of the value derived from the key against the reference's
	// Namespace
	ReferenceSelectorOpEqualsNamespace ReferenceSelectorOperator = "EqualsWatchNamespace"
)
