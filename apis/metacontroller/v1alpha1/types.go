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

// Package v1alpha1 contains API Schema definitions for the metac v1 API group
// +kubebuilder:object:generate=true
// +groupName=metac.openebs.io
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +genclient
// +genclient:noStatus
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cctl
type CompositeController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec CompositeControllerSpec `json:"spec"`
	// +optional
	Status CompositeControllerStatus `json:"status,omitempty"`
}

type CompositeControllerSpec struct {
	ParentResource CompositeControllerParentResourceRule  `json:"parentResource"`
	ChildResources []CompositeControllerChildResourceRule `json:"childResources,omitempty"`

	Hooks *CompositeControllerHooks `json:"hooks,omitempty"`

	ResyncPeriodSeconds *int32 `json:"resyncPeriodSeconds,omitempty"`
	GenerateSelector    *bool  `json:"generateSelector,omitempty"`
}

// ResourceRule helps in identifying the type of the API resource
type ResourceRule struct {
	// APIVersion is the combination of group & version
	// of the resource
	APIVersion string `json:"apiVersion"`

	// Resource is the name of the resource. Its also
	// the plural of Kind
	Resource string `json:"resource"`
}

type CompositeControllerParentResourceRule struct {
	ResourceRule    `json:",inline"`
	RevisionHistory *CompositeControllerRevisionHistory `json:"revisionHistory,omitempty"`
}

type CompositeControllerRevisionHistory struct {
	FieldPaths []string `json:"fieldPaths,omitempty"`
}

// ChildUpdateMethod represents a typed constant to determine
// the update strategy of a child resource
type ChildUpdateMethod string

const (
	// ChildUpdateOnDelete implies no update to the child resource;
	// it just lets this child be deleted by someone else
	ChildUpdateOnDelete ChildUpdateMethod = "OnDelete"

	// ChildUpdateRecreate implies delete this child resource
	// and recreate the same on next sync
	ChildUpdateRecreate ChildUpdateMethod = "Recreate"

	// ChildUpdateInPlace implies update the existing child resource
	ChildUpdateInPlace ChildUpdateMethod = "InPlace"

	// ChildUpdateRollingRecreate implies delete the child resource
	// and recreate the same on next sync
	ChildUpdateRollingRecreate ChildUpdateMethod = "RollingRecreate"

	// ChildUpdateRollingInPlace implies update the existing child resource
	ChildUpdateRollingInPlace ChildUpdateMethod = "RollingInPlace"
)

type CompositeControllerChildResourceRule struct {
	ResourceRule   `json:",inline"`
	UpdateStrategy *CompositeControllerChildUpdateStrategy `json:"updateStrategy,omitempty"`
}

type CompositeControllerChildUpdateStrategy struct {
	Method       ChildUpdateMethod       `json:"method,omitempty"`
	StatusChecks ChildUpdateStatusChecks `json:"statusChecks,omitempty"`
}

type ChildUpdateStatusChecks struct {
	Conditions []StatusConditionCheck `json:"conditions,omitempty"`
}

type StatusConditionCheck struct {
	Type   string  `json:"type"`
	Status *string `json:"status,omitempty"`
	Reason *string `json:"reason,omitempty"`
}

type ServiceReference struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Port      *int32  `json:"port,omitempty"`
	Protocol  *string `json:"protocol,omitempty"`
}

type CompositeControllerHooks struct {
	Sync     *Hook `json:"sync,omitempty"`
	Finalize *Hook `json:"finalize,omitempty"`

	PreUpdateChild  *Hook `json:"preUpdateChild,omitempty"`
	PostUpdateChild *Hook `json:"postUpdateChild,omitempty"`
}

type CompositeControllerStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
type CompositeControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CompositeController `json:"items"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
type ControllerRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	ParentPatch runtime.RawExtension         `json:"parentPatch"`
	Children    []ControllerRevisionChildren `json:"children,omitempty"`
}

type ControllerRevisionChildren struct {
	APIGroup string   `json:"apiGroup"`
	Kind     string   `json:"kind"`
	Names    []string `json:"names"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
type ControllerRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ControllerRevision `json:"items"`
}

// +genclient
// +genclient:noStatus
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=dctl
type DecoratorController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec DecoratorControllerSpec `json:"spec"`
	// +optional
	Status DecoratorControllerStatus `json:"status,omitempty"`
}

type DecoratorControllerSpec struct {
	Resources   []DecoratorControllerResourceRule   `json:"resources"`
	Attachments []DecoratorControllerAttachmentRule `json:"attachments,omitempty"`

	Hooks *DecoratorControllerHooks `json:"hooks,omitempty"`

	ResyncPeriodSeconds *int32 `json:"resyncPeriodSeconds,omitempty"`
}

type DecoratorControllerResourceRule struct {
	ResourceRule       `json:",inline"`
	LabelSelector      *metav1.LabelSelector `json:"labelSelector,omitempty"`
	AnnotationSelector *AnnotationSelector   `json:"annotationSelector,omitempty"`
}

type AnnotationSelector struct {
	MatchAnnotations map[string]string                 `json:"matchAnnotations,omitempty"`
	MatchExpressions []metav1.LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

type DecoratorControllerAttachmentRule struct {
	ResourceRule   `json:",inline"`
	UpdateStrategy *DecoratorControllerAttachmentUpdateStrategy `json:"updateStrategy,omitempty"`
}

type DecoratorControllerAttachmentUpdateStrategy struct {
	Method ChildUpdateMethod `json:"method,omitempty"`
}

type DecoratorControllerHooks struct {
	Sync     *Hook `json:"sync,omitempty"`
	Finalize *Hook `json:"finalize,omitempty"`
}

type DecoratorControllerStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
type DecoratorControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []DecoratorController `json:"items"`
}
