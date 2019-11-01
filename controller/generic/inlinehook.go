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

package generic

import (
	"sync"

	"github.com/pkg/errors"
)

// InlineInvokeFn is the signature for all inline hook invocation functions
type InlineInvokeFn func(req *SyncHookRequest, resp *SyncHookResponse) error

type inlineHookRegistry struct {
	sync.Mutex
	invokeFuncs map[string]InlineInvokeFn
}

var inlineHookRegistryInstance = &inlineHookRegistry{
	invokeFuncs: make(map[string]InlineInvokeFn),
}

// AddToInlineRegistry will add function name and correponding
// function to inline hook registry
func AddToInlineRegistry(funcName string, fn InlineInvokeFn) {
	inlineHookRegistryInstance.Lock()
	defer inlineHookRegistryInstance.Unlock()
	inlineHookRegistryInstance.invokeFuncs[funcName] = fn
}

// InlineHookInvoker manages invocation of inline hook
type InlineHookInvoker struct {
	FuncName string
}

// NewInlineHookInvoker returns a new instance of inline hook invoker
func NewInlineHookInvoker(funcName string) (*InlineHookInvoker, error) {
	if funcName == "" {
		return nil,
			errors.Errorf("Inline invoker function name can't be empty")
	}
	return &InlineHookInvoker{FuncName: funcName}, nil
}

// Invoke this inline hook by passing the given request
// and fill up the given response with the hook's response
func (i *InlineHookInvoker) Invoke(req *SyncHookRequest, resp *SyncHookResponse) error {
	fn := inlineHookRegistryInstance.invokeFuncs[i.FuncName]
	if fn == nil {
		return errors.Errorf(
			"Inline hook function not found for %s", i.FuncName,
		)
	}
	return fn(req, resp)
}
