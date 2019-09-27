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
	Controller  *v1alpha1.GenericController `json:"controller"`
	Object      *unstructured.Unstructured  `json:"object"`
	Attachments common.AnyUnstructRegistry  `json:"attachments"`
	Finalizing  bool                        `json:"finalizing"`
}

// SyncHookResponse is the expected format of the JSON response from the sync hook.
type SyncHookResponse struct {
	Labels      map[string]*string           `json:"labels"`
	Annotations map[string]*string           `json:"annotations"`
	Status      map[string]interface{}       `json:"status"`
	Attachments []*unstructured.Unstructured `json:"attachments"`

	ResyncAfterSeconds float64 `json:"resyncAfterSeconds"`

	// Finalized is only used by the finalize hook.
	Finalized bool `json:"finalized"`
}
