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

// HookCaller enables invocation of appropriate hook
type HookCaller struct {
	// CallFn abstracts invocation of hook. Typically specific
	// hook implementors will have their call methods set here
	CallFn func(request, response interface{}) error
}

// HookCallerOption is a typed function that helps in building
// up the HookCaller instance
//
// This follows the pattern called "functional options"
type HookCallerOption func(*HookCaller) error

// NewHookCaller returns a new instance of HookCaller
// This requires at-least one option to be sent from
// its callers.
func NewHookCaller(must HookCallerOption, others ...HookCallerOption) (*HookCaller, error) {

	var options = []HookCallerOption{must}
	options = append(options, others...)

	c := &HookCaller{}
	for _, o := range options {
		o(c)
	}

	if c.CallFn == nil {
		return nil, errors.Errorf("Invalid hook: Nil CallFunc")
	}
	return c, nil
}

// Call invokes the hook
func (c *HookCaller) Call(request, response interface{}) error {
	return c.CallFn(request, response)
}
