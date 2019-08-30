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

type GenericController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   GenericControllerSpec   `json:"spec"`
	Status GenericControllerStatus `json:"status,omitempty"`
}

// GenericControllerSpec is the specifications for GenericController
// API
type GenericControllerSpec struct {
	Watch       GenericControllerResource     `json:"watch"`
	Attachments []GenericControllerAttachment `json:"sync,omitempty"`

	// Hooks to be invoked to get at the desired state
	Hooks *GenericControllerHooks `json:"hooks,omitempty"`

	// ResyncPeriodSeconds is the time interval in seconds after which
	// the GenericController's reconcile gets triggered. In other words
	// this is the interval of reconciliation which runs as a continuous
	// loop
	ResyncPeriodSeconds *int32 `json:"resyncPeriodSeconds,omitempty"`

	// Parameters represent a set of optional key value pairs that
	// can take part while arriving at the desired state
	Parameters map[string]string `json:"parameters,omitempty"`
}

// GenericControllerHooks holds the sync as well as finalize hooks
type GenericControllerHooks struct {
	Sync     *Hook `json:"sync"`
	Finalize *Hook `json:"finalize,omitempty"`
}

// GenericControllerResource represent a resource that is understood
// by generic controller. It is used to represent a watch resource
// as well as attachment resources.
//
// NOTE:
// 	A watched resource and corresponding attachment resources can be
// arbitrary. In other words, a watched resource may not be a owner to the
// attachments mentioned in the generic controller resource. Similarly,
// attachments may not be filtered by the watched resource's selector
// property.
//
// NOTE:
// 	A watch as well as any attachment will have its own label selector &/
// annotation selector.
type GenericControllerResource struct {
	ResourceRule       `json:",inline"`
	NameSelector       []string              `json:"nameSelector,omitempty"`
	LabelSelector      *metav1.LabelSelector `json:"labelSelector,omitempty"`
	AnnotationSelector *AnnotationSelector   `json:"annotationSelector,omitempty"`
}

// GenericControllerAttachment represents a resources that takes
// part in sync &/or finalize actions
type GenericControllerAttachment struct {
	// This represents a single resource that participates in the
	// sync/finalize
	//
	// One can either use this or Group but not both
	GenericControllerResource `json:",inline"`

	// UpdateStrategy is the strategy to be used for the attachments
	// after the sync/finalize process
	UpdateStrategy *GenericControllerAttachmentUpdateStrategy `json:"updateStrategy,omitempty"`
}

// GenericControllerAttachmentUpdateStrategy represents the update strategy
// to be followed for the attachments
type GenericControllerAttachmentUpdateStrategy struct {
	Method ChildUpdateMethod `json:"method,omitempty"`
}

// GenericControllerStatusPhase represents various execution states
// supported by GenericController
type GenericControllerStatusPhase string

const (
	// GenericControllerStatusPhaseCompleted is used to indicate Running
	// state of GenericController
	GenericControllerStatusPhaseCompleted GenericControllerStatusPhase = "Running"

	// GenericControllerStatusPhaseError is used to indicate Error
	// state of GenericController
	GenericControllerStatusPhaseError GenericControllerStatusPhase = "Error"
)

// GenericControllerStatus represents the current state of this controller
type GenericControllerStatus struct {
	Phase      GenericControllerStatusPhase `json:"phase"`
	Conditions []GenericControllerCondition `json:"conditions,omitempty"`
}

// GenericControllerConditionState represents various execution states
// of a sync/finalize attachment supported by GenericController
type GenericControllerConditionState string

const (
	// GenericControllerConditionStateInProgress is used as a condition to indicate
	// InProgress state of any sync/finalize attachment
	GenericControllerConditionStateInProgress GenericControllerConditionState = "InProgress"

	// GenericControllerConditionStateError is used as a condition to indicate
	// error state of any sync/finalize attachment
	GenericControllerConditionStateError GenericControllerConditionState = "Error"
)

// GenericControllerConditionAssert represents various assertion states
// of a sync/finalize attachment supported by GenericController
type GenericControllerConditionAssert string

const (
	// GenericControllerConditionAssertPassed is used to state if a sync/finalize
	// attachment was successful during its assertion
	GenericControllerConditionAssertPassed GenericControllerConditionAssert = "Passed"

	// GenericControllerConditionAssertFailed is used to state if a sync/finalize
	// attachement failed during its assertion
	GenericControllerConditionAssertFailed GenericControllerConditionAssert = "Failed"
)

// GenericControllerCondition represents a condition that can be
// used to represent the current state of this controller. This can also
// be used to indicate if this controller can proceed further.
//
// Condition will be used only when it is required. It should be used
// sparingly to reduce the sync hot loop that gets kicked in when
// an observed resource is updated during its reconcile.
type GenericControllerCondition struct {
	// ID uniquely represents a condition from a list of conditions. This
	// can have a one-to-one mapping against each sync/finalize attachment.
	ID string `json:"id"`

	// State represents the execution status of a sync/finalize attachement
	// specified in this controller.
	State *GenericControllerConditionState `json:"state"`

	// Assert represents the assertion status of a sync/finalize attachement
	// specified in this controller.
	Assert *GenericControllerConditionAssert `json:"assert,omitempty"`

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

type GenericControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []GenericController `json:"items"`
}
