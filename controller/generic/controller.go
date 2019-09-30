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

package generic

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
	gctlAnnotationKeyPrefix = "gctl.metac.openebs.io/"
)

// Manager that handles a GenericController API resource
//
// TODO (@amitkumardas):
// Rename this to watchController & this file
// to watchcontroller.go
type genericControllerManager struct {
	// Controller resource / yaml
	Controller *v1alpha1.GenericController

	// ResourceManager is used to fetch server API resources
	ResourceManager *dynamicdiscovery.APIResourceManager

	// dynamic clientset to operate against resources managed
	// by this controller instance
	DynamicClientSet *dynamicclientset.Clientset

	// holds all watch API resources declared in this
	// GenericController yaml
	watchAPIRegistry common.ResourceRegistryByGK

	// selector to filter watch & attachment resources
	selector *Selector

	// channels to flag stopping or completing the
	// reconcile process
	stopCh, doneCh chan struct{}

	// watch resources will be queued here
	// before being reconciled
	watchQ workqueue.RateLimitingInterface

	// the strategy to follow during reconcile
	updateStrategies attachmentUpdateStrategies

	// informers are needed to capture the changes against
	// the watch resource & attachments from the cache
	// thereby reducing the pressure on kube api server
	watchInformers      common.ResourceInformerRegistryByVR
	attachmentInformers common.ResourceInformerRegistryByVR

	// instance that deals with this controller's finalizer
	// if any
	finalizer *finalizer.Manager
}

// String implements Stringer interface
func (mgr *genericControllerManager) String() string {
	if mgr.Controller == nil {
		// TODO (@amitkumardas):
		// Rename this to GenericWatchController
		return "GCtl-WatchCtl"
	}
	return fmt.Sprintf(
		"GCtl %s/%s: WatchCtl",
		mgr.Controller.Namespace, mgr.Controller.Name,
	)
}

// newGenericControllerManager returns a new instance of generic
// controller with required parent & child informers, selectors,
// update strategy & so on.
func newGenericControllerManager(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	dynClientset *dynamicclientset.Clientset,
	dynInformerFactory *dynamicinformer.SharedInformerFactory,
	ctrl *v1alpha1.GenericController,
) (ctrlMgr *genericControllerManager, newErr error) {

	mgr := &genericControllerManager{
		Controller:       ctrl,
		ResourceManager:  resourceMgr,
		DynamicClientSet: dynClientset,

		watchAPIRegistry: make(common.ResourceRegistryByGK),

		watchInformers:      make(common.ResourceInformerRegistryByVR),
		attachmentInformers: make(common.ResourceInformerRegistryByVR),

		watchQ: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"GenericController-"+ctrl.Namespace+"-"+ctrl.Name,
		),

		finalizer: &finalizer.Manager{
			// finalizer manager is entrusted with this finalizer name
			// this gets applied against the watch during finalize i.e.
			// handling deletion of watch resource
			Name: "protect.gctl.metac.openebs.io/" + ctrl.Namespace + "-" + ctrl.Name,
			// gets enabled if Finalize property is set
			Enabled: ctrl.Spec.Hooks.Finalize != nil,
		},
	}

	var err error

	mgr.selector, err = makeSelector(resourceMgr, ctrl)
	if err != nil {
		return nil, err
	}

	watchAPI := resourceMgr.GetByResource(
		ctrl.Spec.Watch.APIVersion, ctrl.Spec.Watch.Resource,
	)
	if watchAPI == nil {
		return nil, errors.Errorf(
			"%s: Can't find %q of %q",
			mgr, ctrl.Spec.Watch.Resource, ctrl.Spec.Watch.APIVersion,
		)
	}
	// NOTE:
	// We use a registry even though there is a single watch
	// we might remove this registry if we believe single
	// watch is good & sufficient in GenericController
	mgr.watchAPIRegistry.Set(watchAPI.Group, watchAPI.Kind, watchAPI)

	// Remember the update strategy for each attachment type.
	mgr.updateStrategies, err = makeUpdateStrategyForAttachments(
		resourceMgr, ctrl.Spec.Attachments,
	)
	if err != nil {
		return nil, err
	}

	// close the successfully created informers for resources
	// in-case of any errors during initialization
	defer func() {
		if newErr != nil {
			// If newController fails, Close() any informers we created
			// since Stop() will never be called.
			for _, informer := range mgr.attachmentInformers {
				informer.Close()
			}
			for _, informer := range mgr.watchInformers {
				informer.Close()
			}
		}
	}()

	// init watch informers
	informer, err := dynInformerFactory.GetOrCreate(
		ctrl.Spec.Watch.APIVersion, ctrl.Spec.Watch.Resource,
	)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"%s: Can't create informer for %q of %q",
			mgr, ctrl.Spec.Watch.Resource, ctrl.Spec.Watch.APIVersion,
		)
	}
	// NOTE:
	// This is a registry of watch informers even though GenericController
	// needs only one watch. This may be removed to a single informer
	// if we conclude that single watch is best for GenericController.
	mgr.watchInformers.Set(
		ctrl.Spec.Watch.APIVersion, ctrl.Spec.Watch.Resource, informer,
	)

	// initialise the informers for attachments
	for _, a := range ctrl.Spec.Attachments {
		informer, err := dynInformerFactory.GetOrCreate(a.APIVersion, a.Resource)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"%s: Can't create informer for %q of %q",
				mgr, a.Resource, a.APIVersion,
			)
		}
		mgr.attachmentInformers.Set(a.APIVersion, a.Resource, informer)
	}

	return mgr, nil
}

// Start starts the decorator controller based on its fields
// that were initialised earlier (mostly via its constructor)
func (mgr *genericControllerManager) Start() {
	// init the channels with empty structs
	mgr.stopCh = make(chan struct{})
	mgr.doneCh = make(chan struct{})

	// Install event handlers. GenericControllers can be created at any time,
	// so we have to assume the shared informers are already running. We can't
	// add event handlers in newController() since c might be incomplete.
	watchHandlers := cache.ResourceEventHandlerFuncs{
		AddFunc:    mgr.enqueueWatch,
		UpdateFunc: mgr.updateWatch,
		DeleteFunc: mgr.enqueueWatch,
	}
	var resyncPeriod time.Duration
	if mgr.Controller.Spec.ResyncPeriodSeconds != nil {
		// Use a custom resync period if requested
		// NOTE: This only applies to the parent
		resyncPeriod = time.Duration(*mgr.Controller.Spec.ResyncPeriodSeconds) * time.Second
		// Put a reasonable limit on it.
		if resyncPeriod < time.Second {
			resyncPeriod = time.Second
		}
	}
	for _, informer := range mgr.watchInformers {
		if resyncPeriod != 0 {
			informer.Informer().AddEventHandlerWithResyncPeriod(watchHandlers, resyncPeriod)
		} else {
			informer.Informer().AddEventHandler(watchHandlers)
		}
	}

	go func() {
		// close done channel i.e. mark closure of this start invocation
		defer close(mgr.doneCh)
		// provide the ability to run operations after panics
		defer utilruntime.HandleCrash()

		glog.Infof("Starting %s", mgr)
		defer glog.Infof("Shutting down %s", mgr)

		// Wait for dynamic client and all informers.
		glog.Infof("%s: Waiting for caches to sync", mgr)
		syncFuncs := make(
			[]cache.InformerSynced,
			0,
			1+1+len(mgr.Controller.Spec.Attachments),
		)
		for _, informer := range mgr.watchInformers {
			syncFuncs = append(syncFuncs, informer.Informer().HasSynced)
		}
		for _, informer := range mgr.attachmentInformers {
			syncFuncs = append(syncFuncs, informer.Informer().HasSynced)
		}
		if !k8s.WaitForCacheSync(mgr.Controller.Key(), mgr.stopCh, syncFuncs...) {
			// We wait forever unless Stop() is called, so this isn't an error.
			glog.Warningf("%s: Cache sync never finished", mgr)
			return
		}

		// ensure sufficient workers to reconcile good number of
		// GenericController resources simultaneously
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				wait.Until(mgr.worker, time.Second, mgr.stopCh)
			}()
		}
		wg.Wait()
	}()
}

func (mgr *genericControllerManager) Stop() {
	// closing stopCh will unblock all the logics where this
	// channel was passed earlier. This triggers closing of
	// doneCh as well
	close(mgr.stopCh)
	mgr.watchQ.ShutDown()

	// IMO since nothing is pushed into doneCh, this will block
	// till doneCh is closed.
	//
	// Note: doneCh will be closed after all the workers are
	// stopped via above close(c.stopCh) invocation
	<-mgr.doneCh

	// Remove event handlers and close informers for all attachment
	// resources.
	for _, informer := range mgr.attachmentInformers {
		informer.Informer().RemoveEventHandlers()
		informer.Close()
	}
	// Remove event handlers and close informer for all watched
	// resources.
	for _, informer := range mgr.watchInformers {
		informer.Informer().RemoveEventHandlers()
		informer.Close()
	}
}

// worker works for ever. Its only work is to process the
// workitem i.e. the observed resource
func (mgr *genericControllerManager) worker() {
	for mgr.processNextWorkItem() {
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
func (mgr *genericControllerManager) processNextWorkItem() bool {
	// queue will give us the next item (parent resource in this case)
	// to be reconciled unless shutdown was invoked against this queue
	key, quit := mgr.watchQ.Get()
	if quit {
		return false
	}
	defer mgr.watchQ.Done(key)

	// actual reconcile logic is invoked
	err := mgr.syncWatch(key.(string))
	if err != nil {
		utilruntime.HandleError(
			errors.Wrapf(err, "%s: Failed to sync %q", mgr, key),
		)
		mgr.watchQ.AddRateLimited(key)
		return true
	}

	mgr.watchQ.Forget(key)
	return true
}

// enqueueWatch as the name suggests enqueues the eligible watch
// resource to be reconciled during dequeue.
//
// In other words, if the given watch resource is eligible it will be
// added to this controller queue to be extracted later & reconciled.
func (mgr *genericControllerManager) enqueueWatch(obj interface{}) {
	// If the watched doesn't match our selector,
	// and it doesn't have our finalizer, we don't care about it.
	//
	// In other words, if resource has this controller's finalizer
	// or matches this controller's selectors, then this resource
	// belongs to this controller & hence should be queued
	if watchObj, ok := obj.(*unstructured.Unstructured); ok {
		isMatch := mgr.selector.Matches(watchObj)
		hasFinalizer := dynamicobject.HasFinalizer(watchObj, mgr.finalizer.Name)
		if !isMatch && !hasFinalizer {
			glog.V(4).Infof(
				"%s: Will not enqueue %s/%s of kind:%s: IsMatch=%t: HasFinalizer=%t",
				mgr, watchObj.GetNamespace(), watchObj.GetName(), watchObj.GetKind(),
				isMatch, hasFinalizer,
			)
			return
		}
	}

	glog.V(4).Infof("%s: Will try to enqueue %v", mgr, obj)

	key, err := makeWatchQueueKey(obj)
	if err != nil {
		glog.V(4).Infof(
			"%s: Enqueue failed: Can't make key from %+v: %v", mgr, obj, err,
		)
		utilruntime.HandleError(
			errors.Wrapf(err, "%s: Enqueue failed: Can't make key from %+v", mgr, obj),
		)
		return
	}
	mgr.watchQ.Add(key)
}

func (mgr *genericControllerManager) enqueueWatchAfter(obj interface{}, delay time.Duration) {
	key, err := makeWatchQueueKey(obj)
	if err != nil {
		glog.V(4).Infof(
			"%s: Enqueue failed: Can't make key from %+v: %v", mgr, obj, err,
		)
		utilruntime.HandleError(
			errors.Wrapf(err, "%s: Enqueue failed: Can't make key from %+v", mgr, obj),
		)
		return
	}
	mgr.watchQ.AddAfter(key, delay)
}

// updateWatch enqueues the watch object without any checks
func (mgr *genericControllerManager) updateWatch(old, cur interface{}) {
	mgr.enqueueWatch(cur)
}

// syncWatch reconciles the watch resource represented by this provided
// key
//
// NOTE:
//	Errors are logged as debug messages since errors may auto correct
// eventually
func (mgr *genericControllerManager) syncWatch(key string) error {
	var err error
	defer func() {
		if !glog.V(4) {
			return
		}
		if err != nil {
			glog.Warningf("%s: Can't sync watch %s: %v", mgr, key, err)
			return
		}
		glog.Infof("%s: Watch %s sync completed", mgr, key)
	}()

	apiVersion, kind, namespace, name, err := splitWatchQueueKey(key)
	if err != nil {
		return err
	}

	watchResource := mgr.ResourceManager.GetByKind(apiVersion, kind)
	if watchResource == nil {
		return errors.Errorf("%s: Can't find resource %s", mgr, key)
	}

	watchInformer := mgr.watchInformers.Get(apiVersion, watchResource.Name)
	if watchInformer == nil {
		return errors.Errorf("%s: Can't find informer %s", mgr, key)
	}

	watchObj, err := watchInformer.Lister().Get(namespace, name)
	if apierrors.IsNotFound(err) {
		// Swallow the error since there's no point retrying if the
		// watch is gone.
		glog.V(4).Infof("%s: Can't sync %s: Watch doesn't exist: %v", mgr, key, err)
		return nil
	}
	if err != nil {
		return err
	}

	// remember we use a defer statement to intercept error as debug log.
	// Hence, we dont return below invocation directly.
	err = mgr.syncWatchObj(watchObj)
	return err
}

func (mgr *genericControllerManager) syncWatchObj(watch *unstructured.Unstructured) error {
	// If it doesn't match our selector, and it doesn't have our finalizer,
	// ignore it.
	isMatch := mgr.selector.Matches(watch)
	hasFinalizer := dynamicobject.HasFinalizer(watch, mgr.finalizer.Name)
	if !isMatch && !hasFinalizer {
		glog.V(4).Infof(
			"%s: Will not sync watch %s: IsMatch=%t: HasFinalizer=%t",
			mgr, common.DescObjectAsKey(watch), isMatch, hasFinalizer,
		)
		return nil
	}

	glog.V(4).Infof("%s: Will sync watch %s", mgr, common.DescObjectAsKey(watch))

	watchClient, err := mgr.DynamicClientSet.GetClientByKind(
		watch.GetAPIVersion(), watch.GetKind(),
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"%s: Failed to get client for watch %s",
			mgr, common.DescObjectAsKey(watch),
		)
	}

	// Before taking any other action, add our finalizer (if desired).
	// This ensures we have a chance to clean up after any action we later take.
	watchCopy, err := mgr.finalizer.SyncObject(watchClient, watch)
	if err != nil {
		// If we fail to do this, abort before doing anything else and requeue.
		return errors.Wrapf(
			err,
			"%s: Can't sync finalizer for watch %s",
			mgr, common.DescObjectAsKey(watch),
		)
	}
	watch = watchCopy

	// Check the finalizer again in case we just removed it.
	isMatch = mgr.selector.Matches(watch)
	hasFinalizer = dynamicobject.HasFinalizer(watch, mgr.finalizer.Name)
	if !isMatch && !hasFinalizer {
		glog.V(4).Infof(
			"%s: Will not sync watch %s: IsMatch=%t: HasFinalizer=%t",
			mgr, common.DescObjectAsKey(watch), isMatch, hasFinalizer,
		)
		return nil
	}

	// List all attachments related to this parent.
	// This only lists the children we created. Existing children are ignored.
	observedAttachments, err := mgr.getAttachments(watch)
	if err != nil {
		return err
	}

	// Call the sync hook
	syncRequest := &SyncHookRequest{
		Controller:  mgr.Controller,
		Watch:       watch,
		Attachments: observedAttachments,
	}
	syncResult, err := mgr.callSyncHook(syncRequest)
	if err != nil {
		return err
	}

	if glog.V(4) && len(syncResult.Attachments) == 0 {
		glog.Infof("%s: Nil attachments in hook response for watch %s",
			mgr, common.DescObjectAsKey(watch),
		)
	}

	desiredAttachments :=
		common.MakeAnyUnstructRegistryByReference(watch, syncResult.Attachments)

	// Enqueue a delayed resync, if requested.
	if syncResult.ResyncAfterSeconds > 0 {
		mgr.enqueueWatchAfter(
			watch, time.Duration(syncResult.ResyncAfterSeconds*float64(time.Second)),
		)
	}

	// Logic to set desired labels, annotations & status on watch.
	// Also remove finalizer if requested.

	// Make a copy since watch is from the cache.
	watchCopy = watch.DeepCopy()

	finalWatchLabels := watchCopy.GetLabels()
	if finalWatchLabels == nil {
		finalWatchLabels = make(map[string]string)
	}

	finalWatchAnnotations := watchCopy.GetAnnotations()
	if finalWatchAnnotations == nil {
		finalWatchAnnotations = make(map[string]string)
	}

	// initialize the desired status by overriding the
	// sync result's status
	finalWatchStatus := k8s.GetNestedObject(watchCopy.Object, "status")
	if syncResult.Status == nil {
		// A null .status in the sync response means leave it unchanged
		// i.e. use the existing status
		syncResult.Status = finalWatchStatus
	}

	labelsChanged := updateStringMap(finalWatchLabels, syncResult.Labels)
	annotationsChanged := updateStringMap(finalWatchAnnotations, syncResult.Annotations)
	statusChanged := !reflect.DeepEqual(finalWatchStatus, syncResult.Status)

	// Only update the watch if anything changed
	//
	// Updating a watch is done only if its meta information changes
	// i.e. labels, annotations &/or status
	if labelsChanged || annotationsChanged || statusChanged ||
		(syncResult.Finalized && dynamicobject.HasFinalizer(watch, mgr.finalizer.Name)) {

		watchCopy.SetLabels(finalWatchLabels)
		watchCopy.SetAnnotations(finalWatchAnnotations)
		k8s.SetNestedField(watchCopy.Object, syncResult.Status, "status")

		if statusChanged && watchClient.HasSubresource("status") {
			// The regular Update below will ignore changes to .status
			// so we do it separately.
			result, err := watchClient.Namespace(watch.GetNamespace()).
				UpdateStatus(watchCopy, metav1.UpdateOptions{})
			if err != nil {
				return errors.Wrapf(
					err,
					"%s: Failed to update status for watch %s",
					mgr, common.DescObjectAsKey(watch),
				)
			}
			// The Update below needs to use the latest ResourceVersion.
			watchCopy.SetResourceVersion(result.GetResourceVersion())
		}

		// check if its time to remove its finalizer
		if syncResult.Finalized {
			dynamicobject.RemoveFinalizer(watchCopy, mgr.finalizer.Name)
		}

		glog.V(4).Infof("%s: Updating watch %s", mgr, common.DescObjectAsKey(watch))

		_, err = watchClient.
			Namespace(watch.GetNamespace()).Update(watchCopy, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err,
				"%s: Failed to update watch %s", mgr, common.DescObjectAsKey(watch),
			)
		}

		glog.V(4).Infof("%s: Updated watch %s", mgr, common.DescObjectAsKey(watch))
	}

	// Add an generic controller name annotation to all desired attachments
	// to remember/indicate that they are reconciled by this watch resource
	for _, group := range desiredAttachments {
		for _, attachment := range group {
			ann := attachment.GetAnnotations()
			watchKey := common.MakeAnnotationKeyFromObj(watch)

			if ann[watchKey] == "MetaGenericController" {
				continue
			}
			if ann == nil {
				ann = make(map[string]string)
			}
			ann[watchKey] = "MetaGenericController"
			attachment.SetAnnotations(ann)
		}
	}

	// Reconcile attachment objects belonging to this watch.
	//
	// Controller reconciles attachments if
	//
	//	1. the watch is "alive" (not pending deletion), or
	//	2. if watch is pending deletion and controller has a 'finalize' hook
	if watch.GetDeletionTimestamp() == nil || mgr.finalizer.ShouldFinalize(watch) {

		// build a new instance of attachment update strategy manager
		updateStrategyMgr, err := newAttachmentUpdateStrategyManager(
			mgr.ResourceManager,
			mgr.Controller.Spec.Attachments,
		)
		if err != nil {
			return err
		}

		// Reconcile attachments via attachment manager
		attMgr := &common.AttachmentManager{
			AttachmentExecuteBase: common.AttachmentExecuteBase{
				GetChildUpdateStrategyByGK: updateStrategyMgr.GetByGKOrDefault,
				Watch:                      watch,
			},

			DynamicClientSet: mgr.DynamicClientSet,
			Observed:         observedAttachments,
			Desired:          desiredAttachments,
		}
		return attMgr.SetDefaultsIfNil().Apply()
	}

	return nil
}

// getAttachments returns the attachments as declared
// in GenericController resource
//
// TODO (@amitkumardas):
//
// This will possibly become more and more policy oriented.
// IMO this should support a variety of combinations to
// filter each attachment kind. Each attachment kind
// might require its own combination of filters/selectors.
func (mgr *genericControllerManager) getAttachments(
	watch *unstructured.Unstructured,
) (common.AnyUnstructRegistry, error) {

	attachmentRegistry := make(common.AnyUnstructRegistry)

	for _, attachmentKind := range mgr.Controller.Spec.Attachments {
		attachmentInformer := mgr.attachmentInformers.Get(
			attachmentKind.APIVersion, attachmentKind.Resource,
		)
		if attachmentInformer == nil {
			return nil, errors.Errorf(
				"%s: No informer for attachment %q of %q",
				mgr, attachmentKind.Resource, attachmentKind.APIVersion,
			)
		}
		var attachmentObjs []*unstructured.Unstructured
		var err error

		// all possible attachment object for the given attachment kind
		attachmentObjs, err = attachmentInformer.Lister().List(labels.Everything())
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"%s: Can't list attachments for %s of %s",
				mgr, attachmentKind.Resource, attachmentKind.APIVersion,
			)
		}

		if len(attachmentObjs) == 0 {
			glog.V(4).Infof(
				"%s: No attachments found for %s of %s",
				mgr, attachmentKind.Resource, attachmentKind.APIVersion,
			)
		}

		// steps to initialize the attachment registry
		disAttachRes := mgr.ResourceManager.GetByResource(
			attachmentKind.APIVersion, attachmentKind.Resource,
		)
		if disAttachRes == nil {
			return nil, errors.Errorf(
				"%s: Can't find discovered attachment %s of %s",
				mgr, attachmentKind.Resource, attachmentKind.APIVersion,
			)
		}
		attachmentRegistry.InitGroupByVK(attachmentKind.APIVersion, disAttachRes.Kind)

		// TODO
		// @amitkumardas: Need to think more on this !!
		for _, attObj := range attachmentObjs {
			// Do not consider if match fails
			if !mgr.selector.Matches(attObj) {
				glog.V(4).Infof(
					"%s: Ignore attachment %s/%s of kind %s: Selector doesn't match",
					mgr, attObj.GetNamespace(), attObj.GetName(), attObj.GetKind(),
				)
				continue
			}
			attachmentRegistry.InsertByReference(watch, attObj)
		}
	}
	return attachmentRegistry, nil
}

func (mgr *genericControllerManager) callSyncHook(
	request *SyncHookRequest,
) (*SyncHookResponse, error) {

	if mgr.Controller.Spec.Hooks == nil {
		return nil, errors.Errorf("%s: No hooks defined", mgr)
	}

	var response SyncHookResponse

	// First check if we should instead call the finalize hook,
	// which has the same API as the sync hook except that it's
	// called while the object is pending deletion.
	//
	// In addition to finalizing when the object is deleted, we also finalize
	// when the object no longer matches our selector.
	// This allows the controller to clean up after itself if the object has been
	// updated to disable the functionality added by the controller.
	if mgr.Controller.Spec.Hooks.Finalize != nil &&
		(request.Watch.GetDeletionTimestamp() != nil ||
			!mgr.selector.Matches(request.Watch)) {

		// Finalize
		request.Finalizing = true
		err := common.CallHook(mgr.Controller.Spec.Hooks.Finalize, request, &response)
		if err != nil {
			return nil, errors.Wrapf(err, "Finalize hook failed")
		}
	} else {
		// Sync
		request.Finalizing = false
		if mgr.Controller.Spec.Hooks.Sync == nil {
			return nil, errors.Errorf("Sync hook not defined")
		}

		err := common.CallHook(mgr.Controller.Spec.Hooks.Sync, request, &response)
		if err != nil {
			return nil, errors.Wrapf(err, "Sync hook failed")
		}
	}

	return &response, nil
}

// holds update strategies of various resources
type attachmentUpdateStrategies map[string]*v1alpha1.GenericControllerAttachmentUpdateStrategy

// GetOrDefault returns the upgrade strategy based on
// the given api group & kind
func (m attachmentUpdateStrategies) GetOrDefault(
	apiGroup, kind string,
) v1alpha1.ChildUpdateMethod {

	strategy := m.get(apiGroup, kind)
	if strategy == nil || strategy.Method == "" {
		// defaults to ChildUpdateOnDelete strategy
		return v1alpha1.ChildUpdateOnDelete
	}
	return strategy.Method
}

// get returns the controller's attachment's upgrade strategy
// based on the given api group & kind
func (m attachmentUpdateStrategies) get(
	apiGroup, kind string,
) *v1alpha1.GenericControllerAttachmentUpdateStrategy {

	return m[makeAttachmentUpdateStrategyKey(apiGroup, kind)]
}

// makeAttachmentUpdateStrategyKey builds a key suitable to store
// various attachment update strategies. It makes use of apiGroup
// and kind of the resource to build its key.
func makeAttachmentUpdateStrategyKey(apiGroup, kind string) string {
	return fmt.Sprintf("%s.%s", kind, apiGroup)
}

// makeSelector builds selector for watch as well as for all
// attachements declared in GenericController
func makeSelector(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	schema *v1alpha1.GenericController,
) (*Selector, error) {
	// initialize the selector with watch
	var options = []SelectorOption{
		ResourceSelectorOption(resourceMgr, schema.Spec.Watch),
	}

	// add selector options for attachements
	for _, attachment := range schema.Spec.Attachments {
		options = append(
			options,
			ResourceSelectorOption(
				resourceMgr, attachment.GenericControllerResource,
			),
		)
	}
	return NewSelector(options...)
}

// makeUpdateStrategyForAttachments returns the update strategies
// for the attachments declared in the GenericController
func makeUpdateStrategyForAttachments(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	attachments []v1alpha1.GenericControllerAttachment,
) (attachmentUpdateStrategies, error) {
	m := make(attachmentUpdateStrategies)
	for _, attachment := range attachments {
		// no need to store ondelete strategy since
		// its the default anyways
		if attachment.UpdateStrategy != nil &&
			attachment.UpdateStrategy.Method != v1alpha1.ChildUpdateOnDelete {
			// this is done to map resource name to kind name
			resource := resourceMgr.GetByResource(
				attachment.APIVersion, attachment.Resource,
			)
			if resource == nil {
				return nil, errors.Errorf(
					"can't find attachment %q in %v",
					attachment.Resource,
					attachment.APIVersion,
				)
			}
			// Ignore API version.
			apiGroup, _ := common.ParseAPIVersionToGroupVersion(attachment.APIVersion)
			key := makeAttachmentUpdateStrategyKey(apiGroup, resource.Kind)
			m[key] = attachment.UpdateStrategy
		}
	}
	return m, nil
}

// makeWatchQueueKey builds a key suitable to be used to
// queue
func makeWatchQueueKey(obj interface{}) (string, error) {
	switch o := obj.(type) {
	case cache.DeletedFinalStateUnknown:
		return o.Key, nil
	case cache.ExplicitKey:
		return string(o), nil
	case *unstructured.Unstructured:
		return fmt.Sprintf(
			"%s:%s:%s:%s",
			o.GetAPIVersion(), o.GetKind(), o.GetNamespace(), o.GetName(),
		), nil
	default:
		return "", errors.Errorf(
			"Can't make key for %T: Want type *unstructured.Unstructured", obj,
		)
	}
}

// splitWatchQueueKey accepts the reconcile queue key and
// returns its meta information.
func splitWatchQueueKey(key string) (apiVersion, kind, namespace, name string, err error) {
	parts := strings.SplitN(key, ":", 4)
	if len(parts) != 4 {
		return "", "", "", "",
			errors.Errorf(
				"Invalid queue key %q: Expected format apiVersion:kind:ns:name",
				key,
			)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

// updateStringMap executes either an Add, or Update or Delete of
// key value pair against the destination map based on the provided
// updates. It also returns a flag that indicates if there was any
// change.
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
		old, found := dest[k]
		if !found || old != *v {
			dest[k] = *v
			changed = true
		}
	}
	return changed
}
