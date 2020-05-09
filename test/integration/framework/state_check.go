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

package framework

import (
	"fmt"
	"reflect"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
	dynamicapply "openebs.io/metac/dynamic/apply"
)

// StateCheckOperator defines the check that needs to be
// done against the resource's state
type StateCheckOperator string

const (
	// StateCheckOperatorEquals verifies if expected state
	// matches the observed state found in the cluster
	StateCheckOperatorEquals StateCheckOperator = "Equals"

	// StateCheckOperatorNotEquals verifies if expected state
	// does not match the observed state found in the cluster
	StateCheckOperatorNotEquals StateCheckOperator = "NotEquals"

	// StateCheckOperatorNotFound verifies if expected state
	// is not found in the cluster
	StateCheckOperatorNotFound StateCheckOperator = "NotFound"

	// StateCheckOperatorListCountEquals verifies if expected
	// states matches the observed states found in the cluster
	StateCheckOperatorListCountEquals StateCheckOperator = "ListCountEquals"

	// StateCheckOperatorListCountNotEquals verifies if count of
	// expected states does not match the count of observed states
	// found in the cluster
	StateCheckOperatorListCountNotEquals StateCheckOperator = "ListCountNotEquals"
)

// StateCheck verifies expected resource state against
// the observed state found in the cluster
type StateCheck struct {
	// Check operation performed between the expected state
	// and the observed state
	Operator StateCheckOperator `json:"stateCheckOperator,omitempty"`

	// Count defines the expected number of observed states
	Count *int `json:"count,omitempty"`
}

// StateCheckResultPhase defines the result of StateCheck operation
type StateCheckResultPhase string

const (
	// StateCheckResultPassed defines a successful StateCheckResult
	StateCheckResultPassed StateCheckResultPhase = "StateCheckPassed"

	// StateCheckResultWarning defines a StateCheckResult that has warnings
	StateCheckResultWarning StateCheckResultPhase = "StateCheckResultWarning"

	// StateCheckResultFailed defines an un-successful StateCheckResult
	StateCheckResultFailed StateCheckResultPhase = "StateCheckResultFailed"
)

// ToAssertResultPhase transforms StateCheckResultPhase to AssertResultPhase
func (phase StateCheckResultPhase) ToAssertResultPhase() AssertResultPhase {
	switch phase {
	case StateCheckResultPassed:
		return AssertResultPassed
	case StateCheckResultFailed:
		return AssertResultFailed
	case StateCheckResultWarning:
		return AssertResultWarning
	default:
		return ""
	}
}

// StateCheckResult holds the result of StateCheck operation
type StateCheckResult struct {
	Phase   StateCheckResultPhase `json:"phase"`
	Message string                `json:"message,omitempty"`
	Verbose string                `json:"verbose,omitempty"`
	Warning string                `json:"warning,omitempty"`
}

// StateChecking helps in verifying expected state against the
// state observed in the cluster
type StateChecking struct {
	*Fixture
	Retry *Retryable

	Name       string
	State      *unstructured.Unstructured
	StateCheck StateCheck

	actualListCount int
	operator        StateCheckOperator
	retryOnDiff     bool
	retryOnEqual    bool
	result          *StateCheckResult
	err             error
}

// StateCheckingConfig is used to create an instance of StateChecking
type StateCheckingConfig struct {
	Name       string
	Fixture    *Fixture
	State      *unstructured.Unstructured
	Retry      *Retryable
	StateCheck StateCheck
}

// NewStateChecker returns a new instance of StateChecking
func NewStateChecker(config StateCheckingConfig) *StateChecking {
	return &StateChecking{
		Name:       config.Name,
		Fixture:    config.Fixture,
		State:      config.State,
		Retry:      config.Retry,
		StateCheck: config.StateCheck,
		result:     &StateCheckResult{},
	}
}

func (sc *StateChecking) init() {
	if sc.StateCheck.Operator == "" {
		klog.V(3).Infof(
			"Will default StateCheck operator to StateCheckOperatorEquals",
		)
		sc.operator = StateCheckOperatorEquals
	} else {
		sc.operator = sc.StateCheck.Operator
	}
}

func (sc *StateChecking) validate() {
	var isCountBasedAssert bool
	// evaluate if this is count based assertion
	// based on value
	if sc.StateCheck.Count != nil {
		isCountBasedAssert = true
	}
	// evaluate if its count based assertion
	// based on operator
	if !isCountBasedAssert {
		switch sc.operator {
		case StateCheckOperatorListCountEquals,
			StateCheckOperatorListCountNotEquals:
			isCountBasedAssert = true
		}
	}
	if isCountBasedAssert && sc.StateCheck.Count == nil {
		sc.err = errors.Errorf(
			"Invalid StateCheck %q: Operator %q can't be used with nil count",
			sc.Name,
			sc.operator,
		)
	} else if isCountBasedAssert {
		switch sc.operator {
		case StateCheckOperatorEquals,
			StateCheckOperatorNotEquals,
			StateCheckOperatorNotFound:
			sc.err = errors.Errorf(
				"Invalid StateCheck %q: Operator %q can't be used with count %d",
				sc.Name,
				sc.operator,
				*sc.StateCheck.Count,
			)
		}
	}
}

func (sc *StateChecking) isMergeEqualsObserved(context string) (bool, error) {
	var observed, merged *unstructured.Unstructured
	err := sc.Retry.Waitf(
		// returning true will stop retrying this func
		func() (bool, error) {
			client, err := sc.dynamicClientset.
				GetClientForAPIVersionAndKind(
					sc.State.GetAPIVersion(),
					sc.State.GetKind(),
				)
			if err != nil {
				return false, err
			}
			observed, err = client.
				Namespace(sc.State.GetNamespace()).
				Get(
					sc.State.GetName(),
					metav1.GetOptions{},
				)
			if err != nil {
				return false, err
			}
			merged = &unstructured.Unstructured{}
			merged.Object, err = dynamicapply.Merge(
				observed.UnstructuredContent(), // observed
				sc.State.UnstructuredContent(), // last applied
				sc.State.UnstructuredContent(), // desired
			)
			if err != nil {
				// we exit the wait condition in case of merge error
				return true, err
			}
			if sc.retryOnDiff && !reflect.DeepEqual(merged, observed) {
				return false, nil
			}
			if sc.retryOnEqual && reflect.DeepEqual(merged, observed) {
				return false, nil
			}
			return true, nil
		},
		context,
	)
	klog.V(2).Infof(
		"Is state equal? %t: Diff\n%s",
		reflect.DeepEqual(merged, observed),
		cmp.Diff(merged, observed),
	)
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(merged, observed), nil
}

func (sc *StateChecking) assertEquals() {
	var message = fmt.Sprintf("StateCheckEquals: Resource %s %s: GVK %s: %s",
		sc.State.GetNamespace(),
		sc.State.GetName(),
		sc.State.GroupVersionKind(),
		sc.Name,
	)
	// We want to retry in case of any difference between
	// expected and observed states. This is done with the
	// expectation of having observed state equal to the
	// expected state eventually.
	sc.retryOnDiff = true
	success, err := sc.isMergeEqualsObserved(message)
	if err != nil {
		sc.err = err
		return
	}
	// init phase as failed
	sc.result.Phase = StateCheckResultFailed
	if success {
		sc.result.Phase = StateCheckResultPassed
	}
	sc.result.Message = message
}

func (sc *StateChecking) assertNotEquals() {
	var message = fmt.Sprintf("StateCheckNotEquals: Resource %s %s: GVK %s: %s",
		sc.State.GetNamespace(),
		sc.State.GetName(),
		sc.State.GroupVersionKind(),
		sc.Name,
	)
	// We want to retry if expected and observed states are found
	// to be equal. This is done with the expectation of having
	// observed state not equal to the expected state eventually.
	sc.retryOnEqual = true
	success, err := sc.isMergeEqualsObserved(message)
	if err != nil {
		sc.err = err
		return
	}
	// init phase as failed
	sc.result.Phase = StateCheckResultFailed
	if !success {
		sc.result.Phase = StateCheckResultPassed
	}
	sc.result.Message = message
}

func (sc *StateChecking) assertNotFound() {
	var message = fmt.Sprintf("StateCheckNotFound: Resource %s %s: GVK %s: %s",
		sc.State.GetNamespace(),
		sc.State.GetName(),
		sc.State.GroupVersionKind(),
		sc.Name,
	)
	var warning string
	// init result to Failed
	var phase = StateCheckResultFailed
	err := sc.Retry.Waitf(
		func() (bool, error) {
			client, err := sc.dynamicClientset.
				GetClientForAPIVersionAndKind(
					sc.State.GetAPIVersion(),
					sc.State.GetKind(),
				)
			if err != nil {
				return false, err
			}
			got, err := client.
				Namespace(sc.State.GetNamespace()).
				Get(
					sc.State.GetName(),
					metav1.GetOptions{},
				)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// phase is set to Passed here
					phase = StateCheckResultPassed
					return true, nil
				}
				return false, err
			}
			if len(got.GetFinalizers()) == 0 && got.GetDeletionTimestamp() != nil {
				phase = StateCheckResultWarning
				warning = fmt.Sprintf(
					"Marking StateCheck %q to passed: Finalizer count %d: Deletion timestamp %s",
					sc.Name,
					len(got.GetFinalizers()),
					got.GetDeletionTimestamp(),
				)
				return true, nil
			}
			// condition fails since resource still exists
			return false, nil
		},
		message,
	)
	if err != nil {
		sc.err = err
		return
	}
	sc.result.Phase = phase
	sc.result.Message = message
	sc.result.Warning = warning
}

func (sc *StateChecking) isListCountMatch() (bool, error) {
	client, err := sc.dynamicClientset.
		GetClientForAPIVersionAndKind(
			sc.State.GetAPIVersion(),
			sc.State.GetKind(),
		)
	if err != nil {
		return false, err
	}
	list, err := client.
		Namespace(sc.State.GetNamespace()).
		List(metav1.ListOptions{
			LabelSelector: labels.Set(
				sc.State.GetLabels(),
			).String(),
		})
	if err != nil {
		return false, err
	}
	sc.actualListCount = len(list.Items)
	return sc.actualListCount == *sc.StateCheck.Count, nil
}

func (sc *StateChecking) assertListCountEquals() {
	var message = fmt.Sprintf("AssertListCountEquals: Resource %s: GVK %s: %s",
		sc.State.GetNamespace(),
		sc.State.GroupVersionKind(),
		sc.Name,
	)
	// init result to Failed
	var phase = StateCheckResultFailed
	err := sc.Retry.Waitf(
		func() (bool, error) {
			match, err := sc.isListCountMatch()
			if err != nil {
				return false, err
			}
			if match {
				phase = StateCheckResultPassed
				// returning a true implies this condition will
				// not be tried
				return true, nil
			}
			return false, nil
		},
		message,
	)
	if err != nil {
		sc.err = err
		return
	}
	sc.result.Phase = phase
	sc.result.Message = message
	sc.result.Verbose = fmt.Sprintf(
		"Expected count %d got %d",
		*sc.StateCheck.Count,
		sc.actualListCount,
	)
}

func (sc *StateChecking) assertListCountNotEquals() {
	var message = fmt.Sprintf("AssertListCountNotEquals: Resource %s: GVK %s: %s",
		sc.State.GetNamespace(),
		sc.State.GroupVersionKind(),
		sc.Name,
	)
	// init result to Failed
	var phase = StateCheckResultFailed
	err := sc.Retry.Waitf(
		func() (bool, error) {
			match, err := sc.isListCountMatch()
			if err != nil {
				return false, err
			}
			if !match {
				phase = StateCheckResultPassed
				// returning a true implies this condition will
				// not be tried
				return true, nil
			}
			return false, nil
		},
		message,
	)
	if err != nil {
		sc.err = err
		return
	}
	sc.result.Phase = phase
	sc.result.Message = message
	sc.result.Verbose = fmt.Sprintf(
		"Expected count %d got %d",
		*sc.StateCheck.Count,
		sc.actualListCount,
	)
}

func (sc *StateChecking) assert() {
	switch sc.operator {
	case StateCheckOperatorEquals:
		sc.assertEquals()

	case StateCheckOperatorNotEquals:
		sc.assertNotEquals()

	case StateCheckOperatorNotFound:
		sc.assertNotFound()

	case StateCheckOperatorListCountEquals:
		sc.assertListCountEquals()

	case StateCheckOperatorListCountNotEquals:
		sc.assertListCountNotEquals()

	default:
		sc.err = errors.Errorf(
			"Invalid StateCheck operator %q",
			sc.operator,
		)
	}
}

// Run executes the assertion
func (sc *StateChecking) Run() (StateCheckResult, error) {
	var fns = []func(){
		sc.init,
		sc.validate,
		sc.assert,
	}
	for _, fn := range fns {
		fn()
		if sc.err != nil {
			return StateCheckResult{}, errors.Wrapf(
				sc.err,
				"Info %s: Warn %s: Verbose %s",
				sc.result.Message,
				sc.result.Warning,
				sc.result.Verbose,
			)
		}
	}
	return *sc.result, nil
}
