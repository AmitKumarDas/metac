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
	"openebs.io/metac/third_party/kubernetes"
)

// AttachmentExecuteBase holds the common properties required to
// operate against an attachment.
type AttachmentExecuteBase struct {
	// GetChildUpdateStrategyByGK fetches the update strategy to be
	// followed while executing update operation against an
	// attachment
	GetChildUpdateStrategyByGK func(group, kind string) v1alpha1.ChildUpdateMethod

	// Resource that is under watch. A watch might be related
	// to the attachments. For example, a watch object might
	// be owner of the attachments, etc.
	Watch *unstructured.Unstructured

	// IsWatchOwner indicates if the watch resource should be
	// the owner of the attachment resources
	//
	// NOTE:
	//	This declaration is required if attachments are created
	// during the process of reconciliation and if these attachments
	// should be claimed by the watch.
	IsWatchOwner *bool
}

// AttachmentManager manages applying the attachment resources
// in Kubernetes cluster. Here apply implies either Create or
// Update or Delete attachment resources against the Kubernetes
// cluster.
//
// NOTE:
//	Caller code is expected to instantiate up this structure with
// appropriate values. Hence, this structure exposes all of its fields
// as public properties.
type AttachmentManager struct {
	AttachmentExecuteBase

	// DynamicClientSet is responsible to provide a dynamic client
	// for a specific resource
	DynamicClientSet *dynamicclientset.Clientset

	// Observed state (i.e. current state in kubernetes) of
	// attachment resources
	Observed AnyUnstructRegistry

	// Desired kubernetes state of attachment resources
	Desired AnyUnstructRegistry

	// Various executioners required to execute apply
	Deleter AnyAttachmentsDeleter
	Updater AnyAttachmentsUpdater
}

// AttachmentManagerOption is a typed function used to
// build AttachmentManager
//
// This follows "functional options" pattern
type AttachmentManagerOption func(*AttachmentManager) error

// AnyAttachmentsDefaultDeleter sets the provided AttachmentManager with a
// Deleter instance that can be used during apply operation
func AnyAttachmentsDefaultDeleter() AttachmentManagerOption {
	return func(m *AttachmentManager) error {
		var errs []error
		var executors []*AttachmentResourcesExecutor

		// delete will be based on observed
		// delete observed & owned objects that are no more desired
		// iterate to group resources of same kind & apiVersion
		for verkind, objects := range m.Observed {
			apiVersion, kind := ParseKeyToAPIVersionKind(verkind)
			client, err := m.DynamicClientSet.GetClientByKind(apiVersion, kind)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			cd := &AttachmentResourcesExecutor{
				AttachmentExecuteBase: m.AttachmentExecuteBase,
				DynamicResourceClient: client,
				Observed:              objects,
				Desired:               m.Desired[verkind],
			}
			executors = append(executors, cd)
		}

		if len(errs) == 0 {
			m.Deleter = executors
		}
		return utilerrors.NewAggregate(errs)
	}
}

// AnyAttachmentsDefaultUpdater sets the provided AttachmentManager with a
// Updater instance that can be used during apply operation
func AnyAttachmentsDefaultUpdater() AttachmentManagerOption {
	return func(m *AttachmentManager) error {
		var errs []error
		var executors []*AttachmentResourcesExecutor

		// create or update is based on desired objects
		// iterate to group resources of same kind & apiVersion
		for verkind, objects := range m.Desired {
			apiVersion, kind := ParseKeyToAPIVersionKind(verkind)
			// get the specific resource client
			client, err := m.DynamicClientSet.GetClientByKind(apiVersion, kind)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			cu := &AttachmentResourcesExecutor{
				AttachmentExecuteBase: m.AttachmentExecuteBase,
				DynamicResourceClient: client,
				Observed:              m.Observed[verkind],
				Desired:               objects,
			}
			executors = append(executors, cu)
		}

		if len(errs) == 0 {
			m.Updater = executors
		}
		return utilerrors.NewAggregate(errs)
	}
}

func appendErrIfNotNil(holder []error, given error) []error {
	if given == nil {
		return holder
	}
	return append(holder, given)
}

// SetDefaultsIfNil sets various default options against this
// ControllerManager instance if applicable
func (m *AttachmentManager) SetDefaultsIfNil() *AttachmentManager {
	if m.Deleter == nil {
		AnyAttachmentsDefaultDeleter()(m)
	}
	if m.Updater == nil {
		AnyAttachmentsDefaultUpdater()(m)
	}
	if m.IsWatchOwner == nil {
		// defaults to set this watch as the owner
		// of the attachments
		m.IsWatchOwner = kubernetes.BoolPtr(true)
	}
	return m
}

// Apply executes create, delete or update operations against
// the child resources set against this manager instance
func (m *AttachmentManager) Apply() error {
	var errs []error

	if m.Deleter != nil {
		errs = appendErrIfNotNil(errs, m.Deleter.Delete())
	}

	if m.Updater != nil {
		errs = appendErrIfNotNil(errs, m.Updater.Update())
	}

	return utilerrors.NewAggregate(errs)
}

// AttachmentResourcesExecutor holds fields required to execute
// operations against the attachment resources
type AttachmentResourcesExecutor struct {
	AttachmentExecuteBase

	// Dynamic client that can execute client related operations
	// against a particular resource i.e. apiVersion & kind
	DynamicResourceClient *dynamicclientset.ResourceClient

	// Group of observed & desired resources anchored by
	// resource name.
	//
	// It is important to have all resources in these maps
	// belong to a one apiVersion & kind.
	//
	// This apiVersion and kind corresponds to
	// DynamicResourceClient property of this instance.
	Observed map[string]*unstructured.Unstructured
	Desired  map[string]*unstructured.Unstructured
}

func (e *AttachmentResourcesExecutor) String() string {
	if e.Watch == nil {
		return "AttachmentResourcesExecutor"
	}
	return fmt.Sprintf(
		"AttachmentResourcesExecutor %s:",
		describeObject(e.Watch),
	)
}

// Update will update the child resources specified in
// each updater item
func (e *AttachmentResourcesExecutor) Update() error {
	var errs []error

	// map the "desired" with its exact "observed"
	// instance before carrying out the update
	// operation
	for name, dObj := range e.Desired {
		ns := dObj.GetNamespace()
		if ns == "" {
			ns = e.Watch.GetNamespace()
		}

		// update since this object is observed to exist
		if oObj := e.Observed[name]; oObj != nil {
			// 3-way merge
			uObj, err := ApplyMerge(oObj, dObj)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			// Attempt an update, if the 3-way merge
			// resulted in any changes.
			if reflect.DeepEqual(
				uObj.UnstructuredContent(),
				oObj.UnstructuredContent(),
			) {
				// Nothing changed.
				continue
			}
			if glog.V(5) {
				glog.Infof(
					"%s: Diff: a=observed, b=desired:\n%s",
					e,
					diff.ObjectReflectDiff(
						oObj.UnstructuredContent(),
						uObj.UnstructuredContent(),
					),
				)
			}

			// Leave it alone if it's pending deletion.
			//
			// TODO (@amitkumardas):
			// Should this be moved before applying update
			if oObj.GetDeletionTimestamp() != nil {
				glog.Infof(
					"%s: Not updating %s: Pending deletion",
					e, describeObject(dObj),
				)
				continue
			}

			// Check the update strategy for this child kind.
			method := e.GetChildUpdateStrategyByGK(
				e.DynamicResourceClient.Group, e.DynamicResourceClient.Kind,
			)
			switch method {
			case v1alpha1.ChildUpdateOnDelete, "":
				// This means we don't try to update anything unless
				// it gets deleted by someone else
				// (we won't delete it ourselves).
				//
				// TODO (@amitkumardas):
				// Should this be moved before applying update
				glog.V(5).Infof(
					"%s: Not updating %s: OnDelete update strategy",
					e, describeObject(dObj),
				)
				continue
			case v1alpha1.ChildUpdateRecreate, v1alpha1.ChildUpdateRollingRecreate:
				// Delete the object (now) and recreate it (on the next sync).
				glog.Infof("%s: Deleting %s for update", e, describeObject(dObj))

				uid := oObj.GetUID()
				// Explicitly request deletion propagation, which is what users expect,
				// since some objects default to orphaning for backwards compatibility.
				propagation := metav1.DeletePropagationBackground
				err := e.DynamicResourceClient.Namespace(ns).Delete(
					dObj.GetName(),
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
				glog.Infof("%s: Updating %s", e, describeObject(dObj))

				_, err := e.DynamicResourceClient.Namespace(ns).Update(
					uObj, metav1.UpdateOptions{},
				)
				if err != nil {
					errs = append(errs, err)
					continue
				}
			default:
				errs = append(errs,
					fmt.Errorf(
						"%s: Invalid update strategy %s for %s",
						e, method, describeObject(dObj),
					),
				)
				continue
			}
		} else {
			// Create since this object is not observed in cluster
			glog.Infof("%s: Creating %s", e, describeObject(dObj))

			// The controller should return a partial object containing only the
			// fields it cares about. We save this partial object so we can do
			// a 3-way merge upon update, in the style of "kubectl apply".
			//
			// Make sure this happens before we add anything else to the object.
			err := dynamicapply.SetLastApplied(dObj, dObj.UnstructuredContent())
			if err != nil {
				errs = append(errs, err)
				continue
			}

			// We claim attachments we create if watch should be the
			// owner
			if e.IsWatchOwner != nil && *e.IsWatchOwner {
				watchAsOwnerRef := MakeOwnerRef(e.Watch)

				// fetch existing owner references of this attachment
				// and add this watch as an additional owner reference
				ownerRefs := dObj.GetOwnerReferences()
				ownerRefs = append(ownerRefs, *watchAsOwnerRef)
				dObj.SetOwnerReferences(ownerRefs)
			}

			_, err =
				e.DynamicResourceClient.Namespace(ns).Create(dObj, metav1.CreateOptions{})
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
func (e *AttachmentResourcesExecutor) Delete() error {
	var errs []error
	for name, obj := range e.Observed {
		if obj.GetDeletionTimestamp() != nil {
			// Skip objects that are already pending deletion.
			continue
		}
		if e.Desired == nil || e.Desired[name] == nil {
			// This observed object wasn't listed as desired.
			// Hence, this is the right candidate to be deleted.
			glog.Infof("%v: deleting %v", describeObject(e.Watch), describeObject(obj))
			uid := obj.GetUID()
			// Explicitly request deletion propagation, which is what
			// users expect, since some objects default to orphaning
			// for backwards compatibility.
			propagation := metav1.DeletePropagationBackground
			err := e.DynamicResourceClient.Namespace(obj.GetNamespace()).Delete(
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

// AnyAttachmentsDeleter holds a list of delete based attachment
// executors.
type AnyAttachmentsDeleter []*AttachmentResourcesExecutor

// Delete will delete all the attachment resources available in
// this instance
func (list AnyAttachmentsDeleter) Delete() error {
	var errs []error
	for _, deleter := range list {
		errs = appendErrIfNotNil(errs, deleter.Delete())
	}
	return utilerrors.NewAggregate(errs)
}

// AnyAttachmentsUpdater holds a list of update based attachment
// executors.
type AnyAttachmentsUpdater []*AttachmentResourcesExecutor

// Update will update all the attachment resources available in
// this instance
func (list AnyAttachmentsUpdater) Update() error {
	var errs []error
	for _, updater := range list {
		errs = appendErrIfNotNil(errs, updater.Update())
	}
	return utilerrors.NewAggregate(errs)
}
