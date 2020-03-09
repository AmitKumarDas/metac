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

package selector

import (
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

// replaceValueStrings maps various strings which if
// present in field value(s) to their replacement
//
// NOTE:
//	This ensures the values are valid when matched
// via labels.Selector
//
// NOTE:
//	FieldSelection delegates the match implementation
// to labels.Selector
var replaceValueStrings = map[string]string{
	"/": "fwdslash",
}

// FieldSelection helps in matching field paths against
// given target objects
//
// It piggy backs on label selector with minor changes to
// label selector's logic
type FieldSelection struct {
	selector  labels.Selector
	fieldKeys []string

	err error
}

func (s *FieldSelection) sanitiseValue(given string) string {
	if given == "" {
		return ""
	}
	newvalue := given
	for old, new := range replaceValueStrings {
		newvalue = strings.ReplaceAll(newvalue, old, new)
	}
	return newvalue
}

// sanitiseLabels replaces one or more special chars from
// values with chars that are considered valid by
// labels.Selector
func (s *FieldSelection) sanitiseLabels(lbls *metav1.LabelSelector) *metav1.LabelSelector {
	var newlbls = &metav1.LabelSelector{
		MatchLabels: map[string]string{},
	}
	for key, val := range lbls.MatchLabels {
		newlbls.MatchLabels[key] = s.sanitiseValue(val)
	}
	for _, exp := range lbls.MatchExpressions {
		var oldval string
		var newValues []string
		// there can be empty values for cases when
		// operator is 'Exists' or 'DoesNotExist'
		if len(exp.Values) > 0 {
			// field selector expects a single value
			oldval = exp.Values[0]
			newVal := s.sanitiseValue(oldval)
			newValues = append(newValues, newVal)
		} else {
			newValues = nil
		}
		newlbls.MatchExpressions = append(
			newlbls.MatchExpressions,
			metav1.LabelSelectorRequirement{
				Key:      exp.Key,      // remains same
				Operator: exp.Operator, // remains same
				Values:   newValues,
			},
		)
	}
	return newlbls
}

func (s *FieldSelection) init(lbls *metav1.LabelSelector) {
	for key := range lbls.MatchLabels {
		if key == "" {
			s.err = errors.Errorf(
				"Init failed: Missing field key: %+v",
				lbls.MatchLabels,
			)
			return
		}
		s.fieldKeys = append(s.fieldKeys, key)
	}
	for _, exp := range lbls.MatchExpressions {
		if exp.Key == "" {
			s.err =
				errors.Errorf(
					"Init failed: Missing field expression key: %+v",
					exp,
				)
			return
		}
		s.fieldKeys = append(s.fieldKeys, exp.Key)
	}
	// instantiate the labels.Selector
	s.selector, s.err = metav1.LabelSelectorAsSelector(lbls)
}

// NewFieldSelector returns a new instance of FieldSelection
func NewFieldSelector(lbls *metav1.LabelSelector) *FieldSelection {
	s := &FieldSelection{}
	newlbls := s.sanitiseLabels(lbls)
	s.init(newlbls)
	return s
}

// Match returns true if the given target matches the field
// selection terms
func (s *FieldSelection) Match(target *unstructured.Unstructured) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	// build the target labels
	targetLbls := make(map[string]string)
	for _, key := range s.fieldKeys {
		// we expect the key represents a path to some nested
		// field path in the structure
		fields := strings.Split(key, ".")
		val, found, err := unstructured.NestedString(target.Object, fields...)
		if err != nil {
			return false,
				errors.Wrapf(
					err,
					"Match failed for key %s: %q / %q",
					key,
					target.GetNamespace(),
					target.GetName(),
				)
		}
		if !found {
			// continue by ignoring this key since value is not found
			//
			// NOTE:
			// 	This is helpful for cases where match is being
			// made from 'Exists' or 'DoesNotExist' operator
			continue
		}
		newVal := s.sanitiseValue(val)
		targetLbls[key] = newVal
	}
	// at this point field expressions are made same as label expressions
	return s.selector.Matches(labels.Set(targetLbls)), nil
}
