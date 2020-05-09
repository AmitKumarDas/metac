/*
Copyright 2018 Google Inc.
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

package finalizer

import (
	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	dynamicclientset "openebs.io/metac/dynamic/clientset"
	dynamicobject "openebs.io/metac/dynamic/object"
)

// Finalizer manages updating metacontroller finalizer value
// against the target resource's finalizers field
type Finalizer struct {
	// Name of the finalizer that needs to be set or unset
	Name string

	// Boolean that flags if finalizer should be added or removed
	Enabled bool
}

// SyncObject reconciles i.e. adds or removes the finalizer on
// the given object as necessary.
func (m *Finalizer) SyncObject(
	client *dynamicclientset.ResourceClient,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	// If the cached object passed in is already in the right state,
	// we'll assume we don't need to check the live object.
	//
	// NOTE:
	//	Right state here can imply either of below:
	//
	// - Finalizer is desired and this object has the finalizer
	// - Finalizer is not desired & this object does not have the finalizer
	if dynamicobject.HasFinalizer(obj, m.Name) == m.Enabled {
		return obj, nil
	}
	// Otherwise, we may need to update the object.
	//
	// NOTE:
	// 	Enabled is typically set by the caller, when metacontroller
	// specs has a finalizer hook
	if m.Enabled {
		// If the object is already pending deletion, we don't add
		// the finalizer. We might have already removed it.
		if obj.GetDeletionTimestamp() != nil {
			glog.V(6).Infof(
				"Won't add finalizer %s: Resource %s %s is pending deletion",
				m.Name,
				obj.GetNamespace(),
				obj.GetName(),
			)
			return obj, nil
		}
		return client.Namespace(obj.GetNamespace()).AddFinalizer(obj, m.Name)
	}
	return client.Namespace(obj.GetNamespace()).RemoveFinalizer(obj, m.Name)
}

// ShouldFinalize returns true if the controller should take action
// to manage children even though the parent is pending deletion
// (i.e. finalize).
func (m *Finalizer) ShouldFinalize(parent metav1.Object) bool {
	// There's no point managing children if the parent has a
	// GC finalizer, because we'd be fighting the GC.
	if hasGCFinalizer(parent) {
		glog.V(7).Infof(
			"Resource has GC finalizer(s) %v: Resource %s %s",
			parent.GetFinalizers(),
			parent.GetNamespace(),
			parent.GetName(),
		)
		return false
	}
	// If we already removed the finalizer, don't try to manage children anymore.
	if !dynamicobject.HasFinalizer(parent, m.Name) {
		glog.V(7).Infof(
			"Finalizer %s not found: Resource %s %s",
			m.Name,
			parent.GetNamespace(),
			parent.GetName(),
		)
		return false
	}
	return m.Enabled
}

// RemoveFinalizer removes the finalizer on the given object
//
// NOTE:
//	This mutates the given object and does not update this state
// at the cluster.
func (m *Finalizer) RemoveFinalizer(obj *unstructured.Unstructured) {
	dynamicobject.RemoveFinalizer(obj, m.Name)
}

// hasGCFinalizer returns true if obj has any GC finalizer.
// In other words, true means the GC will start messing with its children,
// either deleting or orphaning them.
func hasGCFinalizer(obj metav1.Object) bool {
	for _, item := range obj.GetFinalizers() {
		switch item {
		case metav1.FinalizerDeleteDependents, metav1.FinalizerOrphanDependents:
			return true
		}
	}
	return false
}
