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

package hooks

import (
	"fmt"
)

// GoTemplateManager manages execution of GoTemplate
type GoTemplateManager struct {
	// name of GoTemplate
	Name string

	// namespace of GoTemplate
	Namespace string

	// actual template body that will be executed by
	// go template executor
	Template string
}

// GoTemplateManagerOption is a typed function that is meant to
// build an instance of *GoTemplateManager
//
// NOTE:
//	This follows "functional options" pattern
type GoTemplateManagerOption func(*GoTemplateManager) error

// NewGoTemplateManager returns a new instance of GoTemplateManager
// based on the provided options
func NewGoTemplateManager(opts ...GoTemplateManagerOption) (*GoTemplateManager, error) {
	mgr := &GoTemplateManager{}
	for _, o := range opts {
		err := o(mgr)
		if err != nil {
			return nil, err
		}
	}
	return mgr, nil
}

// String implements Stringer interface
func (mgr *GoTemplateManager) String() string {
	return fmt.Sprintf("GoTemplate %s/%s", mgr.Namespace, mgr.Name)
}

// Invoke this webhook by using the provided request and fill up
// the response with the provided response object
func (mgr *GoTemplateManager) Invoke(request, response interface{}) error {
	return nil
}
