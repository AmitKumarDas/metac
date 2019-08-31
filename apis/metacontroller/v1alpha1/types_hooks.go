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
type Hook struct {
	// Webhook based desired state
	Webhook *Webhook `json:"webhook,omitempty"`

	// ConfigHook based desired state
	ConfigHook *ConfigHook `json:"confighook,omitempty"`
}

// Webhook refers to the logic that needs to be invoked
// as a web hook to get the desired state of resources
type Webhook struct {
	URL     *string          `json:"url,omitempty"`
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	Path    *string           `json:"path,omitempty"`
	Service *ServiceReference `json:"service,omitempty"`
}

// ConfigHookType represents the supported config based hooks
type ConfigHookType string

const (
	// ConfigHookTypeJSON represents Jsonnet based logic to derive
	// the desired state
	ConfigHookTypeJSON ConfigHookType = "Jsonnet"

	// ConfigHookTypeGoTemplate represent Go template based logic
	// to derive the desired state
	ConfigHookTypeGoTemplate ConfigHookType = "GoTemplate"
)

// ConfigHook represents the logic to derive the desired state. A config
// hook can be invoked for a sync/finalize action.
//
// A ConfigHook differs from a webhook by being defined in some config file
// that can be loaded/fetched by Metac binary itself.
//
// For example, a confighook can be defined as a Kubernetes ConfigMap and
// can be fetched by the watcher that reconciles any Metac controller
// resource.
type ConfigHook struct {
	Type *ConfigHookType `json:"type,omitempty"`
	Name string          `json:"name"`
}
