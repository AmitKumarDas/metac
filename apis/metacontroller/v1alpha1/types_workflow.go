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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   WorkflowControllerSpec   `json:"spec"`
	Status WorkflowControllerStatus `json:"status,omitempty"`
}

// WorkflowControllerSpec is the specifications for WorkflowController
// API
type WorkflowControllerSpec struct {
	Watch    WorkflowControllerResource     `json:"watch"`
	Options  WorkflowControllerOptions      `json:"options,omitempty"`
	Sync     []WorkflowControllerAttachment `json:"sync,omitempty"`
	Finalize []WorkflowControllerAttachment `json:"finalize,omitempty"`
}

// WorkflowControllerOptions has various tunables, options, parameters
// etc required to execute this WorkflowController
type WorkflowControllerOptions struct {
	// ResyncPeriodSeconds is the time interval in seconds after which
	// the WorkflowController's reconcile gets triggered. In other words
	// this is the interval of reconciliation which runs as a continuous
	// loop
	ResyncPeriodSeconds *int32 `json:"resyncPeriodSeconds,omitempty"`

	// ConfigHook is the default logic used for each sync &/ finalize
	// attachments. This is optional.
	ConfigHook *ConfigHook `json:"configHook,omitempty"`
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
// hook is invoked for each sync/finalize action within this
// WorkflowController.
//
// A ConfigHook differs from a webhook by being defined in some config file
// that can be loaded/fetched by the binary running this WorkflowController.
// For example, the confighook can be defined in a Kubernetes ConfigMap and
// can be fetched by the watcher that reconciles the WorkflowController
// resource.
type ConfigHook struct {
	Type *ConfigHookType `json:"type,omitempty"`
	Name string          `json:"name"`
}

// WorkflowControllerResource represent a resource that is understood
// by workflow controller. It is used to represent a watch resource
// as well as attachment resources.
//
// NOTE:
// 	A watched resource and corresponding attachment resources can be
// arbitrary. In other words, a watched resource may not be a owner to the
// attachments mentioned in a workflow controller. Similarly, attachments
// may not be filtered by the watched resource's selector property.
//
// NOTE:
// 	A watch as well as any attachment will have its own label selector &/
// annotation selector.
type WorkflowControllerResource struct {
	ResourceRule       `json:",inline"`
	NameSelector       []string              `json:"nameSelector,omitempty"`
	LabelSelector      *metav1.LabelSelector `json:"labelSelector,omitempty"`
	AnnotationSelector *AnnotationSelector   `json:"annotationSelector,omitempty"`
}

// WorkflowControllerAttachment represents one or more resources that can
// take part in sync or finalize actions
//
// NOTE:
//	One can only specify either a single attachment resource in this
// structure or a group of resources via this structure's group property.
type WorkflowControllerAttachment struct {
	// ID enables unique identification of this attachment from a list
	// of others
	ID string `json:"id"`

	// Desc can be used to provide descriptive information about this
	// attachment(s) or can be used to describe more about this sync/finalize
	Desc string `json:"desc"`

	// This represents a single resource that participates in the
	// sync/finalize
	//
	// One can either use this or Group but not both
	WorkflowControllerResource `json:",inline"`

	// Group represents a list of resources that participate in the
	// sync/finalize
	//
	// One can either use this or WorkflowControllerResource but not
	// both
	Group []WorkflowControllerResource `json:"group"`

	// UpdateStrategy is the strategy to be used for the attachments
	// after the sync/finalize process
	UpdateStrategy *WorkflowControllerAttachmentUpdateStrategy `json:"updateStrategy,omitempty"`

	// ConfigHook represents the logic to transform the resource(s) of this
	// attachment into their desired state
	ConfigHook *ConfigHook `json:"configHook,omitempty"`
}

// WorkflowControllerAttachmentUpdateStrategy represents the update strategy
// to be followed for the attachments
type WorkflowControllerAttachmentUpdateStrategy struct {
	Method ChildUpdateMethod `json:"method,omitempty"`
}

// WorkflowControllerStatusPhase represents various execution states
// supported by WorkflowController
type WorkflowControllerStatusPhase string

const (
	// WorkflowControllerStatusPhaseCompleted is used to indicate Running
	// state of WorkflowController
	WorkflowControllerStatusPhaseCompleted WorkflowControllerStatusPhase = "Running"

	// WorkflowControllerStatusPhaseError is used to indicate Error
	// state of WorkflowController
	WorkflowControllerStatusPhaseError WorkflowControllerStatusPhase = "Error"
)

// WorkflowControllerStatus represents the current state of this controller
type WorkflowControllerStatus struct {
	Phase      WorkflowControllerStatusPhase `json:"phase"`
	Conditions []WorkflowControllerCondition `json:"conditions,omitempty"`
}

// WorkflowControllerConditionPhase represents various execution states
// of a sync/finalize attachment supported by WorkflowController
type WorkflowControllerConditionPhase string

const (
	// WorkflowControllerConditionPhaseInProgress is used as a condition to indicate
	// InProgress state of any sync/finalize attachment
	WorkflowControllerConditionPhaseInProgress WorkflowControllerConditionPhase = "InProgress"

	// WorkflowControllerConditionPhaseError is used as a condition to indicate
	// error state of any sync/finalize attachment
	WorkflowControllerConditionPhaseError WorkflowControllerConditionPhase = "Error"
)

// WorkflowControllerConditionAssert represents various assertion states
// of a sync/finalize attachment supported by WorkflowController
type WorkflowControllerConditionAssert string

const (
	// WorkflowControllerConditionAssertPassed is used to state if a sync/finalize
	// attachment was successful during its assertion
	WorkflowControllerConditionAssertPassed WorkflowControllerConditionAssert = "Passed"

	// WorkflowControllerConditionAssertFailed is used to state if a sync/finalize
	// attachement failed during its assertion
	WorkflowControllerConditionAssertFailed WorkflowControllerConditionAssert = "Failed"
)

// WorkflowControllerCondition represents a condition that can be
// used to represent the current state of this workflow. This can also
// be used to indicate if this workflow can proceed further.
//
// Condition will be used only when it is required. It should be used
// sparingly to reduce the sync hot loop that gets kicked in when
// an observed resource is updated during its reconcile.
type WorkflowControllerCondition struct {
	// ID uniquely represents a condition from a list of conditions. This
	// can have a one-to-one mapping against each sync/finalize attachment.
	ID string `json:"id"`

	// State represents the execution status of a sync/finalize attachement specified
	// in this controller.
	Phase *WorkflowControllerConditionPhase `json:"phase"`

	// Assert represents the assertion status of a sync/finalize attachement specified
	// in this controller.
	Assert *WorkflowControllerConditionAssert `json:"assert,omitempty"`

	// Descriptive message about this condition
	Message string `json:"message,omitempty"`

	// Error message if any about this condition
	Error string `json:"error,omitempty"`

	// Help message if any to recover from this error
	Help string `json:"help,omitempty"`

	// LastUpdatedTimestamp is the last timestamp when this
	// condition got added/updated
	LastUpdatedTimestamp *metav1.Time `json:"lastUpdatedTimestamp,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type WorkflowControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []WorkflowController `json:"items"`
}
