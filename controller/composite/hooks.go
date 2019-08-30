/*
Copyright 2017 Google Inc.

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

package composite

import (
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/common"
)

// SyncHookRequest is the object sent as JSON to the sync hook.
type SyncHookRequest struct {
	Controller *v1alpha1.CompositeController `json:"controller"`
	Parent     *unstructured.Unstructured    `json:"parent"`
	Children   common.ChildMap               `json:"children"`
	Finalizing bool                          `json:"finalizing"`
}

// String implements Stringer interface
func (r *SyncHookRequest) String() string {
	if r.Parent == nil {
		return "SyncHookRequest"
	}
	return fmt.Sprintf(
		"SyncHookRequest %s/%s of %s",
		r.Parent.GetNamespace(), r.Parent.GetName(), r.Parent.GroupVersionKind(),
	)
}

// NewSyncHookRequest returns a new instance of SyncHookRequest
func NewSyncHookRequest(
	parent *unstructured.Unstructured,
	children common.ChildMap,
) *SyncHookRequest {
	return &SyncHookRequest{
		Parent:   parent,
		Children: children,
	}
}

// SyncHookResponse is the expected format of the JSON response
// from the sync hook.
type SyncHookResponse struct {
	Status   map[string]interface{}       `json:"status"`
	Children []*unstructured.Unstructured `json:"children"`

	ResyncAfterSeconds float64 `json:"resyncAfterSeconds"`

	// Finalized is only used by the finalize hook.
	Finalized bool `json:"finalized"`
}

// HookExecutor can execute a hook
type HookExecutor struct {
	Controller *v1alpha1.CompositeController
}

// String implements Stringer interface
func (e *HookExecutor) String() string {
	if e.Controller == nil {
		return "HookExecutor"
	}
	return fmt.Sprintf(
		"HookExecutor %s/%s of %s",
		e.Controller.Namespace, e.Controller.Name, e.Controller.Kind,
	)
}

// Execute invokes the hook for the given request
func (e *HookExecutor) Execute(req *SyncHookRequest) (*SyncHookResponse, error) {
	if e.Controller == nil || e.Controller.Spec.Hooks == nil {
		return nil, errors.Errorf("%s: Execute failed: Nil hooks", e)
	}
	req.Controller = e.Controller

	var resp SyncHookResponse

	// First check if we should instead call the finalize hook,
	// which has the same API as the sync hook except that it's
	// called while the object is pending deletion.
	if req.Parent.GetDeletionTimestamp() != nil &&
		e.Controller.Spec.Hooks.Finalize != nil {
		// Finalize
		req.Finalizing = true
		err := common.CallHook(e.Controller.Spec.Hooks.Finalize, req, &resp)
		if err != nil {
			return nil, errors.Wrapf(err, "%s: Finalize hook failed for %s", e, req)
		}
	} else {
		// Sync
		req.Finalizing = false
		if e.Controller.Spec.Hooks.Sync == nil {
			return nil,
				errors.Errorf("%s: Sync hook not defined for %s", e, req)
		}

		err := common.CallHook(e.Controller.Spec.Hooks.Sync, req, &resp)
		if err != nil {
			return nil,
				errors.Wrapf(err, "%s: Sync hook failed for %s", e, req)
		}
	}

	return &resp, nil
}

func callSyncHook(
	controller *v1alpha1.CompositeController,
	request *SyncHookRequest,
) (*SyncHookResponse, error) {
	e := HookExecutor{Controller: controller}
	return e.Execute(request)
}
