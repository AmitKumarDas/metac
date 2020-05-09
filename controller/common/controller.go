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
	"strings"

	"github.com/golang/glog"
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
	// AttachmentCreateAnnotationKey is the annotation key used to
	// hold the watch uid that is responsible for creating the
	// attachment
	AttachmentCreateAnnotationKey string = "metac.openebs.io/created-due-to-watch"

	// AttachmentUpdateAnnotationKeySuffix is the annotation suffix
	// used to hold the watch uid that is responsible for updating the
	// attachment
	AttachmentUpdateAnnotationKeySuffix string = "/updated-due-to-watch"

	// GCTLLastAppliedAnnotationKeySuffix is the annotation suffix
	// used to hold the last applied state of the attachment
	GCTLLastAppliedAnnotationKeySuffix string = "/gctl-last-applied"
)

// ClusterStatesControllerBase holds common properties required to manage
// cluster resource states
type ClusterStatesControllerBase struct {
	// GetChildUpdateStrategyByGK fetches the update strategy to be
	// followed while executing update operation against a
	// resource _also referred to as a child or attachment when used
	// in context of watch_.
	GetChildUpdateStrategyByGK func(group, kind string) v1alpha1.ChildUpdateMethod

	// IsPatchByGK returns true if resource need to be patched
	// versus the default 3-way merge during the update operations
	IsPatchByGK func(group, kind string) bool

	// Another resource that is being watched to arrive at some
	// desired state. A watch might be related to this resource
	// under operation. For example, a watch might be owner of
	// this resource.
	Watch *unstructured.Unstructured

	// IsWatchOwner indicates if the watch should be the owner
	// of the resource under operation
	//
	// NOTE:
	//	This declaration is required if resources are created
	// during the process of reconciliation and if these should
	// should be owned by the watch.
	IsWatchOwner *bool

	// UpdateAny follows GenericController's spec.UpdateAny
	// policy. When set to true it grants this operator the
	// permission to update any resources even if these were
	// not created due to the watch set here.
	UpdateAny *bool

	// DeleteAny follows GenericController's spec.DeleteAny
	// policy. When set to true it grants this operator to
	// delete any resources even if these were not created
	// due to the watch set here.
	DeleteAny *bool

	// If UpdateDuringPendingDelete is set to true it will
	// update the resource even if this resource is pending
	// deletion
	UpdateDuringPendingDelete *bool
}

// ClusterStatesController **applies** resources in Kubernetes cluster.
// Apply implies either Create, Update or Delete of resources against
// the Kubernetes cluster.
type ClusterStatesController struct {
	ClusterStatesControllerBase

	// clientset to fetch dynamic client(s) corresponding to resource(s)
	DynamicClientSet *dynamicclientset.Clientset

	// observed states _(i.e. resource states found in kubernetes cluster)_
	Observed AnyUnstructRegistry

	// desired states _(i.e. to apply against the kubernetes cluster)_
	Desired AnyUnstructRegistry

	// resources that need to be deleted explicitly in the
	// kubernetes cluster
	//
	// NOTE:
	//	explicit deletes implies these resources can be deleted even
	// if these were not created by this controller
	ExplicitDeletes AnyUnstructRegistry

	// resources that need to be updated explicitly in the
	// kubernetes cluster
	//
	// NOTE:
	//	explicit updates implies these resources can be updated even
	// if these were not created by this controller
	ExplicitUpdates AnyUnstructRegistry

	// Various executioners required to arrive at desired states i.e. apply
	DeleteFn         func() error
	CreateOrUpdateFn func() error
	ExplicitDeleteFn func() error
	ExplicitUpdateFn func() error

	// error as value
	errs []error
}

// String implements Stringer interface
func (m ClusterStatesController) String() string {
	var strs []string
	strs = append(strs, "ClusterStatesController")
	if m.Watch != nil {
		strs = append(
			strs,
			fmt.Sprintf("Watch %s", DescObjectAsKey(m.Watch)),
		)
	}
	if m.IsWatchOwner != nil {
		strs = append(
			strs,
			fmt.Sprintf("IsWatchOwner=%t", *m.IsWatchOwner),
		)
	}
	return strings.Join(strs, ": ")
}

// initDeleter sets this controller with deleter logic that
// handles deletion of resources across different api version
// & kind combinations
func (m *ClusterStatesController) initDeleter() {
	var errs []error
	var clusterStatesDeleter ClusterStatesDeleter
	// delete is set only if there was corresponding
	// **observed states**
	//
	// iterate to group resources by kind & apiVersion
	for verkind, objects := range m.Observed {
		apiVersion, kind := ParseKeyToAPIVersionKind(verkind)
		// get dynamic client corresponding to kind & apiversion
		client, err := m.DynamicClientSet.GetClientForAPIVersionAndKind(
			apiVersion,
			kind,
		)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// create an instance of delete controller capable
		// of deleting resources of current api version & kind
		resourceDeleteCtrl := &ResourceStatesController{
			ClusterStatesControllerBase: m.ClusterStatesControllerBase,
			DynamicClient:               client,
			Observed:                    objects,
			Desired:                     m.Desired[verkind],
		}
		clusterStatesDeleter = append(
			clusterStatesDeleter,
			resourceDeleteCtrl,
		)
	}
	// set deleter instance only if there were no errors
	if len(errs) == 0 {
		m.DeleteFn = clusterStatesDeleter.Delete
	} else {
		m.errs = append(m.errs, errs...)
	}
}

// initExplicitDeleter sets this Controller with explicit deleter
// logic that handles deletion of resources across different api
// version & kind combinations
func (m *ClusterStatesController) initExplicitDeleter() {
	var errs []error
	var clusterStatesExplicitDeleter ClusterStatesExplicitDeleter
	// explicit delete is set only if it has corresponding
	// **observed state**
	//
	// loop for every kind & apiVersion
	for verkind, objects := range m.Observed {
		apiVersion, kind := ParseKeyToAPIVersionKind(verkind)
		// get dynamic client corresponding to current kind & apiversion
		client, err := m.DynamicClientSet.GetClientForAPIVersionAndKind(
			apiVersion,
			kind,
		)
		if err != nil {
			errs = append(
				errs,
				errors.Wrapf(
					err,
					"Can't init explicit deleter: %s: %s",
					verkind,
					m,
				),
			)
			continue
		}
		// create an instance of explicit delete controller
		// capable of deleting resources of current api version & kind
		explicitDeleteCtrl := &ResourceStatesController{
			ClusterStatesControllerBase: m.ClusterStatesControllerBase,
			DynamicClient:               client,
			Observed:                    objects,
			ExplicitDeletes:             m.ExplicitDeletes[verkind],
		}
		// add resource states deleter instance to cluster states deleter
		clusterStatesExplicitDeleter = append(
			clusterStatesExplicitDeleter,
			explicitDeleteCtrl,
		)
	}
	// set deleter instance only if there were no errors
	if len(errs) == 0 {
		m.ExplicitDeleteFn = clusterStatesExplicitDeleter.Delete
	} else {
		m.errs = append(m.errs, errs...)
	}
}

// initCreateUpdater sets this Controller instance with a
// create or updater logic that handles create or update
// of resources across different api version & kind
// combinations
func (m *ClusterStatesController) initCreateUpdater() {
	var errs []error
	var clusterStatesCreateUpdater ClusterStatesCreateUpdater
	// create or update is set based on **desired states**
	//
	// iterate to group resources by kind & apiVersion
	for verkind, objects := range m.Desired {
		apiVersion, kind := ParseKeyToAPIVersionKind(verkind)
		// get dynamic client corresponding to resource kind & apiversion
		client, err := m.DynamicClientSet.GetClientForAPIVersionAndKind(
			apiVersion,
			kind,
		)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// create an instance of create-update controller
		// capable of creating or deleting resources of
		// current api version & kind
		createUpdateCtrl := &ResourceStatesController{
			ClusterStatesControllerBase: m.ClusterStatesControllerBase,
			DynamicClient:               client,
			Observed:                    m.Observed[verkind],
			Desired:                     objects,
		}
		clusterStatesCreateUpdater = append(
			clusterStatesCreateUpdater,
			createUpdateCtrl,
		)
	}
	// set create / updater instance only if there are no errors
	if len(errs) == 0 {
		m.CreateOrUpdateFn = clusterStatesCreateUpdater.CreateOrUpdate
	} else {
		m.errs = append(m.errs, errs...)
	}
}

// initExplicitUpdater sets this Controller instance with
// explicit updater logic that handles updates of resources
// across different api version & kind combinations
func (m *ClusterStatesController) initExplicitUpdater() {
	var errs []error
	var clusterStatesExplicitUpdater ClusterStatesExplicitUpdater

	// loop over objects by their kind & apiVersion
	for verkind, updates := range m.ExplicitUpdates {
		observed := m.Observed[verkind]
		if len(observed) == 0 {
			// resources are not updated explicitly if
			// they were never observed in the cluster
			glog.V(6).Infof(
				"Will skip init of explicit update: No resources observed for %s: %s",
				verkind,
				m,
			)
			continue
		}
		apiVersion, kind := ParseKeyToAPIVersionKind(verkind)
		// get dynamic client corresponding to resource kind & apiversion
		client, err := m.DynamicClientSet.GetClientForAPIVersionAndKind(
			apiVersion,
			kind,
		)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// explicit update controller capable of updating resources
		// belonging to current api version & kind
		explicitUpdateCtrl := &ResourceStatesController{
			ClusterStatesControllerBase: m.ClusterStatesControllerBase,
			DynamicClient:               client,
			Observed:                    observed,
			ExplicitUpdates:             updates,
		}
		// set UpdateAny to true to enable explicit update
		//
		// NOTE:
		//	This is very important to **transform** Update into an
		// ExplicitUpdate
		explicitUpdateCtrl.UpdateAny = kubernetes.BoolPtr(true)
		// add resource states explicit updater to
		// cluster states explicit updater
		clusterStatesExplicitUpdater = append(
			clusterStatesExplicitUpdater,
			explicitUpdateCtrl,
		)
	}
	// set explicit updater instance only if there are no errors
	if len(errs) == 0 {
		m.ExplicitUpdateFn = clusterStatesExplicitUpdater.Update
	} else {
		m.errs = append(m.errs, errs...)
	}
}

// initialise this Controller
func (m *ClusterStatesController) initIfNil() {
	if m.DeleteFn == nil {
		m.initDeleter()
	}
	if m.CreateOrUpdateFn == nil {
		m.initCreateUpdater()
	}
	if m.ExplicitDeleteFn == nil {
		m.initExplicitDeleter()
	}
	if m.ExplicitUpdateFn == nil {
		m.initExplicitUpdater()
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
func (m *ClusterStatesController) Apply() error {
	m.initIfNil()
	if len(m.errs) != 0 {
		return utilerrors.NewAggregate(m.errs)
	}
	// execute deletes
	m.errs = append(m.errs, m.DeleteFn())
	// execute creates or updates
	m.errs = append(m.errs, m.CreateOrUpdateFn())
	// execute explicit updates
	m.errs = append(m.errs, m.ExplicitUpdateFn())
	// execute explicit deletes
	m.errs = append(m.errs, m.ExplicitDeleteFn())
	return utilerrors.NewAggregate(m.errs)
}

// ResourceStatesController manages create, update or deletion of
// resources belonging to a **single** apiversion & kind
type ResourceStatesController struct {
	ClusterStatesControllerBase

	// dynamic client to invoke kubernetes CRUD operations
	// corresponding to a resource i.e. apiVersion & kind
	DynamicClient *dynamicclientset.ResourceClient

	// observed & desired resources anchored by name
	//
	// NOTE:
	// 	These resources must have **same** api version & kind
	// and should be possible to operate by above DynamicClient
	Observed map[string]*unstructured.Unstructured
	Desired  map[string]*unstructured.Unstructured

	// explicit resources anchored by name
	//
	// NOTE:
	//	These resources must have **same** api version & kind
	// and should be possible to operate by above DynamicClient
	ExplicitDeletes map[string]*unstructured.Unstructured
	ExplicitUpdates map[string]*unstructured.Unstructured
}

// String implements Stringer interface
func (e ResourceStatesController) String() string {
	var strs []string
	strs = append(strs, "ResourceStatesController")
	if e.Watch != nil {
		strs = append(
			strs,
			fmt.Sprintf("Watch %s", DescObjectAsKey(e.Watch)),
		)
	}
	if e.IsWatchOwner != nil {
		strs = append(
			strs,
			fmt.Sprintf("IsWatchOwner=%t", *e.IsWatchOwner),
		)
	}
	return strings.Join(strs, " ")
}

// IsUpdateDuringPendingDelete returns true if update is allowed
// even if the targeted resource is pending deletion
func (e ResourceStatesController) IsUpdateDuringPendingDelete() bool {
	if e.UpdateDuringPendingDelete == nil {
		return false
	}
	return *e.UpdateDuringPendingDelete
}

// Update updates the observed state to its desired state
//
// NOTE:
//	A return value of true indicates a successful update
func (e *ResourceStatesController) update(
	observed *unstructured.Unstructured,
	desired *unstructured.Unstructured,
) (bool, error) {
	// use either desired namespace or watch namespace to update
	ns := desired.GetNamespace()
	if ns == "" {
		ns = e.Watch.GetNamespace()
	}

	// Leave it alone if it's pending deletion && updating during
	// pending deletion is not enabled
	if observed.GetDeletionTimestamp() != nil && !e.IsUpdateDuringPendingDelete() {
		glog.V(5).Infof(
			"Can't update %s: Pending deletion: %s",
			DescObjectAsKey(desired),
			e,
		)
		return false, nil
	}

	// if controller has rights to update any attachments
	updateAny := false
	if e.UpdateAny != nil {
		updateAny = *e.UpdateAny
	}

	// Check if this object was created due to the controller watch
	observedAnns := observed.GetAnnotations()
	currentWatchUID := string(e.Watch.GetUID())
	createdByWatchUID := ""
	if observedAnns != nil {
		createdByWatchUID = observedAnns[AttachmentCreateAnnotationKey]
	}

	// If watches don't match && this controller is not granted
	// to update any arbitrary attachments then skip this update
	if createdByWatchUID != currentWatchUID && !updateAny {
		glog.V(6).Infof(
			"Won't update %s: Annotation %s = %q want %q: UpdateAny = %t: %s",
			DescObjectAsKey(desired),
			AttachmentCreateAnnotationKey,
			createdByWatchUID,
			currentWatchUID,
			updateAny,
			e,
		)
		return false, nil
	}

	// Check the update strategy for this child kind
	method := e.GetChildUpdateStrategyByGK(
		e.DynamicClient.Group,
		e.DynamicClient.Kind,
	)

	// Skip update if update strategy does not allow
	if method == v1alpha1.ChildUpdateOnDelete || method == "" {
		// This means we don't try to update anything unless
		// it gets deleted by someone else i.e. we won't delete it
		// ourselves
		glog.V(6).Infof(
			"Won't update %s: UpdateStrategy=%q: %s",
			DescObjectAsKey(desired),
			method,
			e,
		)
		return false, nil
	}

	// 3-way merge
	//
	// Construct the annotation key that holds the last applied
	// state. The annotation key is based on the current watch.
	//
	// NOTE:
	// 	This lets an attachment to be updated independently even
	// with multiple updates (read 3-way merges) triggered due to
	// different watches.
	lastAppliedKey :=
		string(e.Watch.GetUID()) + GCTLLastAppliedAnnotationKeySuffix

	// Check if its a patch based update vs. 3-way merge based update
	if e.IsPatchByGK(e.DynamicClient.Group, e.DynamicClient.Kind) {
		// Since patch is enabled; resource based on this api group
		// & kind will be patched versus the standard 3-way merge based
		// update.
		//
		// NOTE:
		// 	We set last applied state as observed instance's content.
		// This last applied state is then set against the same observed
		// instance's annotation.
		//
		// This lets metac to have full control of all the fields during
		// the 3-way merge operation. This way of executing 3-way merge
		// to arbitrary resources is equivalent to apply operation of
		// resources created by metac controllers.
		//
		// NOTE:
		// 	However, the final merged instance is saved in the cluster
		// with desired instance's content as the last applied state.
		err := dynamicapply.SetLastAppliedByAnnKey(
			observed,
			observed.UnstructuredContent(),
			lastAppliedKey,
		)
		if err != nil {
			return false, err
		}
	}

	// create a new instance of Apply
	a := NewApplyFromAnnKey(lastAppliedKey)
	// invoke 3-way merge
	mergedObj, err := a.Merge(observed, desired)
	if err != nil {
		return false, err
	}

	// Proceed for update, only if above merge resulted in any
	// differences between observed state vs. desired state
	isDiff, err := a.HasMergeDiff()
	if err != nil {
		return false, err
	}
	if !isDiff {
		glog.V(7).Infof(
			"Won't update %s: Nothing changed: %s",
			DescObjectAsKey(desired),
			e,
		)
		return false, nil
	}
	glog.V(6).Infof(
		"Will update %s since observed != desired: %s",
		DescObjectAsKey(desired),
		e,
	)

	// Act based on the update strategy for this child kind.
	switch method {
	case v1alpha1.ChildUpdateRecreate, v1alpha1.ChildUpdateRollingRecreate:
		// Delete the object (now) and recreate it (on the next sync).
		glog.V(4).Infof(
			"Deleting %s for update: %s",
			DescObjectAsKey(desired),
			e,
		)

		uid := observed.GetUID()
		// Explicitly request deletion propagation, which is what users expect,
		// since some objects default to orphaning for backwards compatibility.
		propagation := metav1.DeletePropagationBackground
		err := e.DynamicClient.Namespace(ns).Delete(
			desired.GetName(),
			&metav1.DeleteOptions{
				Preconditions:     &metav1.Preconditions{UID: &uid},
				PropagationPolicy: &propagation,
			},
		)
		if err != nil {
			return false, err
		}

		glog.Infof(
			"Deleted %s for update: %s",
			DescObjectAsKey(desired),
			e,
		)
	case v1alpha1.ChildUpdateInPlace, v1alpha1.ChildUpdateRollingInPlace:
		// Update the object in-place.
		glog.V(6).Infof(
			"Updating %s: %s",
			DescObjectAsKey(desired),
			e,
		)

		// Set who is responsible for this update.
		// In other words set the watch details in the annotations
		updatedAnns := mergedObj.GetAnnotations()
		if updatedAnns == nil {
			updatedAnns = make(map[string]string)
		}
		updatedAnns[string(e.Watch.GetUID())+AttachmentUpdateAnnotationKeySuffix] =
			DescObjectAsSanitisedKey(e.Watch)
		mergedObj.SetAnnotations(updatedAnns)

		// update the merged state at the cluster
		_, err := e.DynamicClient.Namespace(ns).Update(
			mergedObj,
			metav1.UpdateOptions{},
		)
		if err != nil {
			return false, err
		}

		glog.V(6).Infof(
			"Updated %s: %s",
			DescObjectAsKey(desired),
			e,
		)
	default:
		return false, errors.Errorf(
			"Invalid update strategy %s: %s: %s",
			method,
			DescObjectAsKey(desired),
			e,
		)
	}
	// this resulted in an actual update
	return true, nil
}

// Create creates the desired resource in the kubernetes cluster
func (e *ResourceStatesController) create(desired *unstructured.Unstructured) error {
	ns := desired.GetNamespace()
	if ns == "" {
		// if desired object has not been given a namespace
		// then it is set to watch's namespace
		ns = e.Watch.GetNamespace()
	}

	glog.V(4).Infof(
		"Creating %s: %s",
		DescObjectAsKey(desired),
		e,
	)

	// The controller i.e. sync hook should return a partial attachment
	// containing only the fields it cares about. We save this partial
	// attachment so we can do a 3-way merge upon update, in the style
	// of "kubectl apply".
	//
	// Make sure this happens before we add anything else to the object.
	err := dynamicapply.SetLastAppliedByAnnKey(
		desired,
		desired.UnstructuredContent(),
		string(e.Watch.GetUID())+GCTLLastAppliedAnnotationKeySuffix,
	)
	if err != nil {
		return err
	}

	// add create specific annotation
	// this annotation will be set irrespective of whether a watch
	// is a owner of an attachment or not
	ann := desired.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	ann[AttachmentCreateAnnotationKey] = string(e.Watch.GetUID())
	desired.SetAnnotations(ann)

	// Attachments are set with current watch as
	// the owner reference if watch is flagged to be the owner
	if e.IsWatchOwner != nil && *e.IsWatchOwner {
		watchAsOwnerRef := MakeOwnerRef(e.Watch)

		// fetch existing owner references of this attachment
		// and add this watch as an additional owner reference
		ownerRefs := desired.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *watchAsOwnerRef)
		desired.SetOwnerReferences(ownerRefs)
	}

	_, err = e.DynamicClient.
		Namespace(ns).
		Create(
			desired,
			metav1.CreateOptions{},
		)
	if err != nil {
		return err
	}

	glog.Infof(
		"Created %s: %s",
		DescObjectAsKey(desired),
		e,
	)
	return nil
}

// CreateOrUpdate will create or update the resources
func (e *ResourceStatesController) CreateOrUpdate() error {
	var errs []error
	// map **desired** with its exact **observed**
	// state to execute either an update or create operation
	for name, dObj := range e.Desired {
		if oObj := e.Observed[name]; oObj != nil {
			// -------------------------------------------
			// try update since object already exists
			// -------------------------------------------
			_, err := e.update(oObj, dObj)
			if err != nil {
				errs = append(errs, err)
			}
		} else {
			// ----------------------------------------------------
			// try create since object is not observed in cluster
			// ----------------------------------------------------
			err := e.create(dObj)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

// ExplicitUpdate will update the resources to their desired states.
// This differs from **Update** call by ignoring the validation of
// updating resources that were exclusively created by this controller.
func (e *ResourceStatesController) ExplicitUpdate() error {
	var errs []error
	if len(e.Observed) == 0 {
		glog.V(6).Infof(
			"Will skip explicit update: No observed resources: %s",
			e,
		)
		return nil
	}
	for name, dObj := range e.ExplicitUpdates {
		oObj := e.Observed[name]
		if oObj == nil {
			// explicit update is ignored if this resource
			// was never observed in kubernetes
			continue
		}
		// -------------------------------------------
		// try explicit update since object already exists
		// -------------------------------------------
		_, err := e.update(oObj, dObj)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

// Delete will delete the resources that are no longer desired
func (e *ResourceStatesController) Delete() error {
	var errs []error

	// check if controller has rights to delete any attachments
	deleteAny := false
	if e.DeleteAny != nil {
		deleteAny = *e.DeleteAny
	}

	for name, obj := range e.Observed {
		if obj.GetDeletionTimestamp() != nil {
			// Skip objects that are already pending deletion.
			glog.V(6).Infof(
				"Can't delete %s: Object pending deletion: %s",
				DescObjectAsKey(obj),
				e,
			)
			// try delete of other resources that are no longer desired
			continue
		}

		// check if the observed state is no longer desired
		if e.Desired == nil || e.Desired[name] == nil {
			// check which watch created this resource in the first place
			ann := obj.GetAnnotations()
			wantWatch := string(e.Watch.GetUID())
			gotWatch := ""
			if ann != nil {
				gotWatch = ann[AttachmentCreateAnnotationKey]
			}

			if gotWatch != wantWatch && !deleteAny {
				// Skip objects that was not created due to this watch
				glog.V(6).Infof(
					"Won't delete %s: Annotation %s = %q want %q: DeleteAny = %t: %s",
					DescObjectAsKey(obj),
					AttachmentCreateAnnotationKey,
					gotWatch,
					wantWatch,
					deleteAny,
					e,
				)
				// try delete of other resources that are no longer desired
				continue
			}

			// This observed object wasn't listed as desired.
			// Hence, this is the right candidate to be deleted.
			glog.V(4).Infof(
				"Deleting %s: %s",
				DescObjectAsKey(obj),
				e,
			)
			uid := obj.GetUID()
			// Explicitly request deletion propagation, which is what
			// users expect, since some objects default to orphaning
			// for backwards compatibility.
			propagation := metav1.DeletePropagationBackground
			err := e.DynamicClient.Namespace(obj.GetNamespace()).Delete(
				obj.GetName(),
				&metav1.DeleteOptions{
					Preconditions:     &metav1.Preconditions{UID: &uid},
					PropagationPolicy: &propagation,
				},
			)
			if err != nil {
				if apierrors.IsNotFound(err) {
					glog.V(4).Infof(
						"Can't delete %s: IsNotFound: %s: %v",
						DescObjectAsKey(obj),
						e,
						err,
					)
					// try delete of other resources that are no longer desired
					continue
				}
				errs = append(
					errs,
					errors.Wrapf(
						err,
						"Failed to delete %s: %s",
						DescObjectAsKey(obj),
						e,
					),
				)
				// try delete of other resources that are no longer desired
				continue
			}
			glog.Infof(
				"Deleted %s: %s",
				DescObjectAsKey(obj),
				e,
			)
		}
	}
	return utilerrors.NewAggregate(errs)
}

// ExplicitDelete will delete the resources that are no
// longer desired. This differs from **Delete** call by ignoring the
// validation of deleting resources that were exclusively created
// by this controller.
func (e *ResourceStatesController) ExplicitDelete() error {
	if len(e.ExplicitDeletes) == 0 {
		// nothing to delete explicitly
		return nil
	}
	var errs []error
	for name, obj := range e.Observed {
		if e.ExplicitDeletes[name] == nil {
			// this resource is not meant to be deleted explicitly
			continue
		}
		if obj.GetDeletionTimestamp() != nil {
			// skip objects that are already pending deletion.
			glog.V(6).Infof(
				"Can't delete explicitly %s: Object pending deletion: %s",
				DescObjectAsKey(obj),
				e,
			)
			// try explicit delete of other listed resources
			continue
		}
		// observed object is listed for explicit delete.
		glog.V(4).Infof(
			"Will explicitly delete %s: %s",
			DescObjectAsKey(obj),
			e,
		)
		uid := obj.GetUID()
		// Explicitly request deletion propagation, which is what
		// users expect, since some objects default to orphaning
		// for backwards compatibility.
		propagation := metav1.DeletePropagationBackground
		err := e.DynamicClient.Namespace(obj.GetNamespace()).Delete(
			obj.GetName(),
			&metav1.DeleteOptions{
				Preconditions:     &metav1.Preconditions{UID: &uid},
				PropagationPolicy: &propagation,
			},
		)
		if err != nil {
			if apierrors.IsNotFound(err) {
				glog.V(4).Infof(
					"Can't explicitly delete %s: IsNotFound: %s: %v",
					DescObjectAsKey(obj),
					e,
					err,
				)
				// try explicit delete of other listed resources
				continue
			}
			errs = append(
				errs,
				errors.Wrapf(
					err,
					"Failed to delete explicitly %s: %s",
					DescObjectAsKey(obj),
					e,
				),
			)
			// try explicit delete of other listed resources
			continue
		}
		glog.Infof(
			"Explicitly deleted %s: %s",
			DescObjectAsKey(obj),
			e,
		)

	}
	return utilerrors.NewAggregate(errs)
}

// ClusterStatesExplicitDeleter deletes cluster resources that
// spans **one or more** api versions & kinds
type ClusterStatesExplicitDeleter []*ResourceStatesController

// Delete will delete the configured cluster resources that were
// not created by this controller
func (list ClusterStatesExplicitDeleter) Delete() error {
	var errs []error
	for _, deleter := range list {
		err := deleter.ExplicitDelete()
		errs = append(errs, err)
	}
	return utilerrors.NewAggregate(errs)
}

// ClusterStatesDeleter deletes cluster resources that
// spans one or more api versions & kinds
type ClusterStatesDeleter []*ResourceStatesController

// Delete will delete the configured cluster resources
func (list ClusterStatesDeleter) Delete() error {
	var errs []error
	for _, deleter := range list {
		err := deleter.Delete()
		errs = append(errs, err)
	}
	return utilerrors.NewAggregate(errs)
}

// ClusterStatesCreateUpdater creates or updates cluster
// resources that spans one or more api versions & kinds
type ClusterStatesCreateUpdater []*ResourceStatesController

// CreateOrUpdate will create or update the configured
// cluster resources
func (list ClusterStatesCreateUpdater) CreateOrUpdate() error {
	var errs []error
	for _, createUpdater := range list {
		err := createUpdater.CreateOrUpdate()
		errs = append(errs, err)
	}
	return utilerrors.NewAggregate(errs)
}

// ClusterStatesExplicitUpdater updates cluster resources that
// were not created by this controller
type ClusterStatesExplicitUpdater []*ResourceStatesController

// Update will update the configured cluster resources that were
// not created by this controller
func (list ClusterStatesExplicitUpdater) Update() error {
	var errs []error
	for _, updater := range list {
		err := updater.ExplicitUpdate()
		errs = append(errs, err)
	}
	return utilerrors.NewAggregate(errs)
}
