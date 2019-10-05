/*
Copyright 2018 Google Inc.

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

package common

import (
	"fmt"
	"reflect"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/diff"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	dynamicapply "openebs.io/metac/dynamic/apply"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// TODO (@amitkumardas):
//
// Some functions in this file should be deprecated in favour of
// manage_attachments.go

// Apply provides kubectl style apply logic
type Apply struct {
	// GetLastAppliedFn returns the last applied configuration
	// of the provided unstruct instance
	GetLastAppliedFn func(*unstructured.Unstructured) (map[string]interface{}, error)

	// SetLastAppliedFn sets the last applied configuration against
	// the provided unstruct instance
	SetLastAppliedFn func(obj *unstructured.Unstructured, lastApplied map[string]interface{}) error

	// SanitizeLastAppliedFn sanitizes the last applied configuration
	// to avoid recursive diff while executing apply logic
	//
	// NOTE:
	//	This is typically invoked before calling SetLastAppliedFn
	SanitizeLastAppliedFn func(lastApplied map[string]interface{})
}

// NewApplyFromAnnKey returns a new instance of Apply based on the provided
// annotation key
func NewApplyFromAnnKey(key string) *Apply {
	return &Apply{
		GetLastAppliedFn: func(o *unstructured.Unstructured) (map[string]interface{}, error) {
			return dynamicapply.GetLastAppliedByAnnKey(o, key)
		},
		SetLastAppliedFn: func(o *unstructured.Unstructured, last map[string]interface{}) error {
			return dynamicapply.SetLastAppliedByAnnKey(o, last, key)
		},
		SanitizeLastAppliedFn: func(last map[string]interface{}) {
			// delete the key that stores the last applied configuration
			// from the last applied configuration itself. This is needed
			// to break the chain of last applied states. In other words
			// this avoids last applied config storing details about previous
			// last applied state that in turn stores the details of its
			// previous last applied state & so on.
			dynamicapply.SanitizeLastAppliedByAnnKey(last, key)
		},
	}
}

// Merge applies the update against the original object in the
// style of kubectl apply
func (a *Apply) Merge(
	orig, update *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {

	// defaults to old way
	if a.GetLastAppliedFn == nil {
		a.GetLastAppliedFn = dynamicapply.GetLastApplied
	}
	if a.SetLastAppliedFn == nil {
		a.SetLastAppliedFn = dynamicapply.SetLastApplied
	}
	if a.SanitizeLastAppliedFn == nil {
		// a no-op
		a.SanitizeLastAppliedFn = func(l map[string]interface{}) {}
	}

	return a.merge(orig, update)
}

// merge applies the update against the original object in the
// style of kubectl apply
//
// TODO(@amitkumardas):
// ApplyMerge along with reflect.DeepEqual may be used to build
// assert logic based on observed yaml vs. desired yaml
func (a *Apply) merge(
	orig, update *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {

	// state that was last applied by this controller
	lastApplied, err := a.GetLastAppliedFn(orig)
	if err != nil {
		return nil, err
	}

	newObj := &unstructured.Unstructured{}
	newObj.Object, err = dynamicapply.Merge(
		orig.UnstructuredContent(),
		lastApplied,
		update.UnstructuredContent(),
	)
	if err != nil {
		return nil, err
	}

	// Revert metadata fields that are known to be read-only, system fields,
	// so that attempts to change those fields will never cause a diff to
	// be found by DeepEqual, which would cause needless, no-op updates or
	// recreates.
	//
	// See: https://github.com/GoogleCloudPlatform/metacontroller/issues/76
	if err := revertObjectMetaSystemFields(newObj, orig); err != nil {
		return nil, errors.Wrapf(err, "Failed to revert ObjectMeta system fields")
	}

	// Revert status because we don't currently support a parent changing
	// status of its children, so we need to ensure no diffs on the children
	// involve status.
	if err := revertField(newObj, orig, "status"); err != nil {
		return nil, errors.Wrapf(err, "Failed to revert .status")
	}

	// sanitize the last applied state before storing the same
	// in the newly formed object
	a.SanitizeLastAppliedFn(update.UnstructuredContent())
	a.SetLastAppliedFn(newObj, update.UnstructuredContent())

	return newObj, nil
}

// objectMetaSystemFields is a list of JSON field names within ObjectMeta
// that are both read-only and system-populated according to the comments in
// k8s.io/apimachinery/pkg/apis/meta/v1/types.go.
var objectMetaSystemFields = []string{
	"selfLink",
	"uid",
	"resourceVersion",
	"generation",
	"creationTimestamp",
	"deletionTimestamp",
}

// revertObjectMetaSystemFields overwrites the read-only, system-populated
// fields of ObjectMeta in newObj to match what they were in orig.
// If the field existed before, we create it if necessary and set the value.
// If the field was unset before, we delete it if necessary.
func revertObjectMetaSystemFields(newObj, orig *unstructured.Unstructured) error {
	for _, fieldName := range objectMetaSystemFields {
		if err := revertField(newObj, orig, "metadata", fieldName); err != nil {
			return err
		}
	}
	return nil
}

// revertField reverts field in newObj to match what it was in orig.
func revertField(newObj, orig *unstructured.Unstructured, fieldPath ...string) error {
	// check the field in original
	fieldVal, found, err :=
		unstructured.NestedFieldNoCopy(orig.UnstructuredContent(), fieldPath...)
	if err != nil {
		return errors.Wrapf(
			err,
			"Can't traverse UnstructuredContent to look for field %v", fieldPath,
		)
	}
	if found {
		// The original had this field set, so make sure it remains the same.
		// SetNestedField will recursively ensure the field and all its parent
		// fields exist, and then set the value.
		err := unstructured.SetNestedField(newObj.UnstructuredContent(), fieldVal, fieldPath...)
		if err != nil {
			return errors.Wrapf(err, "Can't revert field %v", fieldPath)
		}
	} else {
		// The original had this field unset, so make sure it remains unset.
		// RemoveNestedField is a no-op if the field or any of its parents
		// don't exist.
		unstructured.RemoveNestedField(newObj.UnstructuredContent(), fieldPath...)
	}
	return nil
}

// MakeOwnerRef builds & returns a new instance of OwnerReference
// from the given unstruct instance
func MakeOwnerRef(obj *unstructured.Unstructured) *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion:         obj.GetAPIVersion(),
		Kind:               obj.GetKind(),
		Name:               obj.GetName(),
		UID:                obj.GetUID(),
		Controller:         k8s.BoolPtr(true),
		BlockOwnerDeletion: k8s.BoolPtr(true),
	}
}

// ChildUpdateStrategyGetter provides the abstraction to figure out
// the required update strategy
type ChildUpdateStrategyGetter interface {
	Get(apiGroup, kind string) v1alpha1.ChildUpdateMethod
}

// ManageChildren ensures the relevant children objects of the
// given parent are in sync
//
// TODO (@amitkumardas) deprecate this in favour of
// AttachmentOperationManager's Apply method
func ManageChildren(
	dynClient *dynamicclientset.Clientset,
	updateStrategy ChildUpdateStrategyGetter,
	parent *unstructured.Unstructured,
	observedChildren, desiredChildren AnyUnstructRegistry,
) error {
	// If some operations fail, keep trying others so, for example,
	// we don't block recovery (create new Pod) on a failed delete.
	var errs []error

	// Delete observed, owned objects that are not desired.
	for key, objects := range observedChildren {
		apiVersion, kind := ParseKeyToAPIVersionKind(key)
		client, err := dynClient.GetClientByKind(apiVersion, kind)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := deleteChildren(client, parent, objects, desiredChildren[key]); err != nil {
			errs = append(errs, err)
			continue
		}
	}

	// Create or update desired objects.
	for key, objects := range desiredChildren {
		apiVersion, kind := ParseKeyToAPIVersionKind(key)
		client, err := dynClient.GetClientByKind(apiVersion, kind)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := updateChildren(
			client,
			updateStrategy,
			parent,
			observedChildren[key],
			objects,
		); err != nil {
			errs = append(errs, err)
			continue
		}
	}

	return utilerrors.NewAggregate(errs)
}

// TODO (@amitkumardas) deprecate this in favour of
// ControllerManager's Delete method
func deleteChildren(
	client *dynamicclientset.ResourceClient,
	parent *unstructured.Unstructured,
	observed, desired map[string]*unstructured.Unstructured,
) error {
	var errs []error
	for name, obj := range observed {
		if obj.GetDeletionTimestamp() != nil {
			// Skip objects that are already pending deletion.
			continue
		}
		if desired == nil || desired[name] == nil {
			// This observed object wasn't listed as desired.
			glog.Infof("%v: deleting %v", describeObject(parent), describeObject(obj))
			uid := obj.GetUID()
			// Explicitly request deletion propagation, which is what users expect,
			// since some objects default to orphaning for backwards compatibility.
			propagation := metav1.DeletePropagationBackground
			err := client.Namespace(obj.GetNamespace()).Delete(obj.GetName(), &metav1.DeleteOptions{
				Preconditions:     &metav1.Preconditions{UID: &uid},
				PropagationPolicy: &propagation,
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("can't delete %v: %v", describeObject(obj), err))
				continue
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

// TODO (@amitkumardas) deprecate this in favour of
// ControllerManager's Update method
func updateChildren(
	client *dynamicclientset.ResourceClient,
	updateStrategy ChildUpdateStrategyGetter,
	parent *unstructured.Unstructured,
	observed, desired map[string]*unstructured.Unstructured,
) error {
	var errs []error
	for name, obj := range desired {
		ns := obj.GetNamespace()
		if ns == "" {
			ns = parent.GetNamespace()
		}
		if oldObj := observed[name]; oldObj != nil {
			// Update
			a := Apply{}
			newObj, err := a.Merge(oldObj, obj)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			// Attempt an update, if the 3-way merge resulted in any changes.
			if reflect.DeepEqual(newObj.UnstructuredContent(), oldObj.UnstructuredContent()) {
				// Nothing changed.
				continue
			}
			if glog.V(5) {
				glog.Infof(
					"reflect diff: a=observed, b=desired:\n%s",
					diff.ObjectReflectDiff(
						oldObj.UnstructuredContent(), newObj.UnstructuredContent(),
					),
				)
			}

			// Leave it alone if it's pending deletion.
			if oldObj.GetDeletionTimestamp() != nil {
				glog.Infof(
					"%v: not updating %v (pending deletion)",
					describeObject(parent),
					describeObject(obj),
				)
				continue
			}

			// Check the update strategy for this child kind.
			switch method := updateStrategy.Get(client.Group, client.Kind); method {
			case v1alpha1.ChildUpdateOnDelete, "":
				// This means we don't try to update anything unless it gets deleted
				// by someone else (we won't delete it ourselves).
				glog.V(5).Infof(
					"%v: not updating %v (OnDelete update strategy)",
					describeObject(parent),
					describeObject(obj),
				)
				continue
			case v1alpha1.ChildUpdateRecreate, v1alpha1.ChildUpdateRollingRecreate:
				// Delete the object (now) and recreate it (on the next sync).
				glog.Infof(
					"%v: deleting %v for update", describeObject(parent), describeObject(obj),
				)
				uid := oldObj.GetUID()
				// Explicitly request deletion propagation, which is what users expect,
				// since some objects default to orphaning for backwards compatibility.
				propagation := metav1.DeletePropagationBackground
				err := client.Namespace(ns).Delete(obj.GetName(), &metav1.DeleteOptions{
					Preconditions:     &metav1.Preconditions{UID: &uid},
					PropagationPolicy: &propagation,
				})
				if err != nil {
					errs = append(errs, err)
					continue
				}
			case v1alpha1.ChildUpdateInPlace, v1alpha1.ChildUpdateRollingInPlace:
				// Update the object in-place.
				glog.Infof("%v: updating %v", describeObject(parent), describeObject(obj))
				if _, err := client.Namespace(ns).Update(newObj, metav1.UpdateOptions{}); err != nil {
					errs = append(errs, err)
					continue
				}
			default:
				errs = append(errs,
					fmt.Errorf(
						"invalid update strategy for %v: unknown method %q",
						client.Kind,
						method,
					),
				)
				continue
			}
		} else {
			// Create
			glog.Infof("%v: creating %v", describeObject(parent), describeObject(obj))

			// The controller should return a partial object containing only the
			// fields it cares about. We save this partial object so we can do
			// a 3-way merge upon update, in the style of "kubectl apply".
			//
			// Make sure this happens before we add anything else to the object.
			if err := dynamicapply.SetLastApplied(obj, obj.UnstructuredContent()); err != nil {
				errs = append(errs, err)
				continue
			}

			// We always claim everything we create.
			controllerRef := MakeOwnerRef(parent)
			ownerRefs := obj.GetOwnerReferences()
			ownerRefs = append(ownerRefs, *controllerRef)
			obj.SetOwnerReferences(ownerRefs)

			if _, err := client.Namespace(ns).Create(obj, metav1.CreateOptions{}); err != nil {
				errs = append(errs, err)
				continue
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}
