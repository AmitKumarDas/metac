/*
Copyright 2017 Google Inc.
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

package composite

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	mcclientset "openebs.io/metac/client/generated/clientset/versioned"
	mclisters "openebs.io/metac/client/generated/listers/metacontroller/v1alpha1"
	"openebs.io/metac/controller/common"
	"openebs.io/metac/controller/common/finalizer"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
	dynamiccontrollerref "openebs.io/metac/dynamic/controllerref"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	dynamicinformer "openebs.io/metac/dynamic/informer"
	k8s "openebs.io/metac/third_party/kubernetes"
)

type parentController struct {
	api *v1alpha1.CompositeController

	resources      *dynamicdiscovery.APIResourceManager
	parentResource *dynamicdiscovery.APIResource

	mcClientSet  mcclientset.Interface
	dynClientSet *dynamicclientset.Clientset

	parentClient   *dynamicclientset.ResourceClient
	parentInformer *dynamicinformer.ResourceInformer

	revisionLister mclisters.ControllerRevisionLister

	stopCh, doneCh chan struct{}
	queue          workqueue.RateLimitingInterface

	updateStrategy updateStrategyMap
	childInformers common.ResourceInformerRegistryByVR

	finalizer *finalizer.Finalizer
}

func newParentController(
	resources *dynamicdiscovery.APIResourceManager,
	dynClientSet *dynamicclientset.Clientset,
	informerFactory *dynamicinformer.SharedInformerFactory,
	mcClient mcclientset.Interface,
	revisionLister mclisters.ControllerRevisionLister,
	api *v1alpha1.CompositeController,
) (pc *parentController, newErr error) {
	// Make a dynamic client for the parent resource.
	parentClient, err := dynClientSet.GetClientByResource(
		api.Spec.ParentResource.APIVersion,
		api.Spec.ParentResource.Resource,
	)
	if err != nil {
		return nil, err
	}
	parentResource := parentClient.APIResource

	updateStrategy, err := makeUpdateStrategyMap(resources, api)
	if err != nil {
		return nil, err
	}

	// Create informer for the parent resource.
	parentInformer, err := informerFactory.GetOrCreate(
		api.Spec.ParentResource.APIVersion,
		api.Spec.ParentResource.Resource,
	)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"can't create informer for parent resource %q %q",
			api.Spec.ParentResource.APIVersion,
			api.Spec.ParentResource.Resource,
		)
	}

	// Create informers for all child resources.
	childInformers := make(common.ResourceInformerRegistryByVR)
	defer func() {
		if newErr != nil {
			// If newParentController fails, Close() any informers we created
			// since Stop() will never be called.
			for _, childInformer := range childInformers {
				childInformer.Close()
			}
			parentInformer.Close()
		}
	}()
	for _, child := range api.Spec.ChildResources {
		childInformer, err := informerFactory.GetOrCreate(
			child.APIVersion,
			child.Resource,
		)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"can't create informer for child resource %q %q",
				child.APIVersion,
				child.Resource,
			)
		}
		childInformers.Set(child.APIVersion, child.Resource, childInformer)
	}

	pc = &parentController{
		api:            api,
		resources:      resources,
		mcClientSet:    mcClient,
		dynClientSet:   dynClientSet,
		childInformers: childInformers,
		parentClient:   parentClient,
		parentInformer: parentInformer,
		parentResource: parentResource,
		revisionLister: revisionLister,
		updateStrategy: updateStrategy,
		queue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"CompositeController-"+api.Name,
		),
		finalizer: &finalizer.Finalizer{
			Name:    "metac.openebs.io/compositecontroller-" + api.Name,
			Enabled: api.Spec.Hooks.Finalize != nil,
		},
	}

	return pc, nil
}

// String is the Stringer implementation for parentController instance
func (pc *parentController) String() string {
	return fmt.Sprintf(
		"%s/%s for %s",
		pc.api.Namespace, pc.api.Name, pc.parentResource.Kind,
	)
}

// Start triggers the reconciliation process of this controller
// instance
func (pc *parentController) Start(workerCount int) {
	// init the channels to signal cancellation
	// a single stop channel for all workers
	// done channel is triggered when all workers are stopped
	pc.stopCh = make(chan struct{})
	pc.doneCh = make(chan struct{})

	// Install event handlers. CompositeControllers can be created at any time,
	// so we have to assume the shared informers are already running. We can't
	// add event handlers in newParentController() since pc might be incomplete.
	parentHandlers := cache.ResourceEventHandlerFuncs{
		AddFunc:    pc.enqueueParentObject,
		UpdateFunc: pc.updateParentObject,
		DeleteFunc: pc.enqueueParentObject,
	}
	if pc.api.Spec.ResyncPeriodSeconds != nil {
		// Use a custom resync period if requested. This only applies to the parent.
		resyncPeriod := time.Duration(*pc.api.Spec.ResyncPeriodSeconds) * time.Second
		// Put a reasonable limit on it.
		if resyncPeriod < time.Second {
			resyncPeriod = time.Second
		}
		pc.parentInformer.Informer().AddEventHandlerWithResyncPeriod(parentHandlers, resyncPeriod)
	} else {
		pc.parentInformer.Informer().AddEventHandler(parentHandlers)
	}
	for _, childInformer := range pc.childInformers {
		childInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    pc.onChildAdd,
			UpdateFunc: pc.onChildUpdate,
			DeleteFunc: pc.onChildDelete,
		})
	}

	if workerCount <= 0 {
		workerCount = 5
	}

	go func() {
		defer close(pc.doneCh)
		defer utilruntime.HandleCrash()

		glog.Infof("Starting CompositeController %s", pc)
		defer glog.Infof("Shutting down CompositeController %s", pc)

		// Wait for dynamic client and all informers.
		glog.Infof(
			"CompositeController %s waiting for caches to sync", pc,
		)

		syncFuncs := make([]cache.InformerSynced, 0, 2+len(pc.api.Spec.ChildResources))
		syncFuncs = append(
			syncFuncs,
			pc.dynClientSet.HasSynced,
			pc.parentInformer.Informer().HasSynced,
		)
		for _, childInformer := range pc.childInformers {
			syncFuncs = append(syncFuncs, childInformer.Informer().HasSynced)
		}

		// decorate WaitForCacheSync with time taken and logging logic
		waitForCacheSync := k8s.CacheSyncTimeTaken(
			pc.String(),
			k8s.CacheSyncFailureAsError(
				pc.String(),
				cache.WaitForCacheSync,
			),
		)
		if !waitForCacheSync(pc.stopCh, syncFuncs...) {
			// We wait forever unless Stop() is called, so this isn't an error.
			glog.Warningf(
				"CompositeController %s cache sync never finished", pc,
			)
			return
		}

		glog.Infof("Starting %d workers for %s", workerCount, pc)
		var wg sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				wait.Until(pc.worker, time.Second, pc.stopCh)
			}()
		}
		wg.Wait()
	}()
}

// Stop triggers the cancel signal
func (pc *parentController) Stop() {
	// closing stop channel will signal all the workers
	// to stop reconciliation
	close(pc.stopCh)
	pc.queue.ShutDown()
	// wait till done channel is closed
	<-pc.doneCh

	// Remove event handlers and close informers (i.e. decrement the counter)
	// for all child resources.
	for _, informer := range pc.childInformers {
		informer.Informer().RemoveEventHandlers()
		informer.Close()
	}
	// Remove event handlers and close informer (i.e. decrement the counter)
	// for the parent resource.
	pc.parentInformer.Informer().RemoveEventHandlers()
	pc.parentInformer.Close()
}

// worker starts a forever reconciliation process for
// the enqueued parent resource
func (pc *parentController) worker() {
	for pc.processNextWorkItem() {
	}
}

// processNextWorkItem reconciles the current queue item
// i.e. parent resource
func (pc *parentController) processNextWorkItem() bool {
	key, quit := pc.queue.Get()
	if quit {
		return false
	}

	defer pc.queue.Done(key)
	err := pc.sync(key.(string))
	if err != nil {
		utilruntime.HandleError(errors.Wrapf(
			err,
			"failed to sync %v %q", pc.parentResource.Kind, key),
		)
		// reconcile failed; add this once more
		// default rate limit should hopefully avoid
		// a hot sync loop
		pc.queue.AddRateLimited(key)
		return true
	}

	// reconciliation was success
	pc.queue.Forget(key)
	return true
}

func (pc *parentController) enqueueParentObject(obj interface{}) {
	key, err := common.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(
			fmt.Errorf("couldn't get key for object %+v: %v", obj, err),
		)
		return
	}
	pc.queue.Add(key)
}

func (pc *parentController) enqueueParentObjectAfter(obj interface{}, delay time.Duration) {
	key, err := common.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(
			fmt.Errorf("couldn't get key for object %+v: %v", obj, err),
		)
		return
	}
	pc.queue.AddAfter(key, delay)
}

func (pc *parentController) updateParentObject(old, cur interface{}) {
	// We used to ignore our own status updates, but we don't anymore.
	// It's sometimes necessary for a hook to see its own status updates
	// so they know that the status was committed to storage.
	// This could cause endless sync hot-loops if your hook always returns a
	// different status (e.g. you have some incrementing counter).
	// Doing that is an anti-pattern anyway because status generation should be
	// idempotent if nothing meaningful has actually changed in the system.
	pc.enqueueParentObject(cur)
}

// resolveControllerRef returns the controller referenced by a
// ControllerRef, or nil if the ControllerRef could not be resolved
// to a matching controller of the correct Kind. In other words, it
// returns the matching parent resource based on the given controller
// reference.
func (pc *parentController) resolveControllerRef(
	childNamespace string,
	controllerRef *metav1.OwnerReference,
) *unstructured.Unstructured {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong APIGroup or Kind.
	apiGroup, _ := common.ParseAPIVersionToGroupVersion(controllerRef.APIVersion)
	if apiGroup != pc.parentResource.Group {
		return nil
	}
	if controllerRef.Kind != pc.parentResource.Kind {
		return nil
	}
	parentNamespace := ""
	if pc.parentResource.Namespaced {
		// If the parent is namespaced, it must be in the same namespace as the
		// child because controllerRef does not support cross-namespace references
		// (except for namespaced child -> cluster-scoped parent).
		parentNamespace = childNamespace
	}
	parent, err := pc.parentInformer.Lister().Get(parentNamespace, controllerRef.Name)
	if err != nil {
		return nil
	}
	if parent.GetUID() != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return parent
}

// onChildAdd eventually leads to enqueuing of its parent
func (pc *parentController) onChildAdd(obj interface{}) {
	child := obj.(*unstructured.Unstructured)

	if child.GetDeletionTimestamp() != nil {
		pc.onChildDelete(child)
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(child); controllerRef != nil {
		parent := pc.resolveControllerRef(child.GetNamespace(), controllerRef)
		if parent == nil {
			// The controllerRef isn't a parent we know about.
			return
		}
		glog.V(4).Infof(
			"%s %s: child %s %s/%s created or updated",
			pc.parentResource.Kind,
			parent.GetName(),
			child.GetKind(),
			child.GetNamespace(),
			child.GetName(),
		)
		pc.enqueueParentObject(parent)
		return
	}

	// Otherwise, it's an orphan. Get a list of all matching parents
	// and sync them to see if anyone wants to adopt it.
	parents := pc.findPotentialParents(child)
	if len(parents) == 0 {
		return
	}
	glog.V(4).Infof(
		"%s: orphan child %s %s/%s created or updated",
		pc.parentResource.Kind,
		child.GetKind(),
		child.GetNamespace(),
		child.GetName(),
	)
	for _, parent := range parents {
		pc.enqueueParentObject(parent)
	}
}

func (pc *parentController) onChildUpdate(old, cur interface{}) {
	oldChild := old.(*unstructured.Unstructured)
	curChild := cur.(*unstructured.Unstructured)

	// Don't sync if it's a no-op update (probably a relist/resync).
	// We don't care about resyncs for children; we rely on the parent resync.
	if oldChild.GetResourceVersion() == curChild.GetResourceVersion() {
		return
	}

	// Other than that, we treat updates the same as creates.
	// Level-triggered controllers shouldn't care what the old state was.
	pc.onChildAdd(cur)
}

func (pc *parentController) onChildDelete(obj interface{}) {
	child, ok := obj.(*unstructured.Unstructured)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(
				fmt.Errorf("%s: couldn't get object from tombstone %+v",
					pc.parentResource.Kind,
					obj,
				),
			)
			return
		}
		child, ok = tombstone.Obj.(*unstructured.Unstructured)
		if !ok {
			utilruntime.HandleError(
				fmt.Errorf(
					"%s: tombstone object is not *unstructured.Unstructured %#v",
					pc.parentResource.Kind,
					obj,
				),
			)
			return
		}
	}

	// If it's an orphan, there's nothing to do because we never adopt orphans
	// that are being deleted.
	controllerRef := metav1.GetControllerOf(child)
	if controllerRef == nil {
		return
	}

	// Sync the parent of this child (if it's ours).
	parent := pc.resolveControllerRef(child.GetNamespace(), controllerRef)
	if parent == nil {
		// The controllerRef isn't a parent we know about.
		return
	}
	glog.V(4).Infof(
		"%s %s: child %s %s/%s deleted",
		pc.parentResource.Kind,
		parent.GetName(),
		child.GetKind(),
		child.GetNamespace(),
		child.GetName(),
	)
	pc.enqueueParentObject(parent)
}

// findPotentialParents as the name suggests tries to find
// eligible parent objects (as per the parent resource declaration in
// composite controller) that can adopt the given child object.
//
// NOTE:
//	This is invoked when the given child object is not set
// with any controller reference.
func (pc *parentController) findPotentialParents(
	child *unstructured.Unstructured,
) []*unstructured.Unstructured {
	childLabels := labels.Set(child.GetLabels())

	var parents []*unstructured.Unstructured
	var err error
	if pc.parentResource.Namespaced {
		// If the parent is namespaced, it must be in the
		// same namespace as the child.
		parents, err = pc.parentInformer.Lister().ListNamespace(
			child.GetNamespace(), labels.Everything(),
		)
	} else {
		parents, err = pc.parentInformer.Lister().List(labels.Everything())
	}
	if err != nil {
		return nil
	}

	var matchingParents []*unstructured.Unstructured
	for _, parent := range parents {
		selector, err := pc.makeSelector(parent, nil)
		if err != nil || selector.Empty() {
			continue
		}
		if selector.Matches(childLabels) {
			matchingParents = append(matchingParents, parent)
		}
	}
	return matchingParents
}

// sync is the reconciliation logic as per composite controller
// instance's spec
func (pc *parentController) sync(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	glog.V(4).Infof("CompositeController %s: sync %s/%s", pc, namespace, name)

	parent, err := pc.parentInformer.Lister().Get(namespace, name)
	if apierrors.IsNotFound(err) {
		// Swallow the error since there's no point retrying if parent is gone.
		glog.V(4).Infof(
			"CompositeController %s: parent %s/%s has been deleted",
			pc, namespace, name,
		)
		return nil
	}
	if err != nil {
		return err
	}
	return pc.syncParentObject(parent)
}

// syncParentObject reconciles as per CompositeController specification
// by evaluating the provided parent resource
func (pc *parentController) syncParentObject(parent *unstructured.Unstructured) error {
	// Before taking any other action, add our finalizer (if desired).
	// This ensures we have a chance to clean up after any action we
	// later take.
	updatedParent, err := pc.finalizer.SyncObject(pc.parentClient, parent)
	if err != nil {
		// If we fail to do this, abort before doing anything else and requeue.
		return errors.Wrapf(
			err,
			"CompositeController %s: can't sync finalizer for %v/%v",
			pc,
			parent.GetNamespace(),
			parent.GetName(),
		)
	}
	parent = updatedParent

	// Claim all matching child resources, including orphan/adopt as necessary.
	observedChildren, err := pc.claimChildren(parent)
	if err != nil {
		return err
	}

	// Reconcile ControllerRevisions belonging to this parent.
	// Call the sync hook for each revision, then compute the overall status and
	// desired children, accounting for any rollout in progress.
	syncResult, err := pc.syncRevisions(parent, observedChildren)
	if err != nil {
		return err
	}
	desiredChildren :=
		common.MakeAnyUnstructRegistryByReference(parent, syncResult.Children)

	// Enqueue a delayed resync, if requested.
	if syncResult.ResyncAfterSeconds > 0 {
		pc.enqueueParentObjectAfter(
			parent,
			time.Duration(syncResult.ResyncAfterSeconds*float64(time.Second)),
		)
	}

	// If all revisions agree that they've finished finalizing,
	// remove our finalizer.
	if syncResult.Finalized {
		updatedParent, err := pc.parentClient.Namespace(parent.GetNamespace()).
			RemoveFinalizer(parent, pc.finalizer.Name)
		if err != nil {
			return errors.Wrapf(
				err,
				"CompositeController %s: can't remove finalizer for parent %s/%s",
				pc,
				parent.GetNamespace(),
				parent.GetName(),
			)
		}
		parent = updatedParent
	}

	// Enforce invariants between parent selector and child labels.
	selector, err := pc.makeSelector(parent, nil)
	if err != nil {
		return err
	}
	for _, group := range desiredChildren {
		for _, child := range group {
			// We don't use GetLabels() because that swallows conversion errors.
			childLabels, _, err := unstructured.NestedStringMap(
				child.UnstructuredContent(), "metadata", "labels",
			)
			if err != nil {
				return errors.Wrapf(
					err,
					"CompositeController %s: invalid labels on desired child %s %s/%s",
					pc,
					child.GetKind(),
					child.GetNamespace(),
					child.GetName())
			}
			// If selector generation is enabled, add the controller-uid label
			// to all desired children so they match the generated selector.
			if pc.api.Spec.GenerateSelector != nil && *pc.api.Spec.GenerateSelector {
				if childLabels == nil {
					childLabels = make(map[string]string, 1)
				}
				if _, found := childLabels["controller-uid"]; !found {
					childLabels["controller-uid"] = string(parent.GetUID())
					child.SetLabels(childLabels)
				}
			}
			// Make sure all desired children match the parent's selector.
			// We consider it user error to try to create children that would be
			// immediately orphaned.
			if !selector.Matches(labels.Set(childLabels)) {
				return errors.Errorf(
					"CompositeController %s: labels on desired child %s %s/%s don't match parent selector",
					pc,
					child.GetKind(), child.GetNamespace(), child.GetName(),
				)
			}
		}
	}

	// Reconcile child objects belonging to this parent.
	// Remember manage error, but continue to update status regardless.
	//
	// We only manage children if the parent is "alive" (not pending deletion),
	// or if it's pending deletion and we have a `finalize` hook.
	var manageErr error
	if parent.GetDeletionTimestamp() == nil || pc.finalizer.ShouldFinalize(parent) {
		// Reconcile children.
		if err := common.ManageChildren(
			pc.dynClientSet,
			pc.updateStrategy,
			parent,
			observedChildren,
			desiredChildren,
		); err != nil {
			manageErr = errors.Wrapf(
				err,
				"CompositeController %s: can't reconcile children for %s/%s",
				pc,
				parent.GetNamespace(),
				parent.GetName(),
			)
		}
	}

	// Update parent status.
	// We'll want to make sure this happens after manageChildren once
	// we support observedGeneration.
	if _, err := pc.updateParentStatus(parent, syncResult.Status); err != nil {
		return errors.Wrapf(
			err,
			"CompositeController %s: can't update status for %s/%s",
			pc,
			parent.GetNamespace(), parent.GetName(),
		)
	}

	return manageErr
}

// makeSelector builds label selector based on parent
// instance i.e. generateSelector or .spec.selector
func (pc *parentController) makeSelector(
	parent *unstructured.Unstructured,
	extraMatchLabels map[string]string,
) (labels.Selector, error) {
	labelSelector := &metav1.LabelSelector{}

	if pc.api.Spec.GenerateSelector != nil && *pc.api.Spec.GenerateSelector {
		// Select by controller-uid, like Job does.
		// Any selector on the parent is ignored in this case.
		labelSelector = metav1.AddLabelToSelector(
			labelSelector, "controller-uid", string(parent.GetUID()),
		)
	} else {
		// Get the parent's LabelSelector.
		err := k8s.GetNestedFieldInto(
			labelSelector, parent.UnstructuredContent(), "spec", "selector",
		)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"CompositeController %s: can't get label selector from %s/%s",
				pc,
				parent.GetNamespace(),
				parent.GetName(),
			)
		}
		// An empty selector doesn't make sense for a CompositeController parent.
		// This is likely user error, and could be dangerous (selecting everything).
		if len(labelSelector.MatchLabels) == 0 && len(labelSelector.MatchExpressions) == 0 {
			return nil, errors.Errorf(
				"CompositeController %s: .spec.selector of %s/%s must have either matchLabels, matchExpressions, or both",
				pc,
				parent.GetNamespace(),
				parent.GetName(),
			)
		}
	}

	for key, value := range extraMatchLabels {
		labelSelector = metav1.AddLabelToSelector(labelSelector, key, value)
	}

	selector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"CompositeController %s: can't convert label selector (%#v) for %s/%s",
			pc,
			labelSelector,
			parent.GetNamespace(),
			parent.GetName(),
		)
	}
	return selector, nil
}

// canAdoptFunc returns a nil if the given parent can be adopted
// or error if parent can not be adopted
func (pc *parentController) canAdoptFunc(parent *unstructured.Unstructured) func() error {
	return k8s.ErrorOnDeletionTimestamp(func() (metav1.Object, error) {
		// Make sure this is always an uncached read.
		fresh, err := pc.parentClient.Namespace(parent.GetNamespace()).Get(
			parent.GetName(), metav1.GetOptions{},
		)
		if err != nil {
			return nil, err
		}
		if fresh.GetUID() != parent.GetUID() {
			return nil, errors.Errorf(
				"CompositeController %s: original %s/%s is gone: got uid %v, wanted %v",
				pc,
				parent.GetNamespace(),
				parent.GetName(),
				fresh.GetUID(),
				parent.GetUID(),
			)
		}
		return fresh, nil
	})
}

// claimChildren claims resources based on the provided
// parent resource
//
// NOTE:
//	Claim process can either adopt a child with the provided
// parent resource or release already adopted child based on
// the current match
func (pc *parentController) claimChildren(
	parent *unstructured.Unstructured,
) (common.AnyUnstructRegistry, error) {
	// Set up values common to all child types.
	parentNamespace := parent.GetNamespace()
	parentGVK := pc.parentResource.GroupVersionKind()
	selector, err := pc.makeSelector(parent, nil)
	if err != nil {
		return nil, err
	}
	canAdoptFunc := pc.canAdoptFunc(parent)

	// Claim all child types.
	childMap := make(common.AnyUnstructRegistry)
	for _, child := range pc.api.Spec.ChildResources {
		// List all objects of the child kind in the parent object's namespace,
		// or in all namespaces if the parent is cluster-scoped.
		childClient, err :=
			pc.dynClientSet.GetClientByResource(child.APIVersion, child.Resource)
		if err != nil {
			return nil, err
		}
		childInformer := pc.childInformers.Get(child.APIVersion, child.Resource)
		if childInformer == nil {
			return nil, errors.Errorf(
				"CompositeController %s: no informer for child %q in apiVersion %q",
				pc,
				child.Resource,
				child.APIVersion,
			)
		}
		var all []*unstructured.Unstructured
		if pc.parentResource.Namespaced {
			all, err = childInformer.Lister().ListNamespace(parentNamespace, labels.Everything())
		} else {
			all, err = childInformer.Lister().List(labels.Everything())
		}
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"CompositeController %s: can't list %q children",
				pc,
				childClient.Kind,
			)
		}

		// Always include the requested groups, even if there are no entries.
		childMap.InitGroupByVK(child.APIVersion, childClient.Kind)

		// Handle orphan/adopt and filter by owner+selector.
		crm := dynamiccontrollerref.NewUnstructClaimManager(
			childClient,
			parent,
			selector,
			parentGVK,
			childClient.GroupVersionKind(),
			canAdoptFunc,
		)
		children, err := crm.BulkClaim(all)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"CompositeController %s: can't claim %q children",
				pc,
				childClient.Kind,
			)
		}

		// Add children to map by name.
		// Note that we limit each parent to only working within its own namespace.
		for _, obj := range children {
			childMap.InsertByReference(parent, obj)
		}
	}
	return childMap, nil
}

func (pc *parentController) updateParentStatus(parent *unstructured.Unstructured, status map[string]interface{}) (*unstructured.Unstructured, error) {
	// Inject ObservedGeneration before comparing with old status,
	// so we're comparing against the final form we desire.
	if status == nil {
		status = make(map[string]interface{})
	}
	status["observedGeneration"] = parent.GetGeneration()

	// Overwrite .status field of parent object without touching other parts.
	// We can't use Patch() because we need to ensure that the UID matches.
	return pc.parentClient.Namespace(parent.GetNamespace()).AtomicStatusUpdate(parent, func(obj *unstructured.Unstructured) bool {
		oldStatus := obj.UnstructuredContent()["status"]
		if reflect.DeepEqual(oldStatus, status) {
			// Nothing to do.
			return false
		}

		obj.UnstructuredContent()["status"] = status
		return true
	})
}
