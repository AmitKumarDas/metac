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
	"encoding/json"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Assert handles assertion of desired state against
// the observed state found in the cluster
type Assert struct {
	// Desired state(s) that is asserted against the observed
	// state(s)
	State *unstructured.Unstructured `json:"state"`

	// StateCheck has assertions related to state of resources
	StateCheck *StateCheck `json:"stateCheck,omitempty"`

	// PathCheck has assertions related to resource paths
	PathCheck *PathCheck `json:"pathCheck,omitempty"`
}

// String implements the Stringer interface
func (a Assert) String() string {
	raw, err := json.MarshalIndent(
		a,
		" ",
		".",
	)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// AssertResultPhase is a typed definition to determine the
// result of executing an assert
type AssertResultPhase string

const (
	// AssertResultPassed indicates that the desired
	// assertion was successful
	AssertResultPassed AssertResultPhase = "AssertPassed"

	// AssertResultWarning indicates that the desired
	// assertion resulted in warning
	AssertResultWarning AssertResultPhase = "AssertWarning"

	// AssertResultFailed indicates that the desired
	// assertion was not successful
	AssertResultFailed AssertResultPhase = "AssertFailed"
)

// ToTestStepResultPhase transforms AssertResultPhase to TestResultPhase
func (phase AssertResultPhase) ToTestStepResultPhase() TestStepResultPhase {
	switch phase {
	case AssertResultPassed:
		return TestStepResultPassed
	case AssertResultFailed:
		return TestStepResultFailed
	case AssertResultWarning:
		return TestStepResultWarning
	default:
		return ""
	}
}

// AssertResult holds the result of assertion
type AssertResult struct {
	Phase   AssertResultPhase `json:"phase"`
	Message string            `json:"message,omitempty"`
	Verbose string            `json:"verbose,omitempty"`
	Warning string            `json:"warning,omitempty"`
}

// AssertCheckType defines the type of assert check
type AssertCheckType int

const (
	// AssertCheckTypeState defines a state check based assertion
	AssertCheckTypeState AssertCheckType = iota

	// AssertCheckTypePath defines a path check based assertion
	AssertCheckTypePath
)

// Assertable is used to perform matches of desired state(s)
// against observed state(s)
type Assertable struct {
	*Fixture
	Retry  *Retryable
	Name   string
	Assert *Assert

	assertCheckType AssertCheckType
	retryOnDiff     bool
	retryOnEqual    bool
	result          *AssertResult
	err             error
}

// AssertableConfig is used to create an instance of Assertable
type AssertableConfig struct {
	Fixture *Fixture
	Retry   *Retryable
	Name    string
	Assert  *Assert
}

// NewAsserter returns a new instance of Assertion
func NewAsserter(config AssertableConfig) *Assertable {
	return &Assertable{
		Assert:  config.Assert,
		Retry:   config.Retry,
		Fixture: config.Fixture,
		Name:    config.Name,
		result:  &AssertResult{},
	}
}

func (a *Assertable) init() {
	var checks int
	if a.Assert.PathCheck != nil {
		checks++
		a.assertCheckType = AssertCheckTypePath
	}
	if a.Assert.StateCheck != nil {
		checks++
		a.assertCheckType = AssertCheckTypeState
	}
	if checks > 1 {
		a.err = errors.Errorf(
			"Failed to assert %q: More than one assert checks found",
			a.Name,
		)
		return
	}
	if checks == 0 {
		// assert default to StateCheck based assertion
		a.Assert.StateCheck = &StateCheck{
			Operator: StateCheckOperatorEquals,
		}
	}
}

func (a *Assertable) runAssertByPath() {
	chk := NewPathChecker(
		PathCheckingConfig{
			Name:      a.Name,
			Fixture:   a.Fixture,
			State:     a.Assert.State,
			PathCheck: *a.Assert.PathCheck,
			Retry:     a.Retry,
		},
	)
	got, err := chk.Run()
	if err != nil {
		a.err = err
		return
	}
	a.result = &AssertResult{
		Phase:   got.Phase.ToAssertResultPhase(),
		Message: got.Message,
		Verbose: got.Verbose,
		Warning: got.Warning,
	}
}

func (a *Assertable) runAssertByState() {
	chk := NewStateChecker(
		StateCheckingConfig{
			Name:       a.Name,
			Fixture:    a.Fixture,
			State:      a.Assert.State,
			StateCheck: *a.Assert.StateCheck,
			Retry:      a.Retry,
		},
	)
	got, err := chk.Run()
	if err != nil {
		a.err = err
		return
	}
	a.result = &AssertResult{
		Phase:   got.Phase.ToAssertResultPhase(),
		Message: got.Message,
		Verbose: got.Verbose,
		Warning: got.Warning,
	}
}

func (a *Assertable) runAssert() {
	switch a.assertCheckType {
	case AssertCheckTypePath:
		a.runAssertByPath()
	case AssertCheckTypeState:
		a.runAssertByState()
	default:
		a.err = errors.Errorf(
			"Failed to run assert %q: Invalid operator %q",
			a.Name,
			a.assertCheckType,
		)
	}
}

// Run executes the assertion
func (a *Assertable) Run() (AssertResult, error) {
	if a.Name == "" {
		return AssertResult{}, errors.Errorf(
			"Failed to run assert: Missing assert name",
		)
	}
	if a.Assert == nil || a.Assert.State == nil {
		return AssertResult{}, errors.Errorf(
			"Failed to run assert %q: Nil assert state",
			a.Name,
		)
	}
	var fns = []func(){
		a.init,
		a.runAssert,
	}
	for _, fn := range fns {
		fn()
		if a.err != nil {
			return AssertResult{}, a.err
		}
	}
	return *a.result, nil
}
