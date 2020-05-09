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
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
)

// PathCheckOperator defines the check that needs to be
// done against the resource's field based on this field's
// path
type PathCheckOperator string

const (
	// PathCheckOperatorExists verifies if expected field
	// is found in the observed resource found in the cluster
	//
	// NOTE:
	// 	This is the **default** if nothing is specified
	//
	// NOTE:
	//	This is a path only check operation
	PathCheckOperatorExists PathCheckOperator = "Exists"

	// PathCheckOperatorNotExists verifies if expected field
	// is not found in the observed resource found in the cluster
	//
	// NOTE:
	//	This is a path only check operation
	PathCheckOperatorNotExists PathCheckOperator = "NotExists"

	// PathCheckOperatorEquals verifies if expected field
	// value matches the field value of the observed resource
	// found in the cluster
	//
	// NOTE:
	//	This is a path as well as value based check operation
	PathCheckOperatorEquals PathCheckOperator = "Equals"

	// PathCheckOperatorNotEquals verifies if expected field
	// value does not match the observed resource's field value
	// found in the cluster
	//
	// NOTE:
	//	This is a path as well as value based check operation
	PathCheckOperatorNotEquals PathCheckOperator = "NotEquals"

	// PathCheckOperatorGTE verifies if expected field value
	// is greater than or equal to the field value of the
	// observed resource found in the cluster
	//
	// NOTE:
	//	This is a path as well as value based check operation
	PathCheckOperatorGTE PathCheckOperator = "GTE"

	// PathCheckOperatorLTE verifies if expected field value
	// is less than or equal to the field value of the
	// observed resource found in the cluster
	//
	// NOTE:
	//	This is a path as well as value based check operation
	PathCheckOperatorLTE PathCheckOperator = "LTE"
)

// PathValueDataType defines the expected data type of the value
// set against the path
type PathValueDataType string

const (
	// PathValueDataTypeInt64 expects path's value with int64
	// as its data type
	PathValueDataTypeInt64 PathValueDataType = "int64"

	// PathValueDataTypeFloat64 expects path's value with float64
	// as its data type
	PathValueDataTypeFloat64 PathValueDataType = "float64"
)

// PathCheck verifies expected field value against
// the field value of the observed resource found in the
// cluster
type PathCheck struct {
	// Check operation performed between the expected field
	// value and the field value of the observed resource
	// found in the cluster
	Operator PathCheckOperator `json:"pathCheckOperator,omitempty"`

	// Nested path of the field found in the resource
	//
	// NOTE:
	//	This is a mandatory field
	Path string `json:"path"`

	// Expected value that gets verified against the observed
	// value based on the path & operator
	Value interface{} `json:"value,omitempty"`

	// Data type of the value e.g. int64 or float64 etc
	DataType PathValueDataType `json:"dataType,omitempty"`
}

// PathCheckResultPhase defines the result of PathCheck operation
type PathCheckResultPhase string

const (
	// PathCheckResultPassed defines a successful PathCheckResult
	PathCheckResultPassed PathCheckResultPhase = "PathCheckPassed"

	// PathCheckResultWarning defines a PathCheckResult that has warnings
	PathCheckResultWarning PathCheckResultPhase = "PathCheckResultWarning"

	// PathCheckResultFailed defines an un-successful PathCheckResult
	PathCheckResultFailed PathCheckResultPhase = "PathCheckResultFailed"
)

// ToAssertResultPhase transforms StateCheckResultPhase to AssertResultPhase
func (phase PathCheckResultPhase) ToAssertResultPhase() AssertResultPhase {
	switch phase {
	case PathCheckResultPassed:
		return AssertResultPassed
	case PathCheckResultFailed:
		return AssertResultFailed
	case PathCheckResultWarning:
		return AssertResultWarning
	default:
		return ""
	}
}

// PathCheckResult holds the result of PathCheck operation
type PathCheckResult struct {
	Phase   PathCheckResultPhase `json:"phase"`
	Message string               `json:"message,omitempty"`
	Verbose string               `json:"verbose,omitempty"`
	Warning string               `json:"warning,omitempty"`
}

// PathChecking helps in verifying expected field value
// against the field value of observed resource found in
// the cluster
type PathChecking struct {
	*Fixture
	Retry *Retryable

	Name      string
	State     *unstructured.Unstructured
	PathCheck PathCheck

	operator       PathCheckOperator
	dataType       PathValueDataType
	pathOnlyCheck  bool
	valueOnlyCheck bool

	retryIfValueNotLTE    bool
	retryIfValueNotGTE    bool
	retryIfValueNotEquals bool
	retryIfValueEquals    bool
	retryIfPathNotExists  bool
	retryIfPathExists     bool

	result *PathCheckResult
	err    error
}

// PathCheckingConfig is used to create an instance of PathChecking
type PathCheckingConfig struct {
	Fixture   *Fixture
	Retry     *Retryable
	Name      string
	State     *unstructured.Unstructured
	PathCheck PathCheck
}

// NewPathChecker returns a new instance of PathChecking
func NewPathChecker(config PathCheckingConfig) *PathChecking {
	return &PathChecking{
		Name:      config.Name,
		Fixture:   config.Fixture,
		State:     config.State,
		Retry:     config.Retry,
		PathCheck: config.PathCheck,
		result:    &PathCheckResult{},
	}
}

func (pc *PathChecking) init() {
	if pc.PathCheck.Operator == "" {
		// defaults to Exists operator
		klog.V(3).Infof(
			"Will default PathCheck operator to PathCheckOperatorExists",
		)
		pc.operator = PathCheckOperatorExists
	} else {
		pc.operator = pc.PathCheck.Operator
	}
	if pc.PathCheck.DataType == "" {
		// defaults to int64 data type
		klog.V(3).Infof(
			"Will default PathCheck datatype to PathValueDataTypeInt64",
		)
		pc.dataType = PathValueDataTypeInt64
	} else {
		pc.dataType = pc.PathCheck.DataType
	}
	switch pc.operator {
	case PathCheckOperatorExists,
		PathCheckOperatorNotExists:
		pc.pathOnlyCheck = true
	default:
		pc.valueOnlyCheck = true
	}
}

func (pc *PathChecking) validate() {
	switch pc.operator {
	case PathCheckOperatorExists,
		PathCheckOperatorNotExists:
		if pc.valueOnlyCheck {
			pc.err = errors.Errorf(
				"Invalid PathCheck %q: Operator %q can't be used with value %v",
				pc.Name,
				pc.operator,
				pc.PathCheck.Value,
			)
		}
	}
}

func (pc *PathChecking) assertValueInt64(obj *unstructured.Unstructured) (bool, error) {
	got, found, err := unstructured.NestedInt64(
		obj.UnstructuredContent(),
		strings.Split(pc.PathCheck.Path, ".")...,
	)
	if err != nil {
		return false, err
	}
	if !found {
		return false, errors.Errorf(
			"PathCheck %q failed: Path %q not found",
			pc.Name,
			pc.PathCheck.Path,
		)
	}
	val := pc.PathCheck.Value
	expected, ok := val.(int64)
	if !ok {
		return false, errors.Errorf(
			"PathCheck %q failed: %v is of type %T, expected int64",
			pc.Name,
			val,
			val,
		)
	}
	pc.result.Verbose = fmt.Sprintf(
		"Expected value %d got %d",
		expected,
		got,
	)
	if pc.retryIfValueEquals && got == expected {
		return false, nil
	}
	if pc.retryIfValueNotEquals && got != expected {
		return false, nil
	}
	if pc.retryIfValueNotGTE && got < expected {
		return false, nil
	}
	if pc.retryIfValueNotLTE && got > expected {
		return false, nil
	}
	// returning true will no longer retry
	return true, nil
}

func (pc *PathChecking) assertValueFloat64(obj *unstructured.Unstructured) (bool, error) {
	got, found, err := unstructured.NestedFloat64(
		obj.UnstructuredContent(),
		strings.Split(pc.PathCheck.Path, ".")...,
	)
	if err != nil {
		return false, err
	}
	if !found {
		return false, errors.Errorf(
			"PathCheck %q failed: Path %q not found",
			pc.Name,
			pc.PathCheck.Path,
		)
	}
	val := pc.PathCheck.Value
	expected, ok := val.(float64)
	if !ok {
		return false, errors.Errorf(
			"PathCheck %q failed: Value %v is of type %T, expected float64",
			pc.Name,
			val,
			val,
		)
	}
	pc.result.Verbose = fmt.Sprintf(
		"Expected value %f got %f",
		expected,
		got,
	)
	if pc.retryIfValueEquals && got == expected {
		return false, nil
	}
	if pc.retryIfValueNotEquals && got != expected {
		return false, nil
	}
	if pc.retryIfValueNotGTE && got < expected {
		return false, nil
	}
	if pc.retryIfValueNotLTE && got > expected {
		return false, nil
	}
	// returning true will no longer retry
	return true, nil
}

func (pc *PathChecking) assertValue(obj *unstructured.Unstructured) (bool, error) {
	if pc.PathCheck.DataType == PathValueDataTypeInt64 {
		return pc.assertValueInt64(obj)
	}
	// currently float & int64 are supported data types
	return pc.assertValueFloat64(obj)
}

func (pc *PathChecking) assertPath(obj *unstructured.Unstructured) (bool, error) {
	_, found, err := unstructured.NestedFieldNoCopy(
		obj.UnstructuredContent(),
		strings.Split(pc.PathCheck.Path, ".")...,
	)
	if err != nil {
		return false, err
	}
	if pc.retryIfPathNotExists && !found {
		return false, nil
	}
	if pc.retryIfPathExists && found {
		return false, nil
	}
	// returning true will no longer retry
	return true, nil
}

func (pc *PathChecking) assertPathAndValue(context string) (bool, error) {
	err := pc.Retry.Waitf(
		// returning true will stop retrying this func
		func() (bool, error) {
			client, err := pc.dynamicClientset.
				GetClientForAPIVersionAndKind(
					pc.State.GetAPIVersion(),
					pc.State.GetKind(),
				)
			if err != nil {
				return false, err
			}
			observed, err := client.
				Namespace(pc.State.GetNamespace()).
				Get(
					pc.State.GetName(),
					metav1.GetOptions{},
				)
			if err != nil {
				return false, err
			}
			if pc.pathOnlyCheck {
				return pc.assertPath(observed)
			}
			return pc.assertValue(observed)
		},
		context,
	)
	return err == nil, err
}

func (pc *PathChecking) assertPathExists() (success bool, err error) {
	var message = fmt.Sprintf("PathCheckExists: Resource %s %s: GVK %s: %s",
		pc.State.GetNamespace(),
		pc.State.GetName(),
		pc.State.GroupVersionKind(),
		pc.Name,
	)
	pc.result.Message = message
	// We want to retry if path does not exist in observed state.
	// This is done with the expectation of eventually having an
	// observed state with expected path
	pc.retryIfPathNotExists = true
	return pc.assertPathAndValue(message)
}

func (pc *PathChecking) assertPathNotExists() (success bool, err error) {
	var message = fmt.Sprintf("PathCheckNotExists: Resource %s %s: GVK %s: %s",
		pc.State.GetNamespace(),
		pc.State.GetName(),
		pc.State.GroupVersionKind(),
		pc.Name,
	)
	pc.result.Message = message
	// We want to retry if path does not exist in observed state.
	// This is done with the expectation of eventually having an
	// observed state with expected path
	pc.retryIfPathExists = true
	return pc.assertPathAndValue(message)
}

func (pc *PathChecking) assertPathValueNotEquals() (success bool, err error) {
	var message = fmt.Sprintf("PathCheckValueNotEquals: Resource %s %s: GVK %s: %s",
		pc.State.GetNamespace(),
		pc.State.GetName(),
		pc.State.GroupVersionKind(),
		pc.Name,
	)
	pc.result.Message = message
	// We want to retry if path values of expected & observed
	// match. This is done with the expectation of having
	// observed value not equal to the expected value eventually.
	pc.retryIfValueEquals = true
	return pc.assertPathAndValue(message)
}

func (pc *PathChecking) assertPathValueEquals() (success bool, err error) {
	var message = fmt.Sprintf("PathCheckValueEquals: Resource %s %s: GVK %s: %s",
		pc.State.GetNamespace(),
		pc.State.GetName(),
		pc.State.GroupVersionKind(),
		pc.Name,
	)
	pc.result.Message = message
	// We want to retry if path values of expected & observed
	// does not match. This is done with the expectation of having
	// observed value equal to the expected value eventually.
	pc.retryIfValueNotEquals = true
	return pc.assertPathAndValue(message)
}

func (pc *PathChecking) assertPathValueGTE() (success bool, err error) {
	var message = fmt.Sprintf("PathCheckValueGTE: Resource %s %s: GVK %s: %s",
		pc.State.GetNamespace(),
		pc.State.GetName(),
		pc.State.GroupVersionKind(),
		pc.Name,
	)
	pc.result.Message = message
	// We want to retry if path values of expected & observed
	// does not match. This is done with the expectation of having
	// observed value equal to the expected value eventually.
	pc.retryIfValueNotGTE = true
	return pc.assertPathAndValue(message)
}

func (pc *PathChecking) assertPathValueLTE() (success bool, err error) {
	var message = fmt.Sprintf("PathCheckValueLTE: Resource %s %s: GVK %s: %s",
		pc.State.GetNamespace(),
		pc.State.GetName(),
		pc.State.GroupVersionKind(),
		pc.Name,
	)
	pc.result.Message = message
	// We want to retry if path values of expected & observed
	// does not match. This is done with the expectation of having
	// observed value equal to the expected value eventually.
	pc.retryIfValueNotLTE = true
	return pc.assertPathAndValue(message)
}

func (pc *PathChecking) postAssert(success bool, err error) {
	if err != nil {
		pc.err = err
		return
	}
	// initialise phase to failed
	pc.result.Phase = PathCheckResultFailed
	if success {
		pc.result.Phase = PathCheckResultPassed
	}
}

func (pc *PathChecking) assert() {
	switch pc.operator {
	case PathCheckOperatorExists:
		pc.postAssert(pc.assertPathExists())

	case PathCheckOperatorNotExists:
		pc.postAssert(pc.assertPathNotExists())

	case PathCheckOperatorEquals:
		pc.postAssert(pc.assertPathValueEquals())

	case PathCheckOperatorNotEquals:
		pc.postAssert(pc.assertPathValueNotEquals())

	case PathCheckOperatorGTE:
		pc.postAssert(pc.assertPathValueGTE())

	case PathCheckOperatorLTE:
		pc.postAssert(pc.assertPathValueLTE())

	default:
		pc.err = errors.Errorf(
			"PathCheck %q failed: Invalid operator %q",
			pc.Name,
			pc.operator,
		)
	}
}

// Run executes the assertion
func (pc *PathChecking) Run() (PathCheckResult, error) {
	var fns = []func(){
		pc.init,
		pc.validate,
		pc.assert,
	}
	for _, fn := range fns {
		fn()
		if pc.err != nil {
			return PathCheckResult{}, errors.Wrapf(
				pc.err,
				"Info %s: Warn %s: Verbose %s",
				pc.result.Message,
				pc.result.Warning,
				pc.result.Verbose,
			)
		}
	}
	return *pc.result, nil
}
