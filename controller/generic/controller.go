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

// WatchController reconciles the watch specified in GenericController
// custom resource
type WatchController struct {
	// GCtlConfig config / yaml
	GCtlConfig *v1alpha1.GenericController

	// DynamicDiscovery to fetch server API resources
	DynamicDiscovery *dynamicdiscovery.APIResourceDiscovery

	// DynamicClientset to operate against resources managed
	// by this controller instance
	DynamicClientSet *dynamicclientset.Clientset

	// holds all watch API resources declared in this
	// GenericController yaml
	watchAPIRegistry common.ResourceRegistrar

	// selectors to filter watch & attachment resources
	watchSelector      *Selection
	attachmentSelector *Selection

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
	watchInformers      common.ResourceInformerRegistrar
	attachmentInformers common.ResourceInformerRegistrar

	// instance that deals with this controller's finalizer
	// if any
	finalizer *finalizer.Finalizer
}

// String implements Stringer interface
func (mgr *WatchController) String() string {
	if mgr.GCtlConfig == nil {
		return "GenericController"
	}
	return fmt.Sprintf(
		"GenericController %q / %q",
		mgr.GCtlConfig.Namespace,
		mgr.GCtlConfig.Name,
	)
}

// NewWatchController returns a new instance of watch controller
// with required watch & child informers, selectors, update
// strategy & so on.
func NewWatchController(
	dynDiscovery *dynamicdiscovery.APIResourceDiscovery,
	dynClientset *dynamicclientset.Clientset,
	dynInformerFactory *dynamicinformer.SharedInformerFactory,
	config *v1alpha1.GenericController,
) (wCtl *WatchController, newErr error) {

	ctl := &WatchController{
		GCtlConfig:       config,
		DynamicDiscovery: dynDiscovery,
		DynamicClientSet: dynClientset,

		watchAPIRegistry: make(common.ResourceRegistrar),

		watchInformers:      make(common.ResourceInformerRegistrar),
		attachmentInformers: make(common.ResourceInformerRegistrar),

		watchQ: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"WatchGCtl-"+config.Namespace+"-"+config.Name,
		),

		finalizer: &finalizer.Finalizer{
			// Finalizer is entrusted with this finalizer name.
			// This gets applied against the watch s.t GenericController
			// has a chance to handle finalize hook i.e. handle deletion
			// of watch resource
			Name: "protect.gctl.metac.openebs.io/" +
				common.DescMetaAsSanitisedNSName(config.GetObjectMeta()),

			// Enable if Finalize field is set in the generic controller
			Enabled: config.Spec.Hooks.Finalize != nil,
		},
	}

	var err error

	// build watch & attachment selectors
	ctl.watchSelector, ctl.attachmentSelector, err = makeAllSelectors(
		dynDiscovery,
		config,
	)
	if err != nil {
		return nil, err
	}
	watchAPI := dynDiscovery.GetAPIForAPIVersionAndResource(
		config.Spec.Watch.APIVersion,
		config.Spec.Watch.Resource,
	)
	if watchAPI == nil {
		return nil,
			errors.Errorf(
				"Discovery failed: Can't find watch %q with version %q: %s",
				config.Spec.Watch.Resource,
				config.Spec.Watch.APIVersion,
				ctl,
			)
	}
	// add watch server resource _i.e. API resource_ to registry
	//
	// NOTE:
	// We use a registry even though there is a single watch
	// we might remove this registry if we believe single
	// watch is good & sufficient in GenericController
	ctl.watchAPIRegistry.Set(
		watchAPI.Group,
		watchAPI.Kind,
		watchAPI,
	)
	// Remember the update strategy for each attachment type.
	ctl.updateStrategies, err = makeUpdateStrategyForAttachments(
		dynDiscovery,
		config.Spec.Attachments,
	)
	if err != nil {
		return nil, err
	}
	// close the successfully created informers in-case of any
	// errors during initialization
	defer func() {
		if newErr != nil {
			for _, informer := range ctl.attachmentInformers {
				informer.Close()
			}
			for _, informer := range ctl.watchInformers {
				informer.Close()
			}
		}
	}()
	// init watch informer
	informer, err := dynInformerFactory.GetOrCreate(
		config.Spec.Watch.APIVersion,
		config.Spec.Watch.Resource,
	)
	if err != nil {
		return nil,
			errors.Wrapf(
				err,
				"Can't create informer for watch %q with version %q: %s",
				config.Spec.Watch.Resource,
				config.Spec.Watch.APIVersion,
				ctl,
			)
	}
	// add watch informer to informer registry
	//
	// NOTE:
	// 	This is a registry of watch informers even though GenericController
	// supports only a single watch. This may be removed to a single informer
	// if we conclude that single watch is best for GenericController.
	ctl.watchInformers.Set(
		config.Spec.Watch.APIVersion,
		config.Spec.Watch.Resource,
		informer,
	)
	// initialise the informers for attachments
	for _, a := range config.Spec.Attachments {
		informer, err := dynInformerFactory.GetOrCreate(
			a.APIVersion,
			a.Resource,
		)
		if err != nil {
			return nil,
				errors.Wrapf(
					err,
					"Can't create informer for attachment %q with version %q: %s",
					a.Resource,
					a.APIVersion,
					ctl,
				)
		}
		// add attachment informer to informer registry
		ctl.attachmentInformers.Set(
			a.APIVersion,
			a.Resource,
			informer,
		)
	}
	return ctl, nil
}

// Start starts the generic controller based on its fields
// that were initialised earlier (mostly via its constructor)
func (mgr *WatchController) Start(workerCount int) {
	// init the channels with empty structs
	mgr.stopCh = make(chan struct{})
	mgr.doneCh = make(chan struct{})

	// set event handlers. GenericControllers can be created at any time,
	// so we have to assume the shared informers are already running. We can't
	// add event handlers in NewWatchController() since c might be incomplete.
	watchHandlers := cache.ResourceEventHandlerFuncs{
		AddFunc:    mgr.enqueueWatch,
		UpdateFunc: mgr.updateWatch,
		DeleteFunc: mgr.enqueueWatch,
	}
	var resyncPeriod time.Duration
	if mgr.GCtlConfig.Spec.ResyncPeriodSeconds != nil {
		// Use a custom resync period if requested
		// NOTE: This only applies to the parent
		resyncPeriod =
			time.Duration(*mgr.GCtlConfig.Spec.ResyncPeriodSeconds) * time.Second
		// Put a reasonable limit on it.
		if resyncPeriod < time.Second {
			resyncPeriod = time.Second
		}
	}
	for _, informer := range mgr.watchInformers {
		if resyncPeriod != 0 {
			informer.Informer().AddEventHandlerWithResyncPeriod(
				watchHandlers,
				resyncPeriod,
			)
		} else {
			informer.Informer().AddEventHandler(watchHandlers)
		}
	}
	if workerCount <= 0 {
		// set a reasonable worker count value
		workerCount = 5
	}
	go func() {
		// close done channel i.e. mark closure of this start invocation
		defer close(mgr.doneCh)
		// provide the ability to run operations after panics
		defer utilruntime.HandleCrash()

		glog.Infof("Starting %s", mgr)
		defer glog.Infof("Shutting down %s", mgr)

		// Wait for dynamic client and all informers.
		glog.V(7).Infof("Waiting for caches to sync: %s", mgr)
		syncFuncs := make(
			[]cache.InformerSynced,
			0,
			1+1+len(mgr.GCtlConfig.Spec.Attachments),
		)
		for _, informer := range mgr.watchInformers {
			syncFuncs = append(syncFuncs, informer.Informer().HasSynced)
		}
		for _, informer := range mgr.attachmentInformers {
			syncFuncs = append(syncFuncs, informer.Informer().HasSynced)
		}
		if !k8s.WaitForCacheSync(
			mgr.GCtlConfig.AsNamespaceNameKey(),
			mgr.stopCh,
			syncFuncs...,
		) {
			// We wait forever unless Stop() is called, so this isn't an error.
			glog.Warningf("Cache sync never finished: %s", mgr)
			return
		}
		glog.V(5).Infof("Starting %d workers: %s", workerCount, mgr)
		var wg sync.WaitGroup
		for i := 0; i < workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				wait.Until(mgr.worker, time.Second, mgr.stopCh)
			}()
		}
		wg.Wait()
	}()
}

// Stop will stop this controller
func (mgr *WatchController) Stop() {
	// closing stopCh will unblock all the logics where this
	// channel was passed earlier. This triggers closing of
	// doneCh as well
	close(mgr.stopCh)
	mgr.watchQ.ShutDown()

	// this will block till doneCh is closed.
	//
	// NOTE:
	//	doneCh will be closed after all the workers are
	// stopped via above close(c.stopCh) invocation
	<-mgr.doneCh

	// Remove event handlers and close informers for all attachment
	// resources.
	for _, attInformer := range mgr.attachmentInformers {
		attInformer.Informer().RemoveEventHandlers()
		attInformer.Close()
	}
	// Remove event handlers and close informer for all watched
	// resources.
	for _, watchInformer := range mgr.watchInformers {
		watchInformer.Informer().RemoveEventHandlers()
		watchInformer.Close()
	}
}

// worker works for ever. Its only work is to process the
// workitem i.e. the watch
func (mgr *WatchController) worker() {
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
func (mgr *WatchController) processNextWorkItem() bool {
	// queue will give us the next item (parent resource in this case)
	// to be reconciled unless shutdown was invoked against this queue
	key, quit := mgr.watchQ.Get()
	if quit {
		return false
	}
	defer mgr.watchQ.Done(key)

	// actual reconcile logic is invoked here
	err := mgr.syncWatch(key.(string))
	if err != nil {
		utilruntime.HandleError(
			errors.Wrapf(
				err,
				"Failed to sync %q: %s",
				key,
				mgr,
			),
		)
		mgr.watchQ.AddRateLimited(key)
		return true
	}
	// reconcile was successful
	mgr.watchQ.Forget(key)
	return true
}

// enqueueWatch as the name suggests enqueues the eligible watch
// resource to be reconciled during dequeue.
//
// In other words, if the given watch resource is eligible it will be
// added to this controller queue to be extracted later & reconciled.
func (mgr *WatchController) enqueueWatch(obj interface{}) {
	// If the watched doesn't match our selector,
	// and it doesn't have our finalizer, we don't care about it.
	//
	// In other words, if resource has this controller's finalizer
	// or matches this controller's selectors, then this resource
	// belongs to this controller & hence should be queued
	if watchObj, ok := obj.(*unstructured.Unstructured); ok {
		isMatch, err := mgr.watchSelector.MatchLAN(watchObj)
		if err != nil {
			glog.Errorf(
				"Can't match watch %s: %s: %+v",
				common.DescObjectAsKey(watchObj),
				mgr,
				err,
			)
			return
		}
		hasFinalizer := dynamicobject.HasFinalizer(watchObj, mgr.finalizer.Name)
		if !isMatch && !hasFinalizer {
			glog.V(6).Infof(
				"Will not enqueue watch %s: IsMatch=%t: HasFinalizer=%t: %s",
				common.DescObjectAsKey(watchObj),
				isMatch,
				hasFinalizer,
				mgr,
			)
			return
		}
	}

	key, err := makeWatchQueueKey(obj)
	if err != nil {
		utilruntime.HandleError(
			errors.Wrapf(
				err,
				"Can't make queue key: Watch %+v: %s",
				obj,
				mgr,
			),
		)
		return
	}
	glog.V(7).Infof(
		"Will enqueue %s: %s",
		key,
		mgr,
	)
	mgr.watchQ.Add(key)
}

func (mgr *WatchController) enqueueWatchAfter(obj interface{}, delay time.Duration) {
	key, err := makeWatchQueueKey(obj)
	if err != nil {
		utilruntime.HandleError(
			errors.Wrapf(
				err,
				"Can't make queue key: Watch %+v: %s",
				obj,
				mgr,
			),
		)
		return
	}
	mgr.watchQ.AddAfter(key, delay)
}

// updateWatch enqueues the watch object without any checks
func (mgr *WatchController) updateWatch(old, cur interface{}) {
	mgr.enqueueWatch(cur)
}

// syncWatch reconciles the watch resource represented by this provided
// key
//
// NOTE:
//	Errors are logged as debug messages since errors may auto correct
// eventually
//
// TODO (@amitkumardas):
// - Unit Tests
func (mgr *WatchController) syncWatch(key string) error {
	var err error
	defer func() {
		if err != nil {
			glog.Warningf("Can't sync: %s", err.Error())
			return
		}
		glog.V(7).Infof("Sync completed for watch %s: %s", key, mgr)
	}()

	apiVersion, kind, namespace, name, err := splitWatchQueueKey(key)
	if err != nil {
		return err
	}
	watchAPI := mgr.DynamicDiscovery.GetAPIForAPIVersionAndKind(
		apiVersion,
		kind,
	)
	if watchAPI == nil {
		return errors.Errorf(
			"Can't discover watch API: Kind %q: Version %q: %s",
			kind,
			apiVersion,
			mgr,
		)
	}
	watchInformer := mgr.watchInformers.Get(apiVersion, watchAPI.Name)
	if watchInformer == nil {
		return errors.Errorf(
			"Can't find informer for watch API %q: Kind %q: Version %q: %s",
			watchAPI.Name,
			kind,
			apiVersion,
			mgr,
		)
	}
	watchObj, err := watchInformer.Lister().Get(namespace, name)
	if apierrors.IsNotFound(err) {
		// swallow **not found** error since there's no point retrying
		// if the watch is deleted from cluster
		glog.V(7).Infof(
			"Can't find watch %q / %q:Kind %q: Version %q: %s: %+v",
			namespace,
			name,
			kind,
			apiVersion,
			mgr,
			err,
		)
		return nil
	}
	// other errors are genuine errors
	if err != nil {
		return err
	}
	// remember we use a defer statement to intercept error as
	// warning log.
	// Hence, we dont return below invocation directly.
	err = mgr.syncWatchObj(watchObj)
	return err
}

// syncWatchObj reconciles the state based on this observed
// watch resource instance and other configurations specified
// in the GenericController
//
// TODO (@amitkumardas):
// - Unit Tests
func (mgr *WatchController) syncWatchObj(watch *unstructured.Unstructured) error {
	// if watch doesn't match the configured selector, and doesn't have
	// our finalizer, then **ignore it**.
	isMatch, err := mgr.watchSelector.MatchLAN(watch)
	if err != nil {
		return errors.Wrapf(
			err,
			"Match failed for watch %s: %s",
			common.DescObjectAsKey(watch),
			mgr,
		)
	}
	hasFinalizer := dynamicobject.HasFinalizer(
		watch,
		mgr.finalizer.Name,
	)
	// if no match && no finalizer
	if !isMatch && !hasFinalizer {
		glog.V(6).Infof(
			"Won't sync watch %s: IsMatch=%t: HasFinalizer=%t: %s",
			common.DescObjectAsKey(watch),
			isMatch,
			hasFinalizer,
			mgr,
		)
		return nil
	}

	glog.V(7).Infof(
		"Will sync watch %s: %s",
		common.DescObjectAsKey(watch),
		mgr,
	)

	watchClient, err := mgr.DynamicClientSet.GetClientForAPIVersionAndKind(
		watch.GetAPIVersion(),
		watch.GetKind(),
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"Failed to get client for watch %s: %s",
			common.DescObjectAsKey(watch),
			mgr,
		)
	}

	// Add or Remove our finalizer **if desired**.
	// This ensures we have a chance to clean up after any action we later take.
	watchCopy, err := mgr.finalizer.SyncObject(watchClient, watch)
	if err != nil {
		return errors.Wrapf(
			err,
			"Can't sync finalizer for watch %s: %s",
			common.DescObjectAsKey(watch),
			mgr,
		)
	}
	watch = watchCopy

	// Check the finalizer again in case we just removed it.
	isMatch, err = mgr.watchSelector.MatchLAN(watch)
	if err != nil {
		return errors.Wrapf(
			err,
			"Match failed for watch %s: %s",
			common.DescObjectAsKey(watch),
			mgr,
		)
	}
	hasFinalizer = dynamicobject.HasFinalizer(watch, mgr.finalizer.Name)
	if !isMatch && !hasFinalizer {
		glog.V(6).Infof(
			"Won't sync watch %s: IsMatch=%t: HasFinalizer=%t: %s",
			common.DescObjectAsKey(watch),
			isMatch,
			hasFinalizer,
			mgr,
		)
		return nil
	}

	// List all attachments related to this watch
	observedAttachments, err := mgr.getObservedAttachments(watch)
	if err != nil {
		return err
	}
	// Call the sync hook since we have the watch as well as
	// required attachments
	syncRequest := &SyncHookRequest{
		Controller:  mgr.GCtlConfig,
		Watch:       watch,
		Attachments: observedAttachments,
	}
	syncResponse, err := mgr.callSyncHook(syncRequest)
	if err != nil {
		return err
	}
	if syncResponse == nil {
		glog.V(6).Infof(
			"Will ignore sync: Received nil hook response: Watch %s: %s",
			common.DescObjectAsKey(watch),
			mgr,
		)
		// nothing to do; hence return
		//
		// one of the scenarios this can happen is when
		// only finalize hook is set and time to finalize
		// has not yet come
		return nil
	}

	// Enqueue a delayed resync, if requested.
	if syncResponse.ResyncAfterSeconds > 0 {
		mgr.enqueueWatchAfter(
			watch,
			time.Duration(syncResponse.ResyncAfterSeconds*float64(time.Second)),
		)
	}

	// build various attachments _(received from the sync hook call)_
	// in a registry format
	desiredAttachments := common.MakeAnyUnstructRegistry(
		syncResponse.Attachments,
	)
	explicitUpdates := common.MakeAnyUnstructRegistry(
		syncResponse.ExplicitUpdates,
	)
	explicitDeletes := common.MakeAnyUnstructRegistry(
		syncResponse.ExplicitDeletes,
	)

	// Logic to set desired labels, annotations & status on watch.
	// Also remove finalizer if requested.
	// Make a copy of watch since it is from the cache.
	watchCopy = watch.DeepCopy()
	// get the original watch's labels
	finalWatchLabels := watchCopy.GetLabels()
	if finalWatchLabels == nil {
		finalWatchLabels = make(map[string]string)
	}
	// get the original watch's annotations
	finalWatchAnnotations := watchCopy.GetAnnotations()
	if finalWatchAnnotations == nil {
		finalWatchAnnotations = make(map[string]string)
	}
	// get the original watch's status
	finalWatchStatus := k8s.GetNestedObject(
		watchCopy.Object,
		"status",
	)
	if syncResponse.Status == nil {
		// A null .status in the sync response means leave it unchanged
		// i.e. use the existing status
		syncResponse.Status = finalWatchStatus
	}
	glog.V(6).Infof(
		"Desired labels=[%v], annotations=[%v], status=[%v]: Watch %s: %s",
		syncResponse.Labels,
		syncResponse.Annotations,
		syncResponse.Status,
		common.DescObjectAsKey(watch),
		mgr,
	)
	labelsChanged := updateStringMap(
		finalWatchLabels,
		syncResponse.Labels,
	)
	annotationsChanged := updateStringMap(
		finalWatchAnnotations,
		syncResponse.Annotations,
	)
	statusChanged := !reflect.DeepEqual(finalWatchStatus, syncResponse.Status)
	glog.V(5).Infof(
		"Is watch change? labels=%t annotations=%t status=%t: Watch %s: %s",
		labelsChanged,
		annotationsChanged,
		statusChanged,
		common.DescObjectAsKey(watch),
		mgr,
	)
	// update the watch only if anything changed
	//
	// Update a watch if any of its following metadata changes:
	// - labels,
	// - annotations,
	// - status,
	// - finalizers
	if labelsChanged ||
		annotationsChanged ||
		statusChanged ||
		(syncResponse.Finalized && dynamicobject.HasFinalizer(watch, mgr.finalizer.Name)) {
		// set these metadata with updated values
		watchCopy.SetLabels(finalWatchLabels)
		watchCopy.SetAnnotations(finalWatchAnnotations)
		k8s.SetNestedField(
			watchCopy.Object,
			syncResponse.Status,
			"status",
		)
		// check if watch resource has a subresource
		hasSubResourceStatus := watchClient.HasSubresource("status")
		glog.V(7).Infof(
			"Watch %s has status as subresource=%t: %s",
			common.DescObjectAsKey(watch),
			hasSubResourceStatus,
			mgr,
		)
		if statusChanged && hasSubResourceStatus {
			// NOTE:
			// 	regular update below will **ignore** changes to **.status**
			// so we do it separately
			result, err :=
				watchClient.
					Namespace(watch.GetNamespace()).
					UpdateStatus(
						watchCopy,
						metav1.UpdateOptions{},
					)
			if err != nil {
				return errors.Wrapf(
					err,
					"Failed to update status for watch %s: %s",
					common.DescObjectAsKey(watch),
					mgr,
				)
			}
			// to proceed with next update due to metadata related changes
			// it needs to use the latest ResourceVersion from this status
			// update
			watchCopy.SetResourceVersion(result.GetResourceVersion())
		}
		// check if its time to remove its finalizer
		if syncResponse.Finalized {
			mgr.finalizer.RemoveFinalizer(watchCopy)
		}
		glog.V(7).Infof(
			"Updating watch %s: %s",
			common.DescObjectAsKey(watch),
			mgr,
		)
		// this update is meant to work for updating metadata
		_, err = watchClient.
			Namespace(watch.GetNamespace()).
			Update(
				watchCopy,
				metav1.UpdateOptions{},
			)
		if err != nil {
			return errors.Wrapf(
				err,
				"Failed to update watch %s: %s",
				common.DescObjectAsKey(watch),
				mgr,
			)
		}
		glog.V(7).Infof(
			"Updated watch %s: %s",
			common.DescObjectAsKey(watch),
			mgr,
		)
	}
	// Check if desired attachments should be reconciled? There will
	// be cases when we do not want to reconcile the attachments.
	//
	// NOTE:
	// 	SkipReconcile looks similar to GenericController's spec.ReadOnly.
	// However, both of them serve different purposes. SkipReconcile is
	// dynamic and is set by the sync hook implementation. However ReadOnly
	// is a static option that is set in GenericController's spec.
	//
	//	In other words, one expects SkipReconcile to vary from true to false
	// & back to true depending on runtime conditions. On the other hand,
	// ReadOnly is expected to be set to true or false for the entire
	// lifecycle of the controller.
	if syncResponse.SkipReconcile {
		// should skip reconciling attachments
		glog.V(7).Infof(
			"Won't update attachments: SkipReconcile %t: Watch %s: %s",
			syncResponse.SkipReconcile,
			common.DescObjectAsKey(watch),
			mgr,
		)
		return nil
	}
	// Additional checks from generic controller specs
	// If create/delete/update are supported for attachments?
	readOnly := false
	if mgr.GCtlConfig.Spec.ReadOnly != nil {
		readOnly = *mgr.GCtlConfig.Spec.ReadOnly
	}
	if readOnly {
		// this controller instance is only meant for watch related changes
		glog.V(7).Infof(
			"Won't update attachments: ReadOnly %t: Watch %s: %s",
			readOnly,
			common.DescObjectAsKey(watch),
			mgr,
		)
		return nil
	}
	// Reconcile attachments mapped to this watch based on
	// condition(s)
	if !mgr.isReconcileAttachments(watch, syncRequest, syncResponse) {
		glog.V(7).Infof(
			"Won't update attachments: IsReconcileAttachments false: Watch %s: %s",
			common.DescObjectAsKey(watch),
			mgr,
		)
		return nil
	}

	// build a new instance of attachment update strategy
	updateStrategyMgr, err := newAttachmentUpdateStrategyManager(
		mgr.DynamicDiscovery,
		mgr.GCtlConfig.Spec.Attachments,
	)
	if err != nil {
		return err
	}
	glog.V(8).Infof(
		"Will apply attachments: Observed vs. Desired:\n- %s\n- %s\n- %s",
		observedAttachments,
		desiredAttachments,
		mgr,
	)
	// Reconcile attachments via attachment manager
	clusterStatesCtrl := &common.ClusterStatesController{
		ClusterStatesControllerBase: common.ClusterStatesControllerBase{
			GetChildUpdateStrategyByGK: updateStrategyMgr.GetStrategyByGKOrDefault,
			IsPatchByGK:                updateStrategyMgr.IsPatchByGK,
			Watch:                      watch,
			UpdateAny:                  mgr.GCtlConfig.Spec.UpdateAny,
			DeleteAny:                  mgr.GCtlConfig.Spec.DeleteAny,
			// TODO (@amitkumardas):
			//
			// Need to decide if this field should be part of
			// GenericController specs like UpdateAny & DeleteAny?
			//
			// This is currently set to true if this request is being
			// processed by finalize hook. In other words, this is set
			// to true during finalize hook invocation.
			UpdateDuringPendingDelete: k8s.BoolPtr(syncRequest.Finalizing),
		},
		DynamicClientSet: mgr.DynamicClientSet,
		Observed:         observedAttachments,
		Desired:          desiredAttachments,
		ExplicitUpdates:  explicitUpdates,
		ExplicitDeletes:  explicitDeletes,
	}
	return clusterStatesCtrl.Apply()
}

// isReconcileAttachments returns true if controller should
// reconcile attachments. It returns true if either of the
// following conditions succeed:
//
//	1. watch is "alive" (i.e. not pending deletion),
//
//	2. if watch is pending deletion &&
//		watch is set with this controller's finalizer,
//
//  3. if watch is pending deletion &&
//		request.Finalizing is true &&
//		response.Finalized is false
//
//	Condition '3' should take care of cases when the watch
// was deleted before GenericController was deployed. This
// watch is probably stuck in its deletion
func (mgr *WatchController) isReconcileAttachments(
	watch *unstructured.Unstructured,
	syncRequest *SyncHookRequest,
	syncResponse *SyncHookResponse,
) bool {
	if watch.GetDeletionTimestamp() == nil || // condition 1
		mgr.finalizer.ShouldFinalize(watch) || // condition 2
		(watch.GetDeletionTimestamp() != nil && // condition 3
			syncRequest.Finalizing &&
			!syncResponse.Finalized) {
		return true
	}
	return false
}

// getObservedAttachments returns the attachments as declared
// in GenericController resource
//
// TODO (@amitkumardas):
// - Unit Tests
func (mgr *WatchController) getObservedAttachments(
	watch *unstructured.Unstructured,
) (common.AnyUnstructRegistry, error) {
	// initialize the attachment registry
	attachmentRegistry := make(common.AnyUnstructRegistry)
	for _, attachmentKind := range mgr.GCtlConfig.Spec.Attachments {
		attachmentInformer := mgr.attachmentInformers.Get(
			attachmentKind.APIVersion,
			attachmentKind.Resource,
		)
		if attachmentInformer == nil {
			return nil,
				errors.Errorf(
					"Can't find attachment informer for %q with version %q: Watch %s: %s",
					attachmentKind.Resource,
					attachmentKind.APIVersion,
					common.DescObjectAsKey(watch),
					mgr,
				)
		}
		var attachmentObjs []*unstructured.Unstructured
		var err error
		// all possible attachment object for the given attachment kind
		attachmentObjs, err =
			attachmentInformer.Lister().List(labels.Everything())
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"Can't list attachments for %s with version %s: Watch %s: %s",
				attachmentKind.Resource,
				attachmentKind.APIVersion,
				common.DescObjectAsKey(watch),
				mgr,
			)
		}
		glog.V(7).Infof(
			"%d attachment %s(s) listed with version %s: Watch %s: %s",
			len(attachmentObjs),
			attachmentKind.Resource,
			attachmentKind.APIVersion,
			common.DescObjectAsKey(watch),
			mgr,
		)
		// steps to initialize the attachment registry

		attachmentAPI := mgr.DynamicDiscovery.GetAPIForAPIVersionAndResource(
			attachmentKind.APIVersion,
			attachmentKind.Resource,
		)
		if attachmentAPI == nil {
			if glog.V(5) {
				glog.Warningf(
					"Can't discover attachment api %s with version %s: Watch %s: %s",
					attachmentKind.Resource,
					attachmentKind.APIVersion,
					common.DescObjectAsKey(watch),
					mgr,
				)
			}
			continue
		}
		// initialise this registry with this particular attachment resource
		attachmentRegistry.Init(
			attachmentKind.APIVersion,
			attachmentAPI.Kind,
		)
		for _, attObj := range attachmentObjs {
			isMatch, err :=
				mgr.attachmentSelector.MatchAttachmentAgainstWatch(
					attObj,
					watch,
				)
			if err != nil {
				return nil, errors.Wrapf(
					err,
					"Match failed for attachment %s against watch %s: %s",
					common.DescObjectAsKey(attObj),
					common.DescObjectAsKey(watch),
					mgr,
				)
			}
			if !isMatch {
				glog.V(7).Infof(
					"Selector doesn't match: Ignore attachment %s for watch %s: %s",
					common.DescObjectAsKey(attObj),
					common.DescObjectAsKey(watch),
					mgr,
				)
				// Do not consider this attachment if it is not meant
				// to be
				continue
			}
			attachmentRegistry.Insert(attObj)
		}
	}
	return attachmentRegistry, nil
}

func (mgr *WatchController) callSyncHook(
	request *SyncHookRequest,
) (*SyncHookResponse, error) {
	// validate the hook specifications
	if mgr.GCtlConfig.Spec.Hooks == nil ||
		(mgr.GCtlConfig.Spec.Hooks.Finalize == nil &&
			mgr.GCtlConfig.Spec.Hooks.Sync == nil) {
		return nil,
			errors.Errorf(
				"Invalid spec: Missing hooks: %s",
				mgr,
			)
	}
	var response SyncHookResponse
	// run a match against the watch
	isMatch, err := mgr.watchSelector.MatchLAN(request.Watch)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"Match failed for watch %s: %s",
			common.DescObjectAsKey(request.Watch),
			mgr,
		)
	}
	// First check if we should instead call the finalize hook,
	// which has the same API as the sync hook except that it's
	// called while the object is pending deletion.
	//
	// In addition to finalizing when the object is deleted, we also
	// **finalize when the object no longer matches our selector**.
	// This allows the controller to clean up after itself if the
	// watch has been updated to disable the functionality added by
	// the controller during sync.
	if mgr.GCtlConfig.Spec.Hooks.Finalize != nil &&
		(request.Watch.GetDeletionTimestamp() != nil || !isMatch) {
		// this is about executing finalize hook
		glog.V(7).Infof(
			"Invoking finalize hook for watch %s: %s",
			common.DescObjectAsKey(request.Watch),
			mgr,
		)
		// set finalizing to true since this is finalize hook invocation
		request.Finalizing = true
		hi := &HookInvoker{
			Schema: mgr.GCtlConfig.Spec.Hooks.Finalize,
		}
		err := hi.Invoke(request, &response)
		if err != nil {
			return nil, errors.Wrapf(err, "Finalize hook failed")
		}
		glog.V(7).Infof(
			"Finalize hook completed for watch %s: %s",
			common.DescObjectAsKey(request.Watch),
			mgr,
		)
	} else {
		// this is about executing sync hook
		if mgr.GCtlConfig.Spec.Hooks.Sync == nil {
			glog.V(7).Infof(
				"Skipping sync for watch %s: Sync hook is not configured: %s",
				common.DescObjectAsKey(request.Watch),
				mgr,
			)
			return nil, nil
		}
		glog.V(7).Infof(
			"Invoking sync hook for watch %s: %s",
			common.DescObjectAsKey(request.Watch),
			mgr,
		)
		// set finalizing to false since this is sync hook invocation
		request.Finalizing = false
		hi := &HookInvoker{
			Schema: mgr.GCtlConfig.Spec.Hooks.Sync,
		}
		err := hi.Invoke(request, &response)
		if err != nil {
			return nil, errors.Wrapf(err, "Sync hook failed")
		}
		glog.V(7).Infof(
			"Sync hook completed for watch %s: %s",
			common.DescObjectAsKey(request.Watch),
			mgr,
		)
	}
	return &response, nil
}

// holds update strategies of various resources
type attachmentUpdateStrategies map[string]*v1alpha1.GenericControllerAttachmentUpdateStrategy

// GetOrDefault returns the upgrade strategy based on
// the provided api group & kind or returns the default
// strategy if nothing is found
func (m attachmentUpdateStrategies) GetOrDefault(
	apiGroup string,
	kind string,
) v1alpha1.ChildUpdateMethod {
	// get the strategy
	strategy := m.get(apiGroup, kind)
	if strategy == nil || strategy.Method == "" {
		// defaults to ChildUpdateOnDelete strategy
		return v1alpha1.ChildUpdateOnDelete
	}
	return strategy.Method
}

// get returns the attachment's upgrade strategy
// based on the provided api group & kind
func (m attachmentUpdateStrategies) get(
	apiGroup string,
	kind string,
) *v1alpha1.GenericControllerAttachmentUpdateStrategy {
	return m[makeAttachmentUpdateStrategyKey(apiGroup, kind)]
}

// makeAttachmentUpdateStrategyKey builds a key suitable to store
// various attachment update strategies. It makes use of apiGroup
// and kind of the resource to build its key.
func makeAttachmentUpdateStrategyKey(apiGroup, kind string) string {
	return fmt.Sprintf("%s.%s", kind, apiGroup)
}

// makeAllSelectors builds selector for watch as well as for all
// attachments declared in GenericController
func makeAllSelectors(
	resourceMgr *dynamicdiscovery.APIResourceDiscovery,
	schema *v1alpha1.GenericController,
) (watchSelector, attachmentSelector *Selection, err error) {
	// selector for watch
	watchSelector, err = NewSelectorForWatch(
		resourceMgr,
		schema.Spec.Watch,
	)
	if err != nil {
		return nil, nil, err
	}
	// one selector for all attachments
	attachmentSelector, err = NewSelectorForAttachments(
		resourceMgr,
		schema.Spec.Attachments,
	)
	if err != nil {
		return nil, nil, err
	}
	return watchSelector, attachmentSelector, nil
}

// makeUpdateStrategyForAttachments returns the update strategies
// for the attachments declared in the GenericController
func makeUpdateStrategyForAttachments(
	resourceMgr *dynamicdiscovery.APIResourceDiscovery,
	attachments []v1alpha1.GenericControllerAttachment,
) (attachmentUpdateStrategies, error) {
	m := make(attachmentUpdateStrategies)
	for _, attachment := range attachments {
		// no need to store ondelete strategy since
		// its the default anyways
		if attachment.UpdateStrategy != nil &&
			attachment.UpdateStrategy.Method != v1alpha1.ChildUpdateOnDelete {
			// get the resource
			resource := resourceMgr.GetAPIForAPIVersionAndResource(
				attachment.APIVersion,
				attachment.Resource,
			)
			if resource == nil {
				return nil, errors.Errorf(
					"Can't find attachment %q with version %q",
					attachment.Resource,
					attachment.APIVersion,
				)
			}
			apiGroup, _ := common.ParseAPIVersionToGroupVersion(attachment.APIVersion)
			// build the key for this attachment
			key := makeAttachmentUpdateStrategyKey(apiGroup, resource.Kind)
			// set the update strategy that was specified for this attachment
			m[key] = attachment.UpdateStrategy
		}
	}
	return m, nil
}

// makeWatchQueueKey builds a key suitable to queue a watch object
func makeWatchQueueKey(watch interface{}) (string, error) {
	switch o := watch.(type) {
	case cache.DeletedFinalStateUnknown:
		return o.Key, nil
	case cache.ExplicitKey:
		return string(o), nil
	case *unstructured.Unstructured:
		return fmt.Sprintf(
			"%s:%s:%s:%s",
			o.GetAPIVersion(),
			o.GetKind(),
			o.GetNamespace(),
			o.GetName(),
		), nil
	default:
		return "", errors.Errorf(
			"Can't make key for watch with type %T",
			watch,
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
				"Invalid queue key %q: Want in format apiVersion:kind:ns:name",
				key,
			)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

// updateStringMap executes either an Add, or Update or Delete of
// key value pair against the **destination** map based on the
// provided updates. It also returns a flag that indicates if there
// was any change.
func updateStringMap(dest map[string]string, updates map[string]*string) (changed bool) {
	if dest == nil {
		// continue if dest is nil
		//
		// NOTE:
		//	adding values to dest will not persist across
		// this function
		return changed
	}
	for k, v := range updates {
		if v == nil {
			// nil/null means **delete** the key
			if _, exists := dest[k]; exists {
				delete(dest, k)
				changed = true
			}
			continue
		}
		// add/update the key.
		old, found := dest[k]
		if !found || old != *v {
			dest[k] = *v
			changed = true
		}
	}
	return changed
}
