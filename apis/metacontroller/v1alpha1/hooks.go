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

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Hook refers to the logic that builds the desired
// state of resources
// +kubebuilder:object:generate=false
type Hook struct {
	// Webhook invocation to arrive at desired state
	Webhook *Webhook `json:"webhook,omitempty"`

	// Inline invocation to arrive at desired state
	Inline *Inline `json:"inline,omitempty"`
}

// Webhook refers to the logic that gets invoked as
// as web hook to arrive at the desired state
type Webhook struct {
	URL      *string           `json:"url,omitempty"`
	Timeout  *metav1.Duration  `json:"timeout,omitempty"`
	CABundle *string           `json:"caBundle,omitempty"`
	Path     *string           `json:"path,omitempty"`
	Service  *ServiceReference `json:"service,omitempty"`
}

// Inline refers to the logic that gets invoked as inline
// function call to arrive at the desired state.
//
// NOTE:
//	This works as a single binary that works via meta controller
// invoking this inline hook function.
type Inline struct {
	FuncName *string `json:"funcName,omitempty"`
}
