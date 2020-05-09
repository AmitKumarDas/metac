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
	"fmt"
	"testing"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestStepResultPhase is a typed definition to determine the
// result of executing a test step
type TestStepResultPhase string

const (
	// TestStepResultPassed implies a passed test
	TestStepResultPassed TestStepResultPhase = "Passed"

	// TestStepResultFailed implies a failed test
	TestStepResultFailed TestStepResultPhase = "Failed"

	// TestStepResultWarning implies a failed test
	TestStepResultWarning TestStepResultPhase = "Warning"
)

// TestStepResult holds the result of executing a TestStep
type TestStepResult struct {
	Step    int                 `json:"step"`
	Phase   TestStepResultPhase `json:"phase"`
	Message string              `json:"message,omitempty"`
	Verbose string              `json:"verbose,omitempty"`
	Warning string              `json:"warning,omitempty"`
}

// IntegrationTestResult holds the results of each TestStep
type IntegrationTestResult struct {
	Phase           TestStepResultPhase       `json:"phase"`
	FailedSteps     int                       `json:"failedSteps"`
	TotalSteps      int                       `json:"totalSteps"`
	TestStepResults map[string]TestStepResult `json:"results"`
}

// String implements the Stringer interface
func (itr IntegrationTestResult) String() string {
	raw, err := json.MarshalIndent(
		itr,
		" ",
		".",
	)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// TestStep that needs to be executed as part of IntegrationTest
type TestStep struct {
	Name              string  `json:"name"`
	Assert            *Assert `json:"assert,omitempty"`
	Apply             Apply   `json:"apply,omitempty"`
	LogErrorAsWarning *bool   `json:"logErrAsWarn,omitempty"`
}

// String implements the Stringer interface
func (step TestStep) String() string {
	raw, err := json.MarshalIndent(
		step,
		" ",
		".",
	)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// TestStepRunner executes a TestStep
type TestStepRunner struct {
	*Fixture
	StepIndex int
	TestStep  TestStep
}

func (r *TestStepRunner) isDelete() (bool, error) {
	if r.TestStep.Apply.State != nil &&
		r.TestStep.Apply.Replicas != nil &&
		*r.TestStep.Apply.Replicas == 0 {
		return true, nil
	}
	if r.TestStep.Apply.State == nil {
		return false, nil
	}
	spec, found, err := unstructured.NestedFieldNoCopy(
		r.TestStep.Apply.State.UnstructuredContent(),
		"spec",
	)
	if err != nil {
		return false, err
	}
	if found && spec == nil {
		return true, nil
	}
	return false, nil
}

func (r *TestStepRunner) delete() (*TestStepResult, error) {
	var message = fmt.Sprintf(
		"Delete %s %s: %s",
		r.TestStep.Apply.State.GetNamespace(),
		r.TestStep.Apply.State.GetName(),
		r.TestStep.Apply.State.GroupVersionKind(),
	)
	client, err := r.dynamicClientset.
		GetClientForAPIVersionAndKind(
			r.TestStep.Apply.State.GetAPIVersion(),
			r.TestStep.Apply.State.GetKind(),
		)
	if err != nil {
		return nil, err
	}
	err = client.
		Namespace(r.TestStep.Apply.State.GetNamespace()).
		Delete(
			r.TestStep.Apply.State.GetName(),
			&metav1.DeleteOptions{},
		)
	if err != nil {
		return nil, err
	}
	return &TestStepResult{
		Phase:   TestStepResultPassed,
		Message: message,
	}, nil
}

func (r *TestStepRunner) assert() (*TestStepResult, error) {
	a := NewAsserter(AssertableConfig{
		Name:    r.TestStep.Name,
		Fixture: r.Fixture,
		Assert:  r.TestStep.Assert,
		Retry:   NewRetry(RetryConfig{}),
	})
	got, err := a.Run()
	if err != nil {
		return nil, err
	}
	return &TestStepResult{
		Phase:   got.Phase.ToTestStepResultPhase(),
		Message: got.Message,
		Verbose: got.Verbose,
		Warning: got.Warning,
	}, nil
}

func (r *TestStepRunner) apply() (*TestStepResult, error) {
	a := NewApplier(
		ApplyableConfig{
			Fixture: r.Fixture,
			Apply:   r.TestStep.Apply,
			Retry:   NewRetry(RetryConfig{}),
		},
	)
	got, err := a.Run()
	if err != nil {
		return nil, err
	}
	return &TestStepResult{
		Phase:   got.Phase.ToTestStepResultPhase(),
		Message: got.Message,
		Warning: got.Warning,
	}, nil
}

func (r *TestStepRunner) tryRunAssert() (*TestStepResult, bool, error) {
	if r.TestStep.Assert == nil {
		return nil, false, nil
	}
	got, err := r.assert()
	return got, true, err
}

func (r *TestStepRunner) tryRunDelete() (*TestStepResult, bool, error) {
	isDel, err := r.isDelete()
	if err != nil {
		return nil, false, err
	}
	if !isDel {
		return nil, false, nil
	}
	got, err := r.delete()
	return got, true, err
}

func (r *TestStepRunner) tryRunApply() (*TestStepResult, bool, error) {
	if r.TestStep.Apply.State == nil {
		return nil, false, nil
	}
	got, err := r.apply()
	return got, true, err
}

// Run executes the test step
func (r *TestStepRunner) Run() (TestStepResult, error) {
	var probables = []func() (*TestStepResult, bool, error){
		r.tryRunAssert,
		r.tryRunDelete, // delete needs to be placed before apply
		r.tryRunApply,
	}
	for _, fn := range probables {
		got, isRun, err := fn()
		if err != nil {
			if r.TestStep.LogErrorAsWarning != nil &&
				*r.TestStep.LogErrorAsWarning {
				// treat error as warning & continue
				return TestStepResult{
					Step:    r.StepIndex,
					Phase:   TestStepResultWarning,
					Warning: err.Error(),
				}, nil
			}
			return TestStepResult{}, err
		}
		if isRun {
			got.Step = r.StepIndex
			return *got, nil
		}
	}
	return TestStepResult{},
		errors.Errorf("Invalid test step: Can't determine action")
}

// IntegrationTesting provides a structural representation to
// write integration test cases
type IntegrationTesting struct {
	*Fixture

	IntegrationTestResult IntegrationTestResult
}

// NewIntegrationTester returns a new instance of IntegrationTester
func NewIntegrationTester(t *testing.T) *IntegrationTesting {
	f := NewFixture(t)
	return &IntegrationTesting{
		Fixture: f,
		IntegrationTestResult: IntegrationTestResult{
			TestStepResults: map[string]TestStepResult{},
		},
	}
}

func (it *IntegrationTesting) validate(step TestStep) error {
	if step.Name == "" {
		return errors.Errorf(
			"Invalid step: Missing name",
		)
	}
	var action int
	if step.Assert != nil {
		action++
	}
	if step.Apply.State != nil {
		action++
	}
	if action == 0 {
		return errors.Errorf(
			"Invalid test step %q: Test step needs one action",
			step.Name,
		)
	}
	if action > 1 {
		return errors.Errorf(
			"Invalid step %q: Test step supports only one action",
			step.Name,
		)
	}
	return nil
}

// Test runs the steps required to execute integration testing
func (it *IntegrationTesting) Test(steps []TestStep) (IntegrationTestResult, error) {
	// stage 1 - validate the steps
	for _, step := range steps {
		err := it.validate(step)
		if err != nil {
			return IntegrationTestResult{}, err
		}
	}
	// stage 2 - run the steps
	var failedSteps int
	for idx, step := range steps {
		r := &TestStepRunner{
			Fixture:   it.Fixture,
			StepIndex: idx + 1,
			TestStep:  step,
		}
		got, err := r.Run()
		if err != nil {
			return IntegrationTestResult{}, errors.Wrapf(
				err,
				"Failed at step %d: %q",
				idx+1,
				step.Name,
			)
		}
		it.IntegrationTestResult.TestStepResults[step.Name] = got
		if got.Phase == TestStepResultFailed {
			failedSteps++
		}
	}
	// build the result
	if failedSteps > 0 {
		it.IntegrationTestResult.Phase = TestStepResultFailed
		it.IntegrationTestResult.FailedSteps = failedSteps
	} else {
		it.IntegrationTestResult.Phase = TestStepResultPassed
	}
	it.IntegrationTestResult.TotalSteps = len(steps)
	return it.IntegrationTestResult, nil
}
