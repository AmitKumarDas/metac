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
	"strings"

	"github.com/golang/glog"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	dynamicapply "openebs.io/metac/dynamic/apply"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
	"openebs.io/metac/third_party/kubernetes"
)

const (
	attachmentCreateAnnotationKey string = "metac.openebs.io/created-due-to-watch"

	attachmentUpdateAnnotationKeySuffix string = "/updated-due-to-watch"

	lastAppliedAnnotationKeySuffix string = "/gctl-last-applied"
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

	// UpdateAny follows the GenericController's spec.UpdateAny. When set
	// to true it grants this executor to update any attachments even if
	// these attachments were not created due to the watch set in this
	// executor.
	UpdateAny *bool

	// DeleteAny follows the GenericController's spec.DeleteAny. When set
	// to true it grants this executor to delete any attachments even if
	// these attachments were not created due to the watch set in this
	// executor.
	DeleteAny *bool

	// If UpdateDuringPendingDelete is set to true it will proceed with
	// updating the resource even if this resource is pending deletion
	UpdateDuringPendingDelete *bool
}

// String implements Stringer interface
func (m AttachmentExecuteBase) String() string {
	var strs []string
	strs = append(strs, "AttachmentExecutor")
	if m.Watch != nil {
		strs = append(strs, DescObjectAsKey(m.Watch))
	}
	if m.IsWatchOwner != nil {
		strs = append(strs, fmt.Sprintf("IsWatchOwner=%t", *m.IsWatchOwner))
	}
	return strings.Join(strs, " ")
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
	DeleteFn         func() error
	CreateOrUpdateFn func() error

	// error as value
	errs []error
}

// AttachmentManagerOption is a typed function used to
// build AttachmentManager
//
// This follows "functional options" pattern
type AttachmentManagerOption func(*AttachmentManager) error

// anyAttachmentsDefaultDeleter sets the provided AttachmentManager with a
// Deleter instance that can be used during apply operation
func anyAttachmentsDefaultDeleter() AttachmentManagerOption {
	return func(m *AttachmentManager) error {
		var errs []error
		var executors AnyAttachmentsDeleter

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
			m.DeleteFn = executors.Delete
		}
		return utilerrors.NewAggregate(errs)
	}
}

// anyAttachmentsDefaultCreateUpdater sets the provided AttachmentManager with a
// Updater instance that can be used during apply operation
func anyAttachmentsDefaultCreateUpdater() AttachmentManagerOption {
	return func(m *AttachmentManager) error {
		var errs []error
		var executors AnyAttachmentsCreateUpdater

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
			m.CreateOrUpdateFn = executors.CreateOrUpdate
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

// String implements Stringer interface
func (m AttachmentManager) String() string {
	return m.AttachmentExecuteBase.String()
}

// setDefaults prepares the attachment manager before invoking the
// Apply
//
// NOTE:
// 	This method sets various **default** options if applicable
func (m *AttachmentManager) setDefaults() {
	if m.DeleteFn == nil {
		m.errs = appendErrIfNotNil(
			m.errs, anyAttachmentsDefaultDeleter()(m),
		)
	}
	if m.CreateOrUpdateFn == nil {
		m.errs = appendErrIfNotNil(
			m.errs, anyAttachmentsDefaultCreateUpdater()(m),
		)
	}
	if m.IsWatchOwner == nil {
		// defaults to set this watch as the owner of the
		// attachments, since this tunable is used only during
		// creation of these attachment(s) while observing the
		// watch resource
		m.IsWatchOwner = kubernetes.BoolPtr(true)
	}
}

// Apply executes create, delete or update operations against
// the child resources set against this manager instance
func (m *AttachmentManager) Apply() error {
	m.setDefaults()
	if len(m.errs) != 0 {
		return utilerrors.NewAggregate(m.errs)
	}

	m.errs = appendErrIfNotNil(m.errs, m.DeleteFn())
	m.errs = appendErrIfNotNil(m.errs, m.CreateOrUpdateFn())

	return utilerrors.NewAggregate(m.errs)
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

// String implements Stringer interface
func (e AttachmentResourcesExecutor) String() string {
	return e.AttachmentExecuteBase.String()
}

// IsUpdateDuringPendingDelete returns true if update is allowed
// even if the targeted resource is pending deletion
func (e AttachmentResourcesExecutor) IsUpdateDuringPendingDelete() bool {
	if e.UpdateDuringPendingDelete == nil {
		return false
	}
	return *e.UpdateDuringPendingDelete
}

// Update updates the observed attachment to its desired attachment
func (e *AttachmentResourcesExecutor) Update(oObj, dObj *unstructured.Unstructured) error {
	// TODO (@amitkumardas):
	//
	// Should this be desired object's namespace or
	// current object's namespace. Updating the namespace
	// may not work just by taking up the desired object's namespace.
	ns := dObj.GetNamespace()
	if ns == "" {
		ns = e.Watch.GetNamespace()
	}

	// Leave it alone if it's pending deletion && updating during
	// pending deletion is not enabled
	if oObj.GetDeletionTimestamp() != nil && !e.IsUpdateDuringPendingDelete() {
		glog.V(4).Infof(
			"%s: Can't update %s: Pending deletion", e, DescObjectAsKey(dObj),
		)
		return nil
	}

	// if controller has rights to update any attachments
	updateAny := false
	if e.UpdateAny != nil {
		updateAny = *e.UpdateAny
	}

	// check if this object was created due to a
	// GenericController watch
	ann := oObj.GetAnnotations()
	wantWatch := string(e.Watch.GetUID())
	gotWatch := ""
	if ann != nil {
		gotWatch = ann[attachmentCreateAnnotationKey]
	}

	// if watches don't match && this controller is not granted
	// to update any arbitrary attachments then skip this update
	if gotWatch != wantWatch && !updateAny {
		glog.V(4).Infof(
			"%s: Can't update %s: Annotation %s has %q want %q: UpdateAny %t",
			e,
			DescObjectAsKey(dObj),
			attachmentCreateAnnotationKey,
			gotWatch,
			wantWatch,
			updateAny,
		)
		return nil
	}

	// Check the update strategy for this child kind
	method := e.GetChildUpdateStrategyByGK(
		e.DynamicResourceClient.Group, e.DynamicResourceClient.Kind,
	)

	// Skip update if update strategy does not allow
	if method == v1alpha1.ChildUpdateOnDelete || method == "" {
		// This means we don't try to update anything unless
		// it gets deleted by someone else i.e. we won't delete it
		// ourselves
		glog.V(4).Infof(
			"%s: Can't update %s: UpdateStrategy=%q", e, DescObjectAsKey(dObj), method,
		)
		return nil
	}

	// Set who is responsible for this update. In other words set the
	// watch details in the annotations
	if ann == nil {
		ann = make(map[string]string)
	}
	ann[string(e.Watch.GetUID())+attachmentUpdateAnnotationKeySuffix] =
		DescObjectAsSanitisedKey(e.Watch)
	dObj.SetAnnotations(ann)

	// 3-way merge
	// Construct the annotation key that holds the last applied
	// state. The annotation key is based on the current watch.
	//
	// NOTE:
	// 	This lets an attachment to be updated independently even
	// with multiple updates triggered via different watches.
	lastAppliedKey := string(e.Watch.GetUID()) + lastAppliedAnnotationKeySuffix
	a := NewApplyFromAnnKey(lastAppliedKey)
	uObj, err := a.Merge(oObj, dObj)
	if err != nil {
		return err
	}

	// Attempt an update, if the 3-way merge
	// resulted in any changes.
	if reflect.DeepEqual(
		uObj.UnstructuredContent(),
		oObj.UnstructuredContent(),
	) {
		glog.V(4).Infof(
			"%s: Can't update %s: Nothing changed.", e, DescObjectAsKey(dObj),
		)
		return nil
	}

	if glog.V(5) {
		glog.Infof(
			"%s: Diff: a=observed, b=desired:\n%s",
			e,
			cmp.Diff(
				oObj.UnstructuredContent(),
				uObj.UnstructuredContent(),
			),
		)
	}

	// Act based on the update strategy for this child kind.
	switch method {
	case v1alpha1.ChildUpdateRecreate, v1alpha1.ChildUpdateRollingRecreate:
		// Delete the object (now) and recreate it (on the next sync).
		glog.V(4).Infof("%s: Deleting %s for update", e, DescObjectAsKey(dObj))

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
			return err
		}

		glog.Infof("%s: Deleted %s for update", e, DescObjectAsKey(dObj))
	case v1alpha1.ChildUpdateInPlace, v1alpha1.ChildUpdateRollingInPlace:
		// Update the object in-place.
		glog.V(4).Infof("%s: Updating %s", e, DescObjectAsKey(dObj))

		_, err := e.DynamicResourceClient.Namespace(ns).Update(
			uObj, metav1.UpdateOptions{},
		)
		if err != nil {
			return err
		}

		glog.V(2).Infof("%s: Updated %s", e, DescObjectAsKey(dObj))
	default:
		return errors.Errorf(
			"%s: Invalid update strategy %s for %s",
			e, method, DescObjectAsKey(dObj),
		)
	}

	return nil
}

// Create creates the desired attachment
func (e *AttachmentResourcesExecutor) Create(dObj *unstructured.Unstructured) error {
	ns := dObj.GetNamespace()
	if ns == "" {
		// if desired object has not been given a namespace
		// then it is set to watch's namespace
		ns = e.Watch.GetNamespace()
	}

	glog.V(4).Infof("%s: Creating %s", e, DescObjectAsKey(dObj))

	// The controller i.e. sync hook should return a partial attachment
	// containing only the fields it cares about. We save this partial
	// attachment so we can do a 3-way merge upon update, in the style
	// of "kubectl apply".
	//
	// Make sure this happens before we add anything else to the object.
	err := dynamicapply.SetLastAppliedByAnnKey(
		dObj,
		dObj.UnstructuredContent(),
		string(e.Watch.GetUID())+lastAppliedAnnotationKeySuffix,
	)
	if err != nil {
		return err
	}

	// add create specific annotation
	// this annotation will be set irrespective of whether a watch
	// is a owner of an attachment or not
	ann := dObj.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	ann[attachmentCreateAnnotationKey] = string(e.Watch.GetUID())
	dObj.SetAnnotations(ann)

	// Attachments are set with watch as the owner reference
	// if watch is flagged to be the owner
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
		return err
	}

	glog.Infof("%s: Created %s", e, DescObjectAsKey(dObj))
	return nil
}

// CreateOrUpdate will create or update the attachments
func (e *AttachmentResourcesExecutor) CreateOrUpdate() error {
	var errs []error

	// map the "desired" with its exact "observed"
	// instance before carrying out create / update
	// operation
	for name, dObj := range e.Desired {
		if oObj := e.Observed[name]; oObj != nil {
			// -------------------------------------------
			// try update since object already exists
			// -------------------------------------------
			err := e.Update(oObj, dObj)
			if err != nil {
				errs = appendErrIfNotNil(errs, err)
			}
		} else {
			// ----------------------------------------------------
			// try create since this object is not observed in cluster
			// ----------------------------------------------------
			err := e.Create(dObj)
			if err != nil {
				errs = appendErrIfNotNil(errs, err)
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

// Delete will delete the child resources based on observed
// & owned child resources that are no longer desired
func (e *AttachmentResourcesExecutor) Delete() error {
	var errs []error

	// check if controller has rights to delete any attachments
	deleteAny := false
	if e.DeleteAny != nil {
		deleteAny = *e.DeleteAny
	}

	for name, obj := range e.Observed {
		if obj.GetDeletionTimestamp() != nil {
			// Skip objects that are already pending deletion.
			glog.V(4).Infof("%s: Can't delete %s: Pending deletion",
				e, DescObjectAsKey(obj),
			)
			continue
		}

		// check which watch created this resource in the first place
		ann := obj.GetAnnotations()
		wantWatch := string(e.Watch.GetUID())
		gotWatch := ""
		if ann != nil {
			gotWatch = ann[attachmentCreateAnnotationKey]
		}

		if gotWatch != wantWatch && !deleteAny {
			// Skip objects that was not created due to this watch
			glog.V(4).Infof(
				"%s: Can't delete %s: Annotation %s has %q want %q: DeleteAny %t",
				e,
				DescObjectAsKey(obj),
				attachmentCreateAnnotationKey,
				gotWatch,
				wantWatch,
				deleteAny,
			)
			continue
		}

		if e.Desired == nil || e.Desired[name] == nil {
			// This observed object wasn't listed as desired.
			// Hence, this is the right candidate to be deleted.
			glog.V(4).Infof("%s: Deleting %s", e, DescObjectAsKey(obj))

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
				if apierrors.IsNotFound(err) {
					glog.V(4).Infof(
						"%s: Can't delete %s: Is not found: %v",
						e, DescObjectAsKey(obj), err)
					continue
				}
				errs = append(
					errs,
					errors.Wrapf(err, "%s: Failed to delete %s", e, DescObjectAsKey(obj)),
				)
				continue
			}

			glog.Infof("%s: Deleted %s", e, DescObjectAsKey(obj))
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
	for _, exec := range list {
		errs = appendErrIfNotNil(errs, exec.Delete())
	}
	return utilerrors.NewAggregate(errs)
}

// AnyAttachmentsCreateUpdater holds a list of executors
// that either create or update the attachments
type AnyAttachmentsCreateUpdater []*AttachmentResourcesExecutor

// CreateOrUpdate will create or update the attachment resources
func (list AnyAttachmentsCreateUpdater) CreateOrUpdate() error {
	var errs []error

	for _, exec := range list {
		errs = appendErrIfNotNil(errs, exec.CreateOrUpdate())
	}
	return utilerrors.NewAggregate(errs)
}
