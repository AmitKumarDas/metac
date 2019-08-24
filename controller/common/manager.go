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
)

// BaseManager holds the properties required for various controller
// manager operations
type BaseManager struct {
	// UpdateStrategy to be followed while executing update
	// operation against a child resource
	UpdateStrategy ChildUpdateStrategy

	// Resource that is being watched
	Watched *unstructured.Unstructured

	// IsWatchOwner indicates if the watched resource is also the
	// owner (of the child resources declared in the controller)
	IsWatchOwner bool
}

// ControllerManager manages applying metac controller child resources
// in Kubernetes cluster. Here apply implies either Create or Update or
// Delete of child resources against a given Kubernetes cluster.
type ControllerManager struct {
	BaseManager

	// DynClientSet is responsible to provide a dynamic client for
	// a specific resource
	DynClientSet *dynamicclientset.Clientset

	// Observed state (i.e. current state in kubernetes) of child
	// resources referred to in the controller
	Observed ChildMap

	// Desired kubernetes state of child resources referred to in
	// the controller
	Desired ChildMap

	// error as value
	err error

	// Various executioners required to execute apply
	Deleter ChildrenListDeleter
	Updater ChildrenListUpdater
}

// ControllerApplyOption is a typed function used to set
// various apply related executioners
type ControllerApplyOption func(*ControllerManager)

// WithDeleter sets the provided ControllerManager with a
// Deleter instance that can be used while apply
func WithDeleter() ControllerApplyOption {
	return func(m *ControllerManager) {
		var errs []error
		var mgrs []*ChildrenExecutor

		// delete will be based on observed
		// delete observed & owned objects that are no more desired
		for verkind, objects := range m.Observed {
			apiVersion, kind := ParseChildMapKey(verkind)
			client, err := m.DynClientSet.Kind(apiVersion, kind)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			cd := &ChildrenExecutor{
				BaseManager: m.BaseManager,
				DynClient:   client,
				Observed:    objects,
				Desired:     m.Desired[verkind],
			}
			mgrs = append(mgrs, cd)
		}
		m.Deleter = mgrs
		m.err = utilerrors.NewAggregate(errs)
	}
}

// WithUpdater sets the provided ControllerManager with a
// Updater instance that can be used while apply
func WithUpdater() ControllerApplyOption {
	return func(m *ControllerManager) {
		var errs []error
		var mgrs []*ChildrenExecutor

		// create or update is based on desired objects
		for verkind, objects := range m.Desired {
			apiVersion, kind := ParseChildMapKey(verkind)
			client, err := m.DynClientSet.Kind(apiVersion, kind)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			cu := &ChildrenExecutor{
				BaseManager: m.BaseManager,
				DynClient:   client,
				Observed:    m.Observed[verkind],
				Desired:     objects,
			}
			mgrs = append(mgrs, cu)
		}
		m.Updater = mgrs
		m.err = utilerrors.NewAggregate(errs)
	}
}

func appendErrIfNotNil(holder []error, given error) []error {
	if given == nil {
		return holder
	}
	return append(holder, given)
}

// WithDefaults sets various default options against this
// ControllerManager instance if applicable
//
// note:
// 	Caller is expected to use either:
//	- WithDefaults or
//	- apply options in Apply
func (m *ControllerManager) WithDefaults() *ControllerManager {
	if m.Deleter == nil {
		WithDeleter()(m)
	}
	if m.Updater == nil {
		WithUpdater()(m)
	}
	return m
}

// Apply executes create, delete or update operations against
// the child resources set against this manager instance
func (m *ControllerManager) Apply(opts ...ControllerApplyOption) error {
	var errs []error

	// capture existing error(s) if any
	errs = appendErrIfNotNil(errs, m.err)

	for _, o := range opts {
		o(m)
	}

	if m.Deleter != nil {
		errs = appendErrIfNotNil(errs, m.Deleter.Delete())
	}

	if m.Updater != nil {
		errs = appendErrIfNotNil(errs, m.Updater.Update())
	}

	return utilerrors.NewAggregate(errs)
}

// ChildrenExecutor holds fields required to execute
// any operation against the child resources
type ChildrenExecutor struct {
	BaseManager
	DynClient *dynamicclientset.ResourceClient
	Observed  map[string]*unstructured.Unstructured
	Desired   map[string]*unstructured.Unstructured
}

// Update will update the child resources specified in
// each updater item
func (e *ChildrenExecutor) Update() error {
	var errs []error
	for name, obj := range e.Desired {
		ns := obj.GetNamespace()
		if ns == "" {
			ns = e.Watched.GetNamespace()
		}
		if oldObj := e.Observed[name]; oldObj != nil {
			// Update
			newObj, err := ApplyUpdate(oldObj, obj)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			// Attempt an update, if the 3-way merge resulted in any changes.
			if reflect.DeepEqual(
				newObj.UnstructuredContent(),
				oldObj.UnstructuredContent(),
			) {
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
					describeObject(e.Watched),
					describeObject(obj),
				)
				continue
			}

			// Check the update strategy for this child kind.
			method := e.UpdateStrategy.GetMethod(e.DynClient.Group, e.DynClient.Kind)
			switch method {
			case v1alpha1.ChildUpdateOnDelete, "":
				// This means we don't try to update anything unless
				// it gets deleted by someone else (we won't delete it ourselves).
				glog.V(5).Infof(
					"%v: not updating %v (OnDelete update strategy)",
					describeObject(e.Watched),
					describeObject(obj),
				)
				continue
			case v1alpha1.ChildUpdateRecreate, v1alpha1.ChildUpdateRollingRecreate:
				// Delete the object (now) and recreate it (on the next sync).
				glog.Infof(
					"%v: deleting %v for update",
					describeObject(e.Watched),
					describeObject(obj),
				)
				uid := oldObj.GetUID()
				// Explicitly request deletion propagation, which is what users expect,
				// since some objects default to orphaning for backwards compatibility.
				propagation := metav1.DeletePropagationBackground
				err := e.DynClient.Namespace(ns).Delete(
					obj.GetName(),
					&metav1.DeleteOptions{
						Preconditions:     &metav1.Preconditions{UID: &uid},
						PropagationPolicy: &propagation,
					},
				)
				if err != nil {
					errs = append(errs, err)
					continue
				}
			case v1alpha1.ChildUpdateInPlace, v1alpha1.ChildUpdateRollingInPlace:
				// Update the object in-place.
				glog.Infof("%v: updating %v", describeObject(e.Watched), describeObject(obj))
				_, err := e.DynClient.Namespace(ns).Update(
					newObj,
					metav1.UpdateOptions{},
				)
				if err != nil {
					errs = append(errs, err)
					continue
				}
			default:
				errs = append(errs,
					fmt.Errorf(
						"invalid update strategy for %v: unknown method %q",
						e.DynClient.Kind,
						method,
					),
				)
				continue
			}
		} else {
			// Create
			glog.Infof("%v: creating %v", describeObject(e.Watched), describeObject(obj))

			// The controller should return a partial object containing only the
			// fields it cares about. We save this partial object so we can do
			// a 3-way merge upon update, in the style of "kubectl apply".
			//
			// Make sure this happens before we add anything else to the object.
			err := dynamicapply.SetLastApplied(obj, obj.UnstructuredContent())
			if err != nil {
				errs = append(errs, err)
				continue
			}

			// We may claim children we create.
			if e.IsWatchOwner {
				controllerRef := MakeControllerRef(e.Watched)
				ownerRefs := obj.GetOwnerReferences()
				ownerRefs = append(ownerRefs, *controllerRef)
				obj.SetOwnerReferences(ownerRefs)
			}

			_, err = e.DynClient.Namespace(ns).Create(obj, metav1.CreateOptions{})
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

// Delete will delete the child resources based on observed
// & owned child resources that are no longer desired
func (e *ChildrenExecutor) Delete() error {
	var errs []error
	for name, obj := range e.Observed {
		if obj.GetDeletionTimestamp() != nil {
			// Skip objects that are already pending deletion.
			continue
		}
		if e.Desired == nil || e.Desired[name] == nil {
			// This observed object wasn't listed as desired.
			// Hence, this is the right candidate to be deleted.
			glog.Infof("%v: deleting %v", describeObject(e.Watched), describeObject(obj))
			uid := obj.GetUID()
			// Explicitly request deletion propagation, which is what
			// users expect, since some objects default to orphaning
			// for backwards compatibility.
			propagation := metav1.DeletePropagationBackground
			err := e.DynClient.Namespace(obj.GetNamespace()).Delete(
				obj.GetName(),
				&metav1.DeleteOptions{
					Preconditions:     &metav1.Preconditions{UID: &uid},
					PropagationPolicy: &propagation,
				},
			)
			if err != nil {
				errs = append(
					errs,
					errors.Wrapf(err, "can't delete %v", describeObject(obj)),
				)
				continue
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

// ChildrenListDeleter manages deletion of a list of
// children executors
type ChildrenListDeleter []*ChildrenExecutor

// Delete will delete all the child resources specified in
// each of the deleter instance
func (list ChildrenListDeleter) Delete() error {
	var errs []error
	for _, deleter := range list {
		errs = appendErrIfNotNil(errs, deleter.Delete())
	}
	return utilerrors.NewAggregate(errs)
}

// ChildrenListUpdater manages update of a list of
// children executors
type ChildrenListUpdater []*ChildrenExecutor

// Update will update all the child resources specified in
// each of the updater instance
func (list ChildrenListUpdater) Update() error {
	var errs []error
	for _, updater := range list {
		errs = appendErrIfNotNil(errs, updater.Update())
	}
	return utilerrors.NewAggregate(errs)
}
