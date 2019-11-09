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
	"github.com/pkg/errors"
	"openebs.io/metac/apis/metacontroller/v1alpha1"
)

// KeyToSlice is a map of key to slice of string
// values
type KeyToSlice map[string][]string

// Get returns the values corresponding to the
// given key. It returns an empty slice if key
// is not found.
func (s KeyToSlice) Get(key string) []string {
	for k, vals := range s {
		if k == key {
			return vals
		}
	}
	return []string{}
}

// Has returns true if given key is available
func (s KeyToSlice) Has(key string) bool {
	for k := range s {
		if k == key {
			return true
		}
	}
	return false
}

// SliceMatcher evaluates a SliceSelectorRequirement. In other
// words it returns if the match succeeded or failed for a slice
// expression.
//
// This should be initialized via NewSliceSelectorMatcher
// constructor for creating a valid matcher instance.
type SliceMatcher struct {
	// key is the label key that the selector applies to.
	Key string

	// operator represents a key's relationship to a set of values.
	Operator v1alpha1.SliceSelectorOperator

	// Store the desired values
	//
	// NOTE:
	//	Use cases will involve verifying the existence or
	// non existence of these desired value(s) based on the chosen
	// operator.
	DesiredValues []string
}

func (r SliceMatcher) String() string {
	return "Slice matcher"
}

// NewSliceMatcher returns a new instance of SliceSelectorMatcher.
//
// If any of these rules is violated, an error is returned:
//
// (1) The operator can only be In, NotIn, Equals, or NotEquals.
// (2) For all the operators, the key & values that are set must be non-empty.
// (3) The key or value is invalid due to its length, or sequence of characters.
// 	See validateLabelKey & validateLabelValue for more details.
//
// NOTE:
// 	The empty string is a valid value in the input values set.
func NewSliceMatcher(key string, op v1alpha1.SliceSelectorOperator, desiredValues []string) (*SliceMatcher, error) {
	sm := &SliceMatcher{}
	if key == "" {
		return nil, errors.Errorf("%s: Key can't be empty", sm)
	}
	if len(desiredValues) == 0 {
		return nil, errors.Errorf("%s: Values can't be empty", sm)
	}
	if err := validateLabelKey(key); err != nil {
		return nil, err
	}
	switch op {
	case v1alpha1.SliceSelectorOpIn,
		v1alpha1.SliceSelectorOpNotIn,
		v1alpha1.SliceSelectorOpEquals,
		v1alpha1.SliceSelectorOpNotEquals:
		// are supported
	default:
		return nil, errors.Errorf("%s: Operator '%v' is not recognized", sm, op)
	}
	// validate before storing these desired values
	for i := range desiredValues {
		if err := validateLabelValue(key, desiredValues[i]); err != nil {
			return nil, err
		}
	}
	// set evaluated values
	sm.Key = key
	sm.Operator = op
	sm.DesiredValues = desiredValues

	return sm, nil
}

// subsetOfObservedValues verifies if desired values is contained
// in the provided observed slice
func (r *SliceMatcher) subsetOfObservedValues(observedValues []string) bool {
	if len(observedValues) == 0 && len(r.DesiredValues) != 0 {
		return false
	}
	observedValueStore := make(map[string]bool)
	for _, observedVal := range observedValues {
		observedValueStore[observedVal] = true
	}
	for _, desiredValue := range r.DesiredValues {
		if !observedValueStore[desiredValue] {
			return false
		}
	}
	return true
}

// equalsObservedValues verifies if observed slice is exactly
// equal to the list of desired values
func (r *SliceMatcher) equalsObservedValues(observedValues []string) bool {
	if len(observedValues) != len(r.DesiredValues) {
		return false
	}
	return r.subsetOfObservedValues(observedValues)
}

// Match returns true if the Requirement matches the give KeySlice.
func (r *SliceMatcher) Match(observedKeySlice KeyToSlice) bool {
	switch r.Operator {
	case v1alpha1.SliceSelectorOpIn:
		return r.subsetOfObservedValues(observedKeySlice.Get(r.Key))
	case v1alpha1.SliceSelectorOpEquals:
		return r.equalsObservedValues(observedKeySlice.Get(r.Key))
	case v1alpha1.SliceSelectorOpNotIn:
		if !observedKeySlice.Has(r.Key) {
			return true
		}
		return !r.subsetOfObservedValues(observedKeySlice.Get(r.Key))
	case v1alpha1.SliceSelectorOpNotEquals:
		if !observedKeySlice.Has(r.Key) {
			return true
		}
		return !r.equalsObservedValues(observedKeySlice.Get(r.Key))
	default:
		return false
	}
}

// SliceSelector exposes match operation against string slices
type SliceSelector struct {
	// List of matchers that makes this slice selector instance.
	// The results of these matchers are ANDed to form the final
	// evaluation.
	matchers []SliceMatcher

	// This function provides an option to override evaluation of
	// match logic of this instance. In other words, if this
	// function is set then matchers of this instance will not be
	// used to evaluate.
	matchFn func(KeyToSlice) bool
}

// SliceSelectorConfig helps in creating a new instance of
// SliceSelector
type SliceSelectorConfig struct {
	// MatchSlice is a map i.e. {key,value} pairs based slice selector.
	// A single {key,value} in the MatchSlice map is equivalent to an
	// element of MatchSliceExpressions, whose key field is "key", the
	// operator is "In", and the value contains an array of values of
	// datatype string.
	//
	// Key should represent the nested field path separated by dot(s)
	// i.e. '.'
	//
	// A MatchSlice is converted into a list of SliceSelectorRequirement
	MatchSlice map[string][]string

	// MatchSliceExpressions is a list of slice selector requirements.
	MatchSliceExpressions []v1alpha1.SliceSelectorRequirement
}

// NewSliceSelectorAlwaysTrue returns a new instance of SliceSelector
// that evaluates the match operation to true.
func NewSliceSelectorAlwaysTrue() *SliceSelector {
	return &SliceSelector{
		matchFn: func(k KeyToSlice) bool {
			return true
		},
	}
}

// NewSliceSelector returns a new instance of SliceSelector
func NewSliceSelector(config SliceSelectorConfig) (*SliceSelector, error) {
	if len(config.MatchSlice)+len(config.MatchSliceExpressions) == 0 {
		return NewSliceSelectorAlwaysTrue(), nil
	}

	selector := &SliceSelector{}
	// build the matchers
	for k, v := range config.MatchSlice {
		m, err := NewSliceMatcher(k, v1alpha1.SliceSelectorOpEquals, v)
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*m)
	}
	for _, expr := range config.MatchSliceExpressions {
		m, err := NewSliceMatcher(expr.Key, expr.Operator, expr.Values)
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*m)
	}
	return selector, nil
}

// Add adds the given matcher to this selector instance
func (s *SliceSelector) Add(m SliceMatcher) *SliceSelector {
	s.matchers = append(s.matchers, m)
	return s
}

// Match returns true if selector matches the given target.
// This logic handles ANDing all the matchers of this
// SliceSelector instance.
func (s *SliceSelector) Match(target KeyToSlice) bool {
	// If matchFn was predefined then use it to evaluate
	// the Match
	if s.matchFn != nil {
		return s.matchFn(target)
	}
	for _, m := range s.matchers {
		if !m.Match(target) {
			return false
		}
	}
	return true
}
