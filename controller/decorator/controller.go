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

package decorator

import (
	"fmt"
	"reflect"
	"strings"
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
	"openebs.io/metac/controller/common"
	"openebs.io/metac/controller/common/finalizer"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	dynamicinformer "openebs.io/metac/dynamic/informer"
	dynamicobject "openebs.io/metac/dynamic/object"
	k8s "openebs.io/metac/third_party/kubernetes"
)

const (
	// decoratorControllerAnnotation is the annotation key
	// to hold the name of the specific decoratorController
	// instance
	decoratorControllerAnnotation = "metac.openebs.io/decorator-controller"
)

type decoratorController struct {
	// schema this controller is based on
	schema *v1alpha1.DecoratorController

	// discovered resourceManager
	resourceManager *dynamicdiscovery.APIResourceManager

	// hold the parent kinds
	parentKinds common.ResourceRegistryByGK

	// selector options to filter the parent
	parentSelector *decoratorSelector

	// dynamic clientset for resources managed by this
	// controller instance
	dynCliSet *dynamicclientset.Clientset

	// channels to flag stopping or completing the
	// reconcile process
	stopCh, doneCh chan struct{}
	// watched resources will be queued here
	// before being reconciled
	queue workqueue.RateLimitingInterface

	// the strategy to follow during reconcile
	updateStrategy updateStrategyMap

	// informers are needed to capture the changes against
	// the watched resource i.e. parent & to list the attachments
	// i.e. children from the cache thereby reducing the pressure
	// on kube api server
	parentInformers common.ResourceInformerRegistryByVR
	childInformers  common.ResourceInformerRegistryByVR

	// instance that deals with this controller's finalizer
	// if any
	finalizer *finalizer.Finalizer
}

// newDecoratorController returns a new instance of decorator
// controller with required parent & child informers, selectors,
// update strategy & so on.
func newDecoratorController(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	dynCliSet *dynamicclientset.Clientset,
	informerFactory *dynamicinformer.SharedInformerFactory,
	schema *v1alpha1.DecoratorController,
) (controller *decoratorController, newErr error) {

	c := &decoratorController{
		schema:          schema,
		resourceManager: resourceMgr,
		dynCliSet:       dynCliSet,

		parentKinds:     make(common.ResourceRegistryByGK),
		parentInformers: make(common.ResourceInformerRegistryByVR),
		childInformers:  make(common.ResourceInformerRegistryByVR),

		queue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"DecoratorController-"+schema.Name,
		),

		finalizer: &finalizer.Finalizer{
			// finalizer manager is entrusted with this finalier name
			Name: "metac.openebs.io/decoratorcontroller-" + schema.Name,
			// gets enabled if Finalize property is set
			Enabled: schema.Spec.Hooks.Finalize != nil,
		},
	}

	var err error

	c.parentSelector, err = newDecoratorSelector(resourceMgr, schema)
	if err != nil {
		return nil, err
	}

	// Keep a list of parent resource info from discovery.
	for _, parent := range schema.Spec.Resources {
		resource := resourceMgr.GetByResource(parent.APIVersion, parent.Resource)
		if resource == nil {
			return nil, errors.Errorf(
				"can't find parent resource %q in apiVersion %q",
				parent.Resource,
				parent.APIVersion,
			)
		}
		c.parentKinds.Set(resource.Group, resource.Kind, resource)
	}

	// Remember the update strategy for each child type.
	c.updateStrategy, err = makeUpdateStrategyMap(resourceMgr, schema)
	if err != nil {
		return nil, err
	}

	// close the successfully created informers for parent
	// and child resources in-case of any errors during
	// initialization
	defer func() {
		if newErr != nil {
			// If newDecoratorController fails, Close() any informers we created
			// since Stop() will never be called.
			for _, informer := range c.childInformers {
				informer.Close()
			}
			for _, informer := range c.parentInformers {
				informer.Close()
			}
		}
	}()

	// init required parent informers
	for _, parent := range schema.Spec.Resources {
		informer, err := informerFactory.GetOrCreate(parent.APIVersion, parent.Resource)
		if err != nil {
			return nil, errors.Errorf(
				"can't create informer for parent resource: %v", err,
			)
		}
		c.parentInformers.Set(parent.APIVersion, parent.Resource, informer)
	}

	// init required child informers
	for _, child := range schema.Spec.Attachments {
		informer, err := informerFactory.GetOrCreate(child.APIVersion, child.Resource)
		if err != nil {
			return nil, errors.Errorf(
				"can't create informer for child resource: %v", err,
			)
		}
		c.childInformers.Set(child.APIVersion, child.Resource, informer)
	}

	return c, nil
}

// Start starts the decorator controller based on its fields
// that were initialised earlier (mostly via its constructor)
func (c *decoratorController) Start(workerCount int) {
	// init the channels with empty structs
	c.stopCh = make(chan struct{})
	c.doneCh = make(chan struct{})

	// Install event handlers. DecoratorControllers can be created at any time,
	// so we have to assume the shared informers are already running. We can't
	// add event handlers in newDecoratorController() since c might be incomplete.
	parentHandlers := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueueParentObject,
		UpdateFunc: c.updateParentObject,
		DeleteFunc: c.enqueueParentObject,
	}
	var resyncPeriod time.Duration
	if c.schema.Spec.ResyncPeriodSeconds != nil {
		// Use a custom resync period if requested
		// NOTE: This only applies to the parent
		resyncPeriod = time.Duration(*c.schema.Spec.ResyncPeriodSeconds) * time.Second
		// Put a reasonable limit on it.
		if resyncPeriod < time.Second {
			resyncPeriod = time.Second
		}
	}
	for _, informer := range c.parentInformers {
		if resyncPeriod != 0 {
			informer.Informer().AddEventHandlerWithResyncPeriod(parentHandlers, resyncPeriod)
		} else {
			informer.Informer().AddEventHandler(parentHandlers)
		}
	}
	for _, informer := range c.childInformers {
		informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onChildAdd,
			UpdateFunc: c.onChildUpdate,
			DeleteFunc: c.onChildDelete,
		})
	}

	if workerCount <= 0 {
		workerCount = 5
	}

	go func() {
		// close done channel i.e. mark closure of this start invocation
		defer close(c.doneCh)
		// provide the ability to run operations after panics
		defer utilruntime.HandleCrash()

		glog.Infof("Starting DecoratorController %v", c.schema.Name)
		defer glog.Infof("Shutting down DecoratorController %v", c.schema.Name)

		// Wait for dynamic client and all informers.
		glog.Infof("Waiting for DecoratorController %v caches to sync", c.schema.Name)
		syncFuncs := make(
			[]cache.InformerSynced,
			0,
			1+len(c.schema.Spec.Resources)+len(c.schema.Spec.Attachments),
		)
		for _, informer := range c.parentInformers {
			syncFuncs = append(syncFuncs, informer.Informer().HasSynced)
		}
		for _, informer := range c.childInformers {
			syncFuncs = append(syncFuncs, informer.Informer().HasSynced)
		}
		if !k8s.WaitForCacheSync(c.schema.Name, c.stopCh, syncFuncs...) {
			// We wait forever unless Stop() is called, so this isn't an error.
			glog.Warningf(
				"DecoratorController %v cache sync never finished", c.schema.Name,
			)
			return
		}

		glog.Infof("Starting %d workers for %v", workerCount, c.schema.Name)
		var wg sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				wait.Until(c.worker, time.Second, c.stopCh)
			}()
		}
		wg.Wait()
	}()
}

func (c *decoratorController) Stop() {
	// closing stopCh will unblock all the logics where this
	// channel was passed earlier. This triggers closing of
	// doneCh as well
	close(c.stopCh)
	c.queue.ShutDown()

	// IMO since nothing is pushed into doneCh, this will block
	// till doneCh is closed.
	//
	// Note: doneCh will be closed after all the workers are
	// stopped via above close(c.stopCh) invocation
	<-c.doneCh

	// Remove event handlers and close informers for all child resources.
	for _, informer := range c.childInformers {
		informer.Informer().RemoveEventHandlers()
		informer.Close()
	}
	// Remove event handlers and close informer for all parent resources.
	for _, informer := range c.parentInformers {
		informer.Informer().RemoveEventHandlers()
		informer.Close()
	}
}

// worker works for ever. Its only work is to process the
// workitem i.e. the observed resource
func (c *decoratorController) worker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem executes the reconcile logic of the
// resource that is currently received as part of watch
//
// NOTE:
// 	It needs to return true most of the times even in case of
// runtime errors to let it being called in a forever loop
//
// NOTE:
//	It returns false only when queue has been marked for shutdown
func (c *decoratorController) processNextWorkItem() bool {
	// queue will give us the next item (parent resource in this case)
	// to be reconciled unless shutdown was invoked against this queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	// real reconcile logic happens here
	err := c.sync(key.(string))
	if err != nil {
		utilruntime.HandleError(
			errors.Errorf("failed to sync %v %q: %v", c.schema.Name, key, err),
		)
		c.queue.AddRateLimited(key)
		return true
	}

	c.queue.Forget(key)
	return true
}

// enqueueParentObject as the name suggests evaluates if the parent
// resource is eligible to be reconciled. In other words, if the
// given parent resource is eligible it will be added to this controller
// queue to be extracted later & reconciled.
func (c *decoratorController) enqueueParentObject(obj interface{}) {
	// If the parent doesn't match our selector, and it doesn't have our
	// finalizer, we don't care about it.
	if parent, ok := obj.(*unstructured.Unstructured); ok {
		if !c.parentSelector.Matches(parent) &&
			!dynamicobject.HasFinalizer(parent, c.finalizer.Name) {
			return
		}
	}

	key, err := parentQueueKey(obj)
	if err != nil {
		utilruntime.HandleError(
			errors.Errorf("couldn't get key for object %+v: %v", obj, err),
		)
		return
	}
	c.queue.Add(key)
}

func (c *decoratorController) enqueueParentObjectAfter(obj interface{}, delay time.Duration) {
	key, err := parentQueueKey(obj)
	if err != nil {
		utilruntime.HandleError(
			errors.Errorf("couldn't get key for object %+v: %v", obj, err),
		)
		return
	}
	c.queue.AddAfter(key, delay)
}

func (c *decoratorController) updateParentObject(old, cur interface{}) {
	// TODO(enisoc): Is there any way to avoid resyncing after our own updates?
	c.enqueueParentObject(cur)
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (c *decoratorController) resolveControllerRef(
	childNamespace string,
	controllerRef *metav1.OwnerReference,
) *unstructured.Unstructured {
	// Is the controllerRef pointing to one of the parent resources we care about?
	// Only look at the group and kind; it doesn't matter if the controller uses
	// a different version than we do.
	apiGroup, _ := common.ParseAPIVersionToGroupVersion(controllerRef.APIVersion)
	resource := c.parentKinds.Get(apiGroup, controllerRef.Kind)
	if resource == nil {
		// It's not one of the resources we care about.
		return nil
	}
	// Get the lister for this resource.
	informer := c.parentInformers.Get(resource.APIVersion, resource.Name)
	if informer == nil {
		return nil
	}
	// We can't look up by UID, so look up by Namespace/Name and then verify UID.
	parentNamespace := ""
	if resource.Namespaced {
		// If the parent is namespaced, it must be in the same namespace as the
		// child because controllerRef does not support cross-namespace references
		// (except for namespaced child -> cluster-scoped parent).
		parentNamespace = childNamespace
	}
	parent, err := informer.Lister().Get(parentNamespace, controllerRef.Name)
	if err != nil {
		return nil
	}
	if parent.GetUID() != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	if !c.parentSelector.Matches(parent) &&
		!dynamicobject.HasFinalizer(parent, c.finalizer.Name) {
		// If the parent doesn't match our selector and doesn't have our finalizer,
		// we don't care about it.
		return nil
	}
	return parent
}

// onChildAdd accepts a child object, finds the respective
// parent resource and enqueues it (i.e. parent resource)
func (c *decoratorController) onChildAdd(obj interface{}) {
	child := obj.(*unstructured.Unstructured)

	if child.GetDeletionTimestamp() != nil {
		c.onChildDelete(child)
		return
	}

	// If it has no ControllerRef, we don't care.
	// DecoratorController doesn't do adoption since there are no child selectors.
	controllerRef := metav1.GetControllerOf(child)
	if controllerRef == nil {
		return
	}

	parent := c.resolveControllerRef(child.GetNamespace(), controllerRef)
	if parent == nil {
		// The controllerRef isn't a parent we know about.
		return
	}
	glog.V(4).Infof(
		"DecoratorController %v: %v %v/%v: child %v %v created or updated",
		c.schema.Name,
		parent.GetKind(),
		parent.GetNamespace(),
		parent.GetName(),
		child.GetKind(),
		child.GetName(),
	)
	c.enqueueParentObject(parent)
}

func (c *decoratorController) onChildUpdate(old, cur interface{}) {
	oldChild := old.(*unstructured.Unstructured)
	curChild := cur.(*unstructured.Unstructured)

	// Don't sync if it's a no-op update (probably a relist/resync).
	// We don't care about resyncs for children; we rely on the parent resync.
	if oldChild.GetResourceVersion() == curChild.GetResourceVersion() {
		return
	}

	// Other than that, we treat updates the same as creates.
	// Level-triggered controllers shouldn't care what the old state was.
	c.onChildAdd(cur)
}

// onChildDelete accepts a child object, finds the respective
// parent resource and enqueues it (i.e. parent resource)
func (c *decoratorController) onChildDelete(obj interface{}) {
	child, ok := obj.(*unstructured.Unstructured)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(
				errors.Errorf("couldn't get object from tombstone %+v", obj),
			)
			return
		}
		child, ok = tombstone.Obj.(*unstructured.Unstructured)
		if !ok {
			utilruntime.HandleError(
				errors.Errorf(
					"tombstone contained object that is not *unstructured.Unstructured %#v",
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
	parent := c.resolveControllerRef(child.GetNamespace(), controllerRef)
	if parent == nil {
		// The controllerRef isn't a parent we know about.
		return
	}
	glog.V(4).Infof(
		"DecoratorController %v: %v %v/%v: child %v %v deleted",
		c.schema.Name,
		parent.GetKind(),
		parent.GetNamespace(),
		parent.GetName(),
		child.GetKind(),
		child.GetName(),
	)
	c.enqueueParentObject(parent)
}

// sync reconciles the parent resource represented by this provided
// key
func (c *decoratorController) sync(key string) error {
	apiVersion, kind, namespace, name, err := splitParentQueueKey(key)
	if err != nil {
		return err
	}

	resource := c.resourceManager.GetByKind(apiVersion, kind)
	if resource == nil {
		return errors.Errorf("can't find kind %q in apiVersion %q", kind, apiVersion)
	}
	informer := c.parentInformers.Get(apiVersion, resource.Name)
	if informer == nil {
		return errors.Errorf(
			"no informer for parent resource %q in apiVersion %q",
			resource.Name,
			apiVersion,
		)
	}
	parent, err := informer.Lister().Get(namespace, name)
	if apierrors.IsNotFound(err) {
		// Swallow the error since there's no point retrying if the parent is gone.
		glog.V(4).Infof("%v %v/%v has been deleted", kind, namespace, name)
		return nil
	}
	if err != nil {
		return err
	}
	return c.syncParentObject(parent)
}

func (c *decoratorController) syncParentObject(parent *unstructured.Unstructured) error {
	// If it doesn't match our selector, and it doesn't have our finalizer,
	// ignore it.
	if !c.parentSelector.Matches(parent) &&
		!dynamicobject.HasFinalizer(parent, c.finalizer.Name) {
		return nil
	}

	glog.V(4).Infof(
		"DecoratorController %v: sync %v %v/%v",
		c.schema.Name, parent.GetKind(), parent.GetNamespace(), parent.GetName(),
	)

	parentClient, err :=
		c.dynCliSet.GetClientByKind(parent.GetAPIVersion(), parent.GetKind())
	if err != nil {
		return errors.Wrapf(
			err,
			"can't get client for parent resource %v %v/%v",
			parent.GetKind(),
			parent.GetNamespace(),
			parent.GetName(),
		)
	}

	// Before taking any other action, add our finalizer (if desired).
	// This ensures we have a chance to clean up after any action we later take.
	updatedParent, err := c.finalizer.SyncObject(parentClient, parent)
	if err != nil {
		// If we fail to do this, abort before doing anything else and requeue.
		return errors.Wrapf(
			err,
			"can't sync finalizer for %v %v/%v",
			parent.GetKind(),
			parent.GetNamespace(),
			parent.GetName(),
		)
	}
	parent = updatedParent

	// Check the finalizer again in case we just removed it.
	if !c.parentSelector.Matches(parent) &&
		!dynamicobject.HasFinalizer(parent, c.finalizer.Name) {
		return nil
	}

	// List all children belonging to this parent, of the kinds we care about.
	// This only lists the children we created. Existing children are ignored.
	observedChildren, err := c.getChildren(parent)
	if err != nil {
		return err
	}

	// Call the sync hook to get the desired annotations and children.
	syncRequest := &SyncHookRequest{
		Controller:  c.schema,
		Object:      parent,
		Attachments: observedChildren,
	}
	syncResult, err := c.callSyncHook(syncRequest)
	if err != nil {
		return err
	}
	desiredChildren :=
		common.MakeAnyUnstructRegistryByReference(parent, syncResult.Attachments)

	// Enqueue a delayed resync, if requested.
	if syncResult.ResyncAfterSeconds > 0 {
		c.enqueueParentObjectAfter(
			parent, time.Duration(syncResult.ResyncAfterSeconds*float64(time.Second)),
		)
	}

	// Set desired labels and annotations on parent.
	// Also remove finalizer if requested.
	// Make a copy since parent is from the cache.
	updatedParent = parent.DeepCopy()
	parentLabels := updatedParent.GetLabels()
	if parentLabels == nil {
		parentLabels = make(map[string]string)
	}
	parentAnnotations := updatedParent.GetAnnotations()
	if parentAnnotations == nil {
		parentAnnotations = make(map[string]string)
	}
	parentStatus := k8s.GetNestedObject(updatedParent.Object, "status")
	if syncResult.Status == nil {
		// A null .status in the sync response means leave it unchanged.
		syncResult.Status = parentStatus
	}

	labelsChanged := updateStringMap(parentLabels, syncResult.Labels)
	annotationsChanged := updateStringMap(parentAnnotations, syncResult.Annotations)
	statusChanged := !reflect.DeepEqual(parentStatus, syncResult.Status)

	// Only do the update if something changed.
	if labelsChanged || annotationsChanged || statusChanged ||
		(syncResult.Finalized && dynamicobject.HasFinalizer(parent, c.finalizer.Name)) {
		updatedParent.SetLabels(parentLabels)
		updatedParent.SetAnnotations(parentAnnotations)
		k8s.SetNestedField(updatedParent.Object, syncResult.Status, "status")

		if statusChanged && parentClient.HasSubresource("status") {
			// The regular Update below will ignore changes to .status
			// so we do it separately.
			result, err := parentClient.Namespace(parent.GetNamespace()).
				UpdateStatus(updatedParent, metav1.UpdateOptions{})
			if err != nil {
				return errors.Errorf("can't update status: %v", err)
			}
			// The Update below needs to use the latest ResourceVersion.
			updatedParent.SetResourceVersion(result.GetResourceVersion())
		}

		if syncResult.Finalized {
			dynamicobject.RemoveFinalizer(updatedParent, c.finalizer.Name)
		}

		glog.V(4).Infof("DecoratorController %v: updating %v %v/%v", c.schema.Name, parent.GetKind(), parent.GetNamespace(), parent.GetName())
		_, err = parentClient.
			Namespace(parent.GetNamespace()).Update(updatedParent, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err,
				"can't update %v %v/%v",
				parent.GetKind(), parent.GetNamespace(), parent.GetName(),
			)
		}
	}

	// Add an annotation to all desired children to remember that they were
	// created by this decorator.
	for _, group := range desiredChildren {
		for _, child := range group {
			ann := child.GetAnnotations()
			if ann[decoratorControllerAnnotation] == c.schema.Name {
				continue
			}
			if ann == nil {
				ann = make(map[string]string)
			}
			ann[decoratorControllerAnnotation] = c.schema.Name
			child.SetAnnotations(ann)
		}
	}

	// Reconcile child objects belonging to this parent.
	// Remember manage error, but continue to update status regardless.
	//
	// We only manage children if the parent is "alive" (not pending deletion),
	// or if it's pending deletion and we have a `finalize` hook.
	var manageErr error
	if parent.GetDeletionTimestamp() == nil || c.finalizer.ShouldFinalize(parent) {
		// Reconcile children.
		err := common.ManageChildren(
			c.dynCliSet, c.updateStrategy, parent, observedChildren, desiredChildren,
		)
		if err != nil {
			manageErr = errors.Wrapf(
				err,
				"can't reconcile children for %v %v/%v",
				parent.GetKind(), parent.GetNamespace(), parent.GetName(),
			)
		}
	}

	return manageErr
}

// getChildren returns the child resources of the given parent
// resource as declared in this decorator controller resource
func (c *decoratorController) getChildren(
	parent *unstructured.Unstructured,
) (common.AnyUnstructRegistry, error) {

	parentUID := parent.GetUID()
	parentNamespace := parent.GetNamespace()
	childMap := make(common.AnyUnstructRegistry)

	for _, child := range c.schema.Spec.Attachments {
		// List all objects of the child kind in the parent object's namespace,
		// or in all namespaces if the parent is cluster-scoped.
		informer := c.childInformers.Get(child.APIVersion, child.Resource)
		if informer == nil {
			return nil, errors.Errorf(
				"no informer for child resource %q in apiVersion %q",
				child.Resource, child.APIVersion,
			)
		}
		var all []*unstructured.Unstructured
		var err error
		if parentNamespace != "" {
			all, err = informer.Lister().ListNamespace(
				parentNamespace, labels.Everything(),
			)
		} else {
			all, err = informer.Lister().List(labels.Everything())
		}
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"can't list children for resource %q in apiVersion %q",
				child.Resource, child.APIVersion,
			)
		}

		// Always include the requested groups, even if there are no entries.
		resource := c.resourceManager.GetByResource(child.APIVersion, child.Resource)
		if resource == nil {
			return nil, errors.Errorf(
				"can't find resource %q in apiVersion %q",
				child.Resource,
				child.APIVersion,
			)
		}
		childMap.InitGroupByVK(child.APIVersion, resource.Kind)

		// Take only the objects that belong to this parent,
		// and that were created by this decorator.
		for _, obj := range all {
			controllerRef := metav1.GetControllerOf(obj)
			if controllerRef == nil || controllerRef.UID != parentUID {
				continue
			}
			if obj.GetAnnotations()[decoratorControllerAnnotation] != c.schema.Name {
				continue
			}
			childMap.InsertByReference(parent, obj)
		}
	}
	return childMap, nil
}

// holds update strategies of various resources
type updateStrategyMap map[string]*v1alpha1.DecoratorControllerAttachmentUpdateStrategy

// Get returns the upgrade strategy based on
// the given api group & kind
func (m updateStrategyMap) Get(apiGroup, kind string) v1alpha1.ChildUpdateMethod {
	strategy := m.get(apiGroup, kind)
	if strategy == nil || strategy.Method == "" {
		// default
		return v1alpha1.ChildUpdateOnDelete
	}
	return strategy.Method
}

// get returns the controller's attachment's upgrade strategy
// based on the given api group & kind
func (m updateStrategyMap) get(apiGroup, kind string) *v1alpha1.DecoratorControllerAttachmentUpdateStrategy {
	return m[updateStrategyMapKey(apiGroup, kind)]
}

func updateStrategyMapKey(apiGroup, kind string) string {
	return fmt.Sprintf("%s.%s", kind, apiGroup)
}

func makeUpdateStrategyMap(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	dc *v1alpha1.DecoratorController,
) (updateStrategyMap, error) {
	m := make(updateStrategyMap)
	for _, child := range dc.Spec.Attachments {
		// no need to store ondelete strategy since
		// its the default anyways
		if child.UpdateStrategy != nil &&
			child.UpdateStrategy.Method != v1alpha1.ChildUpdateOnDelete {
			// this is done to map resource name to kind name
			resource := resourceMgr.GetByResource(child.APIVersion, child.Resource)
			if resource == nil {
				return nil, errors.Errorf(
					"can't find child resource %q in %v",
					child.Resource,
					child.APIVersion,
				)
			}
			// Ignore API version.
			apiGroup, _ := common.ParseAPIVersionToGroupVersion(child.APIVersion)
			key := updateStrategyMapKey(apiGroup, resource.Kind)
			m[key] = child.UpdateStrategy
		}
	}
	return m, nil
}

func parentQueueKey(obj interface{}) (string, error) {
	switch o := obj.(type) {
	case cache.DeletedFinalStateUnknown:
		return o.Key, nil
	case cache.ExplicitKey:
		return string(o), nil
	case *unstructured.Unstructured:
		return fmt.Sprintf(
			"%s:%s:%s:%s", o.GetAPIVersion(), o.GetKind(), o.GetNamespace(), o.GetName(),
		), nil
	default:
		return "", errors.Errorf(
			"can't get key for object of type %T; expected *unstructured.Unstructured",
			obj,
		)
	}
}

func splitParentQueueKey(key string) (apiVersion, kind, namespace, name string, err error) {
	parts := strings.SplitN(key, ":", 4)
	if len(parts) != 4 {
		return "", "", "", "", errors.Errorf("invalid parent key: %q", key)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

func updateStringMap(dest map[string]string, updates map[string]*string) (changed bool) {
	for k, v := range updates {
		if v == nil {
			// nil/null means delete the key
			if _, exists := dest[k]; exists {
				delete(dest, k)
				changed = true
			}
			continue
		}
		// Add/Update the key.
		oldValue, exists := dest[k]
		if !exists || oldValue != *v {
			dest[k] = *v
			changed = true
		}
	}
	return changed
}
