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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/common"
)

// SyncHookRequest is the object sent as JSON to the sync hook.
type SyncHookRequest struct {
	// refers to this generic controller schema
	Controller *v1alpha1.GenericController `json:"controller"`

	// refers to the observed watch object due to the declaration
	// at the geneirc controller specs
	Watch *unstructured.Unstructured `json:"watch"`

	// refers to the filtered attachment objects due to the
	// declaration at the generic controller specs
	Attachments common.AnyUnstructRegistry `json:"attachments"`

	// Flag indicating if this request is for delete reconcile
	// and not create/update reconcile. This flag helps in
	// having single reconcile hook for create/update & delete.
	// It is upto the reconcile logic implementation to separate
	// create/update from delete logic.
	Finalizing bool `json:"finalizing"`
}

// SyncHookResponse is the expected format of the JSON response
// from the sync hook.
type SyncHookResponse struct {
	// desired labels to set against the watch resource
	Labels map[string]*string `json:"labels"`

	// desired annotations to set against the watch resource
	Annotations map[string]*string `json:"annotations"`

	// desired status to set against the watch resource
	Status map[string]interface{} `json:"status"`

	// desired state of all attachments
	Attachments []*unstructured.Unstructured `json:"attachments"`

	// indicate the controller if a resync is required after
	// the specified interval
	ResyncAfterSeconds float64 `json:"resyncAfterSeconds"`

	// When true skips reconciliation of attachments
	// This is expected to be set to a boolean value based
	// on runtime conditions while executing the hook logic
	SkipReconcile bool `json:"skipReconcile"`

	// Finalized should only be used by the finalize hook. If
	// true then this response will be applied by metacontroller.
	Finalized bool `json:"finalized"`
}

// HookInvoker manages invocation of hook. This understands inline
// hook invocation that is supported by generic controller
type HookInvoker struct {
	Schema *v1alpha1.Hook
}

// Invoke invokes the hook based on the given request & fills the
// response post successful invocation
func (i *HookInvoker) Invoke(req *SyncHookRequest, resp *SyncHookResponse) error {
	// if inline call then set appropriate call func
	if i.Schema.Inline != nil && i.Schema.Inline.FuncName != nil {
		// create a new instance of generic controller based inline hook invoker
		ihi, err := NewInlineHookInvoker(*i.Schema.Inline.FuncName)
		if err != nil {
			return err
		}
		return ihi.Invoke(req, resp)
	}
	// this is one of the commonly supported hooks
	return common.InvokeHook(i.Schema, req, resp)
}
