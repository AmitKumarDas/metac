/*
Copyright 2018 Google Inc.
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
	"github.com/pkg/errors"
)

// Invoker enables invocation of appropriate hook
type Invoker struct {
	// InvokeFn abstracts invocation of hook. Typically specific
	// hook implementors will have their call methods set here
	InvokeFn func(request, response interface{}) error
}

// InvokerOption is a typed function that helps in building
// up the Invoker instance
//
// This follows the pattern called "functional options"
type InvokerOption func(*Invoker) error

// NewInvoker returns a new instance of Invoker.
// This requires at-least one option to be sent from
// its callers.
func NewInvoker(must InvokerOption, others ...InvokerOption) (*Invoker, error) {

	var options = []InvokerOption{must}
	options = append(options, others...)

	i := &Invoker{}
	for _, o := range options {
		o(i)
	}

	if i.InvokeFn == nil {
		return nil, errors.Errorf("Invoke func can't be nil")
	}
	return i, nil
}

// Invoke invokes the hook
func (c *Invoker) Invoke(request, response interface{}) error {
	return c.InvokeFn(request, response)
}
