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
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"openebs.io/metac/apis/metacontroller/v1alpha1"
)

// Evaluation flags a target resource as a match or no match based
// on various terms & conditions decribed via SelectorTerm and
// ocassionally via a reference resource.
type Evaluation struct {
	// The target resource that gets evaluated against the selector
	// terms.
	Target *unstructured.Unstructured

	// Selector terms that forms the evaluation logic against the target
	Terms []*v1alpha1.SelectorTerm

	// In some cases, the evaluation of a target is performed by comparing
	// the results of evaluation against another resource.
	//
	// NOTE:
	// 	This reference resource for example can be the parent or watch
	// resource referred to in various meta controllers.
	Reference *unstructured.Unstructured
}

// RunMatch flags the given target as a match or no match (represented as
// true or false correspondingly) by executing this target against **all**
// the SelectTerm(s).
//
// NOTE:
//	Evaluation logic does a union of SelectTerm(s) (i.e. select term results
// are OR-ed) while match expressions (e.g. MatchSlice, MatchLabels,
// MatchAnnotationExpressions, etc) found within one SelectTerm are AND-ed.
func (e Evaluation) RunMatch() (bool, error) {
	if len(e.Terms) == 0 {
		// no terms imply everything matches
		return true, nil
	}

	if e.Target == nil {
		return false, errors.Errorf("Evaluation failed: Nil target")
	}

	// NOTE:
	// 	A match function corresponds to specific match expression
	// found in a SelectTerm. Running all the match functions
	// can evaluate if this SelectTerm passed or failed the evaluation.
	//
	// NOTE:
	//	A match function returns true if its match expression(s) is not
	// specified. A SelectTerm need not specify all the match expressions,
	// since each match expression within a SelectTerm can be optional.
	matchFns := []func(v1alpha1.SelectorTerm) (bool, error){
		e.isFieldMatch,
		e.isAnnotationMatch,
		e.isLabelMatch,
		e.isSliceMatch,
		e.isReferenceMatch,
	}
	matchFnsCount := len(matchFns)

	// Matching SelectTerms are **ORed** against the target. Hence if any
	// SelectTerm is a match i.e. if any iteration has a successful match,
	// the overall match is a success & returns true
	for _, selectTerm := range e.Terms {
		if selectTerm == nil {
			// nothing to be done
			//
			// NOTE:
			//	Ideally a SelectTerm should not be nil
			continue
		}

		// counter to check if it equals successful match functions
		// which in turn implies this SelectTerm match was a success
		successfulSelectTermCount := 0

		// Each match specified in a SelectTerm are ANDed
		//
		// NOTE:
		// 	One of more match expressions found in a SelectTerm
		// are executed via corresponding match functions
		for _, match := range matchFns {
			isMatch, err := match(*selectTerm)
			if err != nil {
				return false, err
			}
			if !isMatch {
				// Since each match within a SelectTerm AND-ed,
				// any failed match implies current SelectTerm failed.
				// Hence ignore the current term & evaluate the
				// next SelectTerm.
				//
				// Technically speaking, this breaks the current for loop
				// & continues with the immediate outer loop.
				break
			}
			successfulSelectTermCount++
		}

		// check whether all match expressions in the current SelectTerm
		// succeeded
		if successfulSelectTermCount == matchFnsCount {
			// no need to check for subsequent SelectTerms since
			// terms are ORed
			return true, nil
		}
	}
	// at this point no SelectTerm(s) would have succeeded
	// hence fail this evaluation
	return false, nil
}

// isAnnotationMatch annotation expressions against the target's annotations
func (e *Evaluation) isAnnotationMatch(term v1alpha1.SelectorTerm) (bool, error) {
	if len(term.MatchAnnotations)+len(term.MatchAnnotationExpressions) == 0 {
		// Match is true if there are no annotation expressions
		//
		// Note that these expressions are optional
		return true, nil
	}

	if e.Target == nil {
		return false, errors.Errorf("Evaluation failed: Nil target")
	}

	// label selector can be used for annotation expressions
	sel := &metav1.LabelSelector{
		MatchLabels:      term.MatchAnnotations,
		MatchExpressions: term.MatchAnnotationExpressions,
	}
	annSel, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return false, errors.Wrapf(err, "Invalid annotation expressions: %v", sel)
	}

	return annSel.Matches(labels.Set(e.Target.GetAnnotations())), nil
}

// isLabelMatch runs label expressions against the target's labels
func (e *Evaluation) isLabelMatch(term v1alpha1.SelectorTerm) (bool, error) {
	if len(term.MatchLabels)+len(term.MatchLabelExpressions) == 0 {
		// Match is true if there are no label expressions
		//
		// Note that these expressions are optional
		return true, nil
	}

	if e.Target == nil {
		return false, errors.Errorf("Evaluation failed: Nil target")
	}

	sel := &metav1.LabelSelector{
		MatchLabels:      term.MatchLabels,
		MatchExpressions: term.MatchLabelExpressions,
	}
	lblSel, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return false, errors.Wrapf(err, "Invalid label expressions: %v", sel)
	}

	return lblSel.Matches(labels.Set(e.Target.GetLabels())), nil
}

// isFieldMatch runs field expresssions against the target
//
// NOTE:
//	A field expression's key is set with a field path of the target
// which in turn is expected to hold a string as its value.
func (e *Evaluation) isFieldMatch(term v1alpha1.SelectorTerm) (bool, error) {
	if len(term.MatchFields)+len(term.MatchFieldExpressions) == 0 {
		// Match is true if there are no field expressions
		//
		// Note that field selector expressions are optional
		return true, nil
	}
	if e.Target == nil {
		return false, errors.Errorf("Field match failed: Nil target")
	}
	// field selector piggy backs on label selector
	sel := NewFieldSelector(&metav1.LabelSelector{
		MatchLabels:      term.MatchFields,
		MatchExpressions: term.MatchFieldExpressions,
	})
	return sel.Match(e.Target)
}

// isSliceMatch runs slice expressions against the target
//
// NOTE:
//	A slice expression key is set with the field path of the target
// which in turn is expected to hold a slice as its value.
func (e *Evaluation) isSliceMatch(term v1alpha1.SelectorTerm) (bool, error) {
	if len(term.MatchSlice)+len(term.MatchSliceExpressions) == 0 {
		// Match is true if there are no slice expressions
		//
		// Note that these expressions are optional
		return true, nil
	}

	if e.Target == nil {
		return false, errors.Errorf("Evaluation failed: Nil target")
	}

	config := SliceSelectorConfig{
		MatchSlice:            term.MatchSlice,
		MatchSliceExpressions: term.MatchSliceExpressions,
	}
	sliceSelector, err := NewSliceSelector(config)
	if err != nil {
		return false, errors.Wrapf(err, "Invalid slice expressions: %v", config)
	}

	// fill up specified selector keys with **actual** values
	// from the target & build a new target that can
	// be evaluated in terms of slice matches
	newTarget := make(map[string][]string)
	for _, sExp := range term.MatchSliceExpressions {
		if sExp.Key == "" {
			return false,
				errors.Errorf("Invalid match slice expressions: Missing key: %v", sExp)
		}
		fields := strings.Split(sExp.Key, ".")

		// extract actual value(s) from target
		actualVals, _, err := unstructured.NestedStringSlice(e.Target.Object, fields...)
		if err != nil {
			return false,
				errors.Wrapf(err, "Match slice expressions failed: Key %s", sExp.Key)
		}
		newTarget[sExp.Key] = actualVals
	}
	for key := range term.MatchSlice {
		if key == "" {
			return false,
				errors.Errorf("Invalid match slice: Missing key: %s", key)
		}
		fields := strings.Split(key, ".")

		// extract actual value(s) from target
		actualVals, _, err := unstructured.NestedStringSlice(e.Target.Object, fields...)
		if err != nil {
			return false,
				errors.Wrapf(err, "Slice match failed: Key %s", key)
		}
		newTarget[key] = actualVals
	}
	// run the match against this newly built target
	return sliceSelector.Match(newTarget), nil
}

// isReferenceMatch runs reference expressions against the target's
// field path and compares the result against same field path
// of the reference resource.
//
// NOTE:
//	A reference expression key is a field path of the resource that
// hold a string as its value.
func (e *Evaluation) isReferenceMatch(term v1alpha1.SelectorTerm) (bool, error) {
	if len(term.MatchReference)+len(term.MatchReferenceExpressions) == 0 {
		// Match is true if there are no reference expressions
		//
		// Since these expressions are optional
		return true, nil
	}

	if e.Target == nil {
		return false, errors.Errorf("Evaluation failed for reference expressions: Nil target")
	}

	if e.Reference == nil {
		return false, errors.Errorf("Evaluation failed for reference expressions: Nil reference")
	}

	var targetLblExpressions []metav1.LabelSelectorRequirement
	referenceKeyValPairs := make(map[string]string)
	notFoundValue := "given-fieldpath-doesnot-exist"

	// -----------------------------------------------------------------------
	// 1/ build label selector requirements from MatchReference
	// -----------------------------------------------------------------------
	for idx, fieldPath := range term.MatchReference {
		if fieldPath == "" {
			return false,
				errors.Errorf("Invalid reference expressions: Missing key at %d", idx)
		}
		fields := strings.Split(fieldPath, ".")

		// extract actual value from target using the field path
		tVal, found, err := unstructured.NestedString(e.Target.Object, fields...)
		if err != nil {
			return false,
				errors.Wrapf(
					err,
					"Failed to get value for reference expressions key %s: Target %s %s",
					fieldPath,
					e.Target.GetNamespace(), e.Target.GetName(),
				)
		}
		if !found {
			// set some unique value for the targetLblExpressions
			//
			// this helps in negating a match when matching an
			// empty value with another empty value is true
			tVal = notFoundValue
		}
		targetLblExpressions = append(
			targetLblExpressions,
			metav1.LabelSelectorRequirement{
				Key: fieldPath, Operator: metav1.LabelSelectorOpIn, Values: []string{tVal},
			},
		)

		// extract actual value from reference using the same fieldPath
		wVal, _, err := unstructured.NestedString(e.Reference.Object, fields...)
		if err != nil {
			return false,
				errors.Wrapf(
					err,
					"Failed to get value for reference expressions key %s: Reference %s %s",
					fieldPath,
					e.Reference.GetNamespace(), e.Reference.GetName(),
				)
		}
		referenceKeyValPairs[fieldPath] = wVal
	}

	// -----------------------------------------------------------------------
	// 2/ build label selector requirements from MatchReferenceExpressions
	// -----------------------------------------------------------------------
	for idx, requirement := range term.MatchReferenceExpressions {
		if requirement.Key == "" {
			return false,
				errors.Errorf("Invalid reference expressions: Missing key at %d", idx)
		}
		fields := strings.Split(requirement.Key, ".")

		// extract actual value from target based on the field path
		tVal, found, err := unstructured.NestedString(e.Target.Object, fields...)
		if err != nil {
			return false,
				errors.Wrapf(
					err,
					"Failed to get value for reference expressions key %s: Target %s %s",
					requirement.Key,
					e.Target.GetNamespace(), e.Target.GetName(),
				)
		}
		if !found {
			// set some value only for the targetLblExpressions
			//
			// this helps in negating a match when matching an
			// empty value with another empty value is true
			tVal = notFoundValue
		}
		// map reference selector operator to their corresponding label
		// selector operator
		referenceToLblOperators := map[v1alpha1.ReferenceSelectorOperator]metav1.LabelSelectorOperator{
			v1alpha1.ReferenceSelectorOpEquals:          metav1.LabelSelectorOpIn,
			v1alpha1.ReferenceSelectorOperator(""):      metav1.LabelSelectorOpIn,
			v1alpha1.ReferenceSelectorOpNotEquals:       metav1.LabelSelectorOpNotIn,
			v1alpha1.ReferenceSelectorOpEqualsUID:       metav1.LabelSelectorOpIn,
			v1alpha1.ReferenceSelectorOpEqualsName:      metav1.LabelSelectorOpIn,
			v1alpha1.ReferenceSelectorOpEqualsNamespace: metav1.LabelSelectorOpIn,
		}

		targetLblExpressions = append(
			targetLblExpressions,
			// need to map to appropriate label operator
			metav1.LabelSelectorRequirement{
				Key:      requirement.Key,
				Operator: referenceToLblOperators[requirement.Operator],
				Values:   []string{tVal},
			},
		)

		var referenceValue string
		switch requirement.Operator {
		case v1alpha1.ReferenceSelectorOpEquals,
			v1alpha1.ReferenceSelectorOpNotEquals,
			v1alpha1.ReferenceSelectorOperator(""):
			// extract actual value from reference using the same key i.e. fieldPath
			referenceValue, _, err = unstructured.NestedString(e.Reference.Object, fields...)
			if err != nil {
				return false,
					errors.Wrapf(
						err,
						"Failed to get value for reference expression key %s: Reference %s %s",
						requirement.Key,
						e.Reference.GetNamespace(), e.Reference.GetName(),
					)
			}
		case v1alpha1.ReferenceSelectorOpEqualsName:
			referenceValue = e.Reference.GetName()
		case v1alpha1.ReferenceSelectorOpEqualsUID:
			referenceValue = string(e.Reference.GetUID())
		case v1alpha1.ReferenceSelectorOpEqualsNamespace:
			referenceValue = e.Reference.GetNamespace()
		default:
			return false,
				errors.Errorf(
					"Operator %s is not recognised for reference expression key %s",
					requirement.Operator, requirement.Key,
				)
		}

		referenceKeyValPairs[requirement.Key] = referenceValue
	}

	// build label selector instance from target expressions
	targetSel := &metav1.LabelSelector{MatchExpressions: targetLblExpressions}
	targetSelEvaluator, err := metav1.LabelSelectorAsSelector(targetSel)
	if err != nil {
		return false,
			errors.Wrapf(
				err,
				"Failed to build target selector from reference expressions: %v", targetSel,
			)
	}

	// At this point all reference expressions are converted to label expressions
	//
	// Hence, we can make use of label selector to evaluate if target matches
	// the reference as per all the ReferenceExpressions
	return targetSelEvaluator.Matches(labels.Set(referenceKeyValPairs)), nil
}
