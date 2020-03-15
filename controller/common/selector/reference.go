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
	"openebs.io/metac/apis/metacontroller/v1alpha1"
)

const (
	notFoundValue string = "some-value-that-should-never-be-used"
)

// ReferenceSelectorConfig is used to build a new instance of
// ReferenceSelection
type ReferenceSelectorConfig struct {
	MatchReference            []string
	MatchReferenceExpressions []v1alpha1.ReferenceSelectorRequirement
}

// ReferenceSelection helps matching a target by comparing
// values from this target against the values from a
// reference
//
// A target is an attachment while reference is a watch in
// the context of MetaControllers.
type ReferenceSelection struct {
	config    ReferenceSelectorConfig
	target    *unstructured.Unstructured
	reference *unstructured.Unstructured

	operatorMapping map[v1alpha1.ReferenceSelectorOperator]metav1.LabelSelectorOperator

	targetExpressions []metav1.LabelSelectorRequirement
	referencePairs    map[string]string

	err error
}

// NewReferenceSelector returns a new instance of
// ReferenceSelection
func NewReferenceSelector(config ReferenceSelectorConfig) *ReferenceSelection {
	s := &ReferenceSelection{
		config:         config,
		referencePairs: map[string]string{},
		// map reference selector operator to corresponding
		// label selector operator
		operatorMapping: map[v1alpha1.ReferenceSelectorOperator]metav1.LabelSelectorOperator{
			v1alpha1.ReferenceSelectorOpEquals:          metav1.LabelSelectorOpIn,
			v1alpha1.ReferenceSelectorOperator(""):      metav1.LabelSelectorOpIn,
			v1alpha1.ReferenceSelectorOpNotEquals:       metav1.LabelSelectorOpNotIn,
			v1alpha1.ReferenceSelectorOpEqualsUID:       metav1.LabelSelectorOpIn,
			v1alpha1.ReferenceSelectorOpEqualsName:      metav1.LabelSelectorOpIn,
			v1alpha1.ReferenceSelectorOpEqualsNamespace: metav1.LabelSelectorOpIn,
		},
	}
	return s
}

func (s *ReferenceSelection) pathToFields(nestedpath string) []string {
	var restored []string
	// '\' is the escape character for .
	sanitised := strings.ReplaceAll(nestedpath, `\.`, "-@@-")
	sfields := strings.Split(sanitised, ".")
	for _, sfield := range sfields {
		restored = append(
			restored,
			strings.ReplaceAll(sfield, "-@@-", "."),
		)
	}
	return restored
}

func (s *ReferenceSelection) addTargetExpressionFromPath(nestedpath string) {
	if s.err != nil {
		return
	}
	// split the path
	fields := s.pathToFields(nestedpath)
	// extract actual value from target using the field path
	targetValue, found, err := unstructured.NestedString(
		s.target.Object,
		fields...,
	)
	if err != nil {
		s.err =
			errors.Wrapf(
				err,
				"MatchReference failed for path %q against target %s",
				nestedpath,
				s.target.GroupVersionKind().String(),
			)
		return
	}
	if !found {
		// set some unique value for the targetLblExpressions
		//
		// this helps in negating a match when matching an
		// empty value with another empty value is true
		targetValue = notFoundValue
	}
	s.targetExpressions = append(
		s.targetExpressions,
		metav1.LabelSelectorRequirement{
			Key:      strings.Join(fields, "."),
			Operator: metav1.LabelSelectorOpIn, // default
			Values:   []string{targetValue},
		},
	)
}

func (s *ReferenceSelection) addReferencePairFromPath(nestedpath string) {
	if s.err != nil {
		return
	}
	// split the path
	fields := s.pathToFields(nestedpath)
	// extract actual value from reference using the path
	referenceVal, _, err := unstructured.NestedString(
		s.reference.Object,
		fields...,
	)
	if err != nil {
		s.err =
			errors.Wrapf(
				err,
				"MatchReference failed for path %q against reference %s",
				nestedpath,
				s.reference.GroupVersionKind().String(),
			)
		return
	}
	s.referencePairs[strings.Join(fields, ".")] = referenceVal
}

func (s *ReferenceSelection) addTargetExpressionFromExp(
	exp v1alpha1.ReferenceSelectorRequirement,
) {
	if s.err != nil {
		return
	}
	// split the path
	fields := s.pathToFields(exp.Key)
	// extract actual value from target based on the field path
	targetValue, found, err := unstructured.NestedString(
		s.target.Object,
		fields...,
	)
	if err != nil {
		s.err = errors.Wrapf(
			err,
			"MatchReferenceExpressions failed for path %s against target %s",
			exp.Key,
			s.target.GroupVersionKind().String(),
		)
		return
	}
	if !found {
		// set some value only for the targetExpressions
		//
		// this helps in negating a match when matching an
		// empty value with another empty value is true
		targetValue = notFoundValue
	}
	// add to expressions
	s.targetExpressions = append(
		s.targetExpressions,
		// need to map to appropriate label operator
		metav1.LabelSelectorRequirement{
			Key:      strings.Join(fields, "."),
			Operator: s.operatorMapping[exp.Operator],
			Values:   []string{targetValue},
		},
	)
}

func (s *ReferenceSelection) validateExpIfRefKey(
	exp v1alpha1.ReferenceSelectorRequirement,
) error {
	if exp.RefKey == "" {
		return nil
	}
	switch exp.Operator {
	case v1alpha1.ReferenceSelectorOpEqualsName,
		v1alpha1.ReferenceSelectorOpEqualsUID,
		v1alpha1.ReferenceSelectorOpEqualsNamespace:
		return errors.Errorf(
			"Invalid MatchReferenceExpressions: Invalid operator %q with refkey %q",
			exp.Operator,
			exp.RefKey,
		)
	default:
		return nil
	}
}

func (s *ReferenceSelection) addReferencePairFromExp(
	exp v1alpha1.ReferenceSelectorRequirement,
) {
	if s.err != nil {
		return
	}
	s.err = s.validateExpIfRefKey(exp)
	if s.err != nil {
		return
	}
	// extract value from reference object
	var referenceValue string
	var err error
	// init the nested field path from the common key
	nestedpath := exp.Key
	if exp.RefKey != "" {
		// override the nested field path with reference key
		nestedpath = exp.RefKey
	}
	fields := s.pathToFields(nestedpath)
	switch exp.Operator {
	case v1alpha1.ReferenceSelectorOpEquals,
		v1alpha1.ReferenceSelectorOpNotEquals,
		v1alpha1.ReferenceSelectorOperator(""):
		// extract actual value from reference
		referenceValue, _, err =
			unstructured.NestedString(
				s.reference.Object,
				fields...,
			)
		if err != nil {
			s.err = errors.Wrapf(
				err,
				"MatchReferenceExpressions failed for path %q against reference %s",
				nestedpath,
				s.reference.GroupVersionKind().String(),
			)
			return
		}
	case v1alpha1.ReferenceSelectorOpEqualsName:
		referenceValue = s.reference.GetName()
	case v1alpha1.ReferenceSelectorOpEqualsUID:
		referenceValue = string(s.reference.GetUID())
	case v1alpha1.ReferenceSelectorOpEqualsNamespace:
		referenceValue = s.reference.GetNamespace()
	default:
		s.err = errors.Errorf(
			"Invalid MatchReferenceExpressions operator %q for path %q",
			exp.Operator,
			nestedpath,
		)
		return
	}
	// save reference value against the target key since target
	// values are stored against target key
	refPairKey := strings.Join(s.pathToFields(exp.Key), ".")
	s.referencePairs[refPairKey] = referenceValue
}

func (s *ReferenceSelection) walkExpressions() {
	for idx, exp := range s.config.MatchReferenceExpressions {
		if exp.Key == "" {
			s.err = errors.Errorf(
				"Invalid MatchReferenceExpressions: Missing key at %d",
				idx,
			)
			return
		}
		s.addReferencePairFromExp(exp)
		s.addTargetExpressionFromExp(exp)
	}
}

func (s *ReferenceSelection) walkPaths() {
	for idx, path := range s.config.MatchReference {
		if path == "" {
			s.err = errors.Errorf(
				"Invalid MatchReference: Missing key at %d",
				idx,
			)
			return
		}
		s.addReferencePairFromPath(path)
		s.addTargetExpressionFromPath(path)
	}
}

func (s *ReferenceSelection) match() (bool, error) {
	// build label selector instance from target expressions
	targetLbls := &metav1.LabelSelector{
		MatchExpressions: s.targetExpressions,
	}
	targetSelector, err :=
		metav1.LabelSelectorAsSelector(targetLbls)
	if err != nil {
		return false, errors.Wrapf(
			err,
			"Failed to build target selector from %+v",
			targetLbls,
		)
	}
	// At this point all reference values are converted to
	// label expressions. Similarly, all target values are
	// converted to label selector.
	//
	// Hence, evaluate if **target** matches the **reference**
	// as per the configured ReferenceExpressions
	return targetSelector.Matches(
		labels.Set(s.referencePairs),
	), nil
}

// Match returns true if target matches the reference
// based on existing select terms
func (s *ReferenceSelection) Match(
	target *unstructured.Unstructured,
	reference *unstructured.Unstructured,
) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	if target == nil || target.Object == nil {
		return false, errors.Errorf(
			"MatchReference failed: Nil target",
		)
	}
	if reference == nil || reference.Object == nil {
		return false, errors.Errorf(
			"MatchReference failed: Nil reference",
		)
	}
	s.target = target
	s.reference = reference
	s.walkExpressions()
	s.walkPaths()
	return s.match()
}
