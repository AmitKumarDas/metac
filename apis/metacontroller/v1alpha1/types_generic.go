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

// GenericController defines GenericController API schema
type GenericController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   GenericControllerSpec   `json:"spec"`
	Status GenericControllerStatus `json:"status,omitempty"`
}

// GenericControllerSpec is the specifications for GenericController
// API
type GenericControllerSpec struct {
	// Resource that is under watch by GenericController. Any actions
	// i.e. 'create', 'update' or 'delete' of this resource will trigger
	// this GenericController's sync process.
	Watch GenericControllerResource `json:"watch"`

	// Attachments are the resources that may be read, created, updated,
	// or deleted as part of formation of the desired state. Attachments
	// are provided along with the watch resource to the sync hooks.
	//
	// NOTE:
	//	GenericController is by default limited to only update & delete
	// the attachments that were created by its controller instance. Other
	// attachments (i.e. the ones created via some other means) are used
	// for readonly purposes during reconciliation.
	Attachments []GenericControllerAttachment `json:"attachments,omitempty"`

	// Hooks to be invoked to arrive at the desired state
	Hooks *GenericControllerHooks `json:"hooks,omitempty"`

	// ResyncPeriodSeconds is the time interval in seconds after which
	// the GenericController's reconcile gets triggered. In other words
	// this is the interval of reconciliation which runs as a continuous
	// loop
	//
	// NOTE:
	//	This is optional
	ResyncPeriodSeconds *int32 `json:"resyncPeriodSeconds,omitempty"`

	// ReadOnly disables this controller from executing create, delete &
	// update operations against any attachments.
	//
	// In other words, when set to true, GenericController can update
	// only the watch resource & is disabled to perform any operation
	// i.e. 'create', 'delete' or 'update' against any attachments.
	//
	// This can be used by sync / finalize hook implementations to read
	// the attachments & update the watch. One should be able to perform
	// sync operations faster in this mode, if the requirement fits this
	// tunable.
	//
	// NOTE:
	//	This is optional. However this should not be set to true if
	// UpdateAny or DeleteAny is set to true.
	//
	// NOTE:
	// 	ReadOnly overrides UpdateAny and DeleteAny tunables
	ReadOnly *bool `json:"readOnly,omitempty"`

	// UpdateAny enables this controller to execute update operations
	// against any attachments.
	//
	// NOTE:
	//	This tunable changes the default working mode of GenericController.
	// When set to true, the controller instance is granted with the
	// permission to update any attachments even if these attachments
	// were not created by this controller instance.
	//
	// NOTE:
	//	This is optional. However this should not be set to true if
	// ReadOnly is set to true.
	UpdateAny *bool `json:"updateAny,omitempty"`

	// DeleteAny enables this controller to execute delete operations
	// against any attachments.
	//
	// NOTE:
	//	This tunable changes the default working mode of GenericController.
	// When set to true, the controller instance is granted with the
	// permission to delete any attachments even if these attachments
	// were not created by this controller instance.
	//
	// NOTE:
	//	This is optional. However this should not be set to true if
	// ReadOnly is set to true.
	DeleteAny *bool `json:"deleteAny,omitempty"`

	// Parameters represent a set of key value pairs that can be used by
	// the sync hook implementation logic.
	//
	// NOTE:
	//	This is optional
	Parameters map[string]string `json:"parameters,omitempty"`
}

// GenericControllerHooks holds the sync as well as finalize hooks
type GenericControllerHooks struct {
	// Hook that gets invoked during create/update reconciliation
	Sync *Hook `json:"sync,omitempty"`

	// Hook that gets invoked during delete reconciliation
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
	ResourceRule `json:",inline"`

	// Include the resource if name selector matches
	NameSelector NameSelector `json:"nameSelector,omitempty"`

	// Include the resource if label selector matches
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// Include the resource if annotation selector matches
	AnnotationSelector *AnnotationSelector `json:"annotationSelector,omitempty"`
}

// GenericControllerAttachment represents a resources that takes
// part in sync &/or finalize.
type GenericControllerAttachment struct {
	// This represents the resource that should participates in
	// sync/finalize
	GenericControllerResource `json:",inline"`

	// UpdateStrategy to be used for the resource to take into
	// account the changes due to sync/finalize
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

// GenericControllerList is a collection of GenericController API schemas
type GenericControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []GenericController `json:"items"`
}

// Key formats the GenericController value into a
// suitable key format
func (gc GenericController) Key() string {
	return GenericControllerKey(gc.Namespace, gc.Name)
}

// GenericControllerKey returns key formatted type for the
// given namespace & name values
func GenericControllerKey(namespace, name string) string {
	return namespace + "/" + name
}
