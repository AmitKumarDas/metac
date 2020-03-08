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

package generic

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"openebs.io/metac/apis/metacontroller/v1alpha1"
)

func TestNewConfigMetaController(t *testing.T) {
	var tests = map[string]struct {
		options []ConfigMetaControllerOption
		isErr   bool
	}{
		"no options": {
			isErr: true,
		},
		"with invalid config path": {
			options: []ConfigMetaControllerOption{
				func(ctl *ConfigMetaController) (err error) {
					ctl.ConfigPath = "/etc/config/metac/"
					return
				},
			},
			isErr: true,
		},
		"with testdata as config path": {
			options: []ConfigMetaControllerOption{
				func(ctl *ConfigMetaController) (err error) {
					ctl.ConfigPath = "testdata/"
					return
				},
			},
			isErr: false,
		},
		"with config loader func": {
			options: []ConfigMetaControllerOption{
				func(ctl *ConfigMetaController) (err error) {
					ctl.ConfigLoadFn = func() ([]*v1alpha1.GenericController, error) {
						return nil, nil
					}
					return
				},
			},
			isErr: false,
		},
		"with erroring config loader func": {
			options: []ConfigMetaControllerOption{
				func(ctl *ConfigMetaController) (err error) {
					ctl.ConfigLoadFn = func() ([]*v1alpha1.GenericController, error) {
						return nil, errors.Errorf("Err")
					}
					return
				},
			},
			isErr: true,
		},
	}
	for name, mock := range tests {
		name := name
		mock := mock
		t.Run(name, func(t *testing.T) {
			_, err := NewConfigMetaController(
				nil,
				nil,
				nil,
				0,
				mock.options...,
			)
			if mock.isErr && err == nil {
				t.Fatalf("Expected error got none")
			}
			if !mock.isErr && err != nil {
				t.Fatalf("Expected no error got [%+v]", err)
			}
		})
	}
}

func TestConfigMetaControllerIsDuplicateConfig(t *testing.T) {
	var tests = map[string]struct {
		Controller *ConfigMetaController
		isErr      bool
	}{
		"nil configs": {
			Controller: &ConfigMetaController{},
			isErr:      false,
		},
		"empty configs": {
			Controller: &ConfigMetaController{
				Configs: []*v1alpha1.GenericController{},
			},
			isErr: false,
		},
		"1 config": {
			Controller: &ConfigMetaController{
				Configs: []*v1alpha1.GenericController{
					&v1alpha1.GenericController{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "test",
						},
					},
				},
			},
			isErr: false,
		},
		"duplicate configs": {
			Controller: &ConfigMetaController{
				Configs: []*v1alpha1.GenericController{
					&v1alpha1.GenericController{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "test",
						},
					},
					&v1alpha1.GenericController{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: "test",
						},
					},
				},
			},
			isErr: true,
		},
	}
	for name, mock := range tests {
		name := name
		mock := mock
		t.Run(name, func(t *testing.T) {
			ctl := mock.Controller
			ctl.isDuplicateConfig()
			if mock.isErr && ctl.err == nil {
				t.Fatalf("Expected error got none")
			}
			if !mock.isErr && ctl.err != nil {
				t.Fatalf("Expected no error got [%+v]", ctl.err)
			}
		})
	}
}

func TestConfigMetaControllerWait(t *testing.T) {
	var tests = map[string]struct {
		controller *ConfigMetaController
		condition  func() (bool, error)
		isErr      bool
	}{
		"cond returns error": {
			controller: &ConfigMetaController{
				WaitIntervalForCondition: 1 * time.Second,
				WaitTimeoutForCondition:  5 * time.Second,
			},
			condition: func() (bool, error) {
				return false, errors.Errorf("Err")
			},
			isErr: true,
		},
		"cond returns true": {
			controller: &ConfigMetaController{
				WaitIntervalForCondition: 1 * time.Second,
				WaitTimeoutForCondition:  5 * time.Second,
			},
			condition: func() (bool, error) {
				return true, nil
			},
			isErr: false,
		},
		"cond returns false": {
			controller: &ConfigMetaController{
				WaitIntervalForCondition: 1 * time.Second,
				WaitTimeoutForCondition:  5 * time.Second,
			},
			condition: func() (bool, error) {
				return false, nil
			},
			isErr: false,
		},
	}
	for name, mock := range tests {
		name := name
		mock := mock
		t.Run(name, func(t *testing.T) {
			ctl := mock.controller
			err := ctl.wait(mock.condition)
			if mock.isErr && err == nil {
				t.Fatalf("Expected error got none")
			}
			if !mock.isErr && err != nil {
				t.Fatalf("Expected no error got [%+v]", err)
			}
		})
	}
}
