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

	// Go templating based desired state
	GoTemplate *GoTemplateHook `json:"gotemplate,omitempty"`

	// Jsonnet based desired state
	Jsonnet *JsonnetHook `json:"jsonnet,omitempty"`

	// Job based desired state
	Job *JobHook `json:"job,omitempty"`
}

// Webhook refers to the logic that needs to be invoked
// as a web hook to get the desired state of resources
type Webhook struct {
	URL     *string          `json:"url,omitempty"`
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	Path    *string           `json:"path,omitempty"`
	Service *ServiceReference `json:"service,omitempty"`
}

// JobHook represents the logic to derive the desired state. A job
// based hook can be invoked for a sync/finalize action.
//
// A JobHook is a Kubernetes Job API that can be applied by Metac
// binary itself.
type JobHook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	// Image refers to container image
	Image string `json:"image"`
}

// GoTemplateHook represents the logic that helps in achieving the
// desired state. This logic written as a go template. Metac fetches
// this go template from Kubernetes ConfigMap. Metac has a go template
// parser to parse the same.
type GoTemplateHook ConfigMap

// JsonnetHook represents the logic that helps in achieving the
// desired state. This logic is written in jsonnet. Metac fetches
// this jsonnet from Kubernetes ConfigMap and has a jsonnet parser
// to parse the same.
type JsonnetHook ConfigMap

// ConfigMap refers to a Kubernetes ConfigMap
type ConfigMap struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace, omitempty"`
}
