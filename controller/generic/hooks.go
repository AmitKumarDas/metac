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

	// Finalized should only be used by the finalize hook. If
	// true then this response will be applied by metacontroller.
	Finalized bool `json:"finalized"`
}
