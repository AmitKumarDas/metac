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
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	metainformers "openebs.io/metac/client/generated/informers/externalversions"
	metalisters "openebs.io/metac/client/generated/listers/metacontroller/v1alpha1"
	"openebs.io/metac/config"
	"openebs.io/metac/controller/common"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	dynamicinformer "openebs.io/metac/dynamic/informer"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// BaseMetaController abstracts Kubernetes informers and listers
// to execute reconcile logic declared in various GenericController
// resources.
//
// NOTE:
//	This structure acts as a base struct to specific ones e.g.
// ConfigMetaController & CRDMetaController
type BaseMetaController struct {
	ResourceManager    *dynamicdiscovery.APIResourceManager
	DynClientset       *dynamicclientset.Clientset
	DynInformerFactory *dynamicinformer.SharedInformerFactory

	WatchControllers map[string]*WatchController
	WorkerCount      int

	doneCh chan struct{}
}

// ConfigMetaController represents a MetaController that
// is based on config files that. This config schema is based
// on GenericController api. Configs are provided to this binary
// at some configured location.
type ConfigMetaController struct {
	BaseMetaController

	// Path from which metac configs will be loaded
	ConfigPath string

	// Function that fetches all generic controller instances
	// required to run Metac
	//
	// NOTE:
	//	One can use either ConfigPath or this function. ConfigPath
	// option has higher priority.
	ConfigLoadFn func() ([]*v1alpha1.GenericController, error)

	// Config instances of type GenericController required to run
	// generic meta controllers. In other words these are the
	// configurations to manage (start, stop) specific watch
	// controllers
	Configs []*v1alpha1.GenericController

	// Total timeout for any condition to succeed.
	//
	// NOTE:
	//	This is currently used to load config that is required
	// to run Metac.
	WaitTimeoutForCondition time.Duration

	// Interval between retries for any condition to succeed.
	//
	// NOTE:
	// 	This is currently used to load config that is required
	// to run Metac
	WaitIntervalForCondition time.Duration

	opts []ConfigMetaControllerOption
	err  error
}

// ConfigMetaControllerOption is a functional option to
// mutate ConfigBasedMetaController instance
//
// This follows **functional options** pattern
type ConfigMetaControllerOption func(*ConfigMetaController) error

// SetMetaControllerConfigLoadFn sets the config loader function
func SetMetaControllerConfigLoadFn(
	fn func() ([]*v1alpha1.GenericController, error),
) ConfigMetaControllerOption {
	return func(c *ConfigMetaController) error {
		c.ConfigLoadFn = fn
		return nil
	}
}

// SetMetaControllerConfigPath sets the config path
func SetMetaControllerConfigPath(path string) ConfigMetaControllerOption {
	return func(c *ConfigMetaController) error {
		c.ConfigPath = path
		return nil
	}
}

// NewConfigMetaController returns a new instance of ConfigMetaController
func NewConfigMetaController(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	dynClientset *dynamicclientset.Clientset,
	dynInformerFactory *dynamicinformer.SharedInformerFactory,
	workerCount int,
	opts ...ConfigMetaControllerOption,
) (*ConfigMetaController, error) {
	// initialize with defaults & the provided values
	ctl := &ConfigMetaController{
		WaitTimeoutForCondition:  30 * time.Minute,
		WaitIntervalForCondition: 1 * time.Second,
		opts:                     opts,
		BaseMetaController: BaseMetaController{
			ResourceManager:    resourceMgr,
			DynClientset:       dynClientset,
			DynInformerFactory: dynInformerFactory,
			WorkerCount:        workerCount,
			WatchControllers:   make(map[string]*WatchController),
		},
	}
	var fns = []func(){
		ctl.runOptions,
		ctl.loadConfigs,
		ctl.isDuplicateConfig,
	}
	for _, fn := range fns {
		fn()
		if ctl.err != nil {
			return nil, ctl.err
		}
	}
	return ctl, nil
}

func (mc *ConfigMetaController) String() string {
	return "Local GenericController"
}

func (mc *ConfigMetaController) runOptions() {
	for _, o := range mc.opts {
		err := o(mc)
		if err != nil {
			mc.err = err
			return
		}
	}
}

func (mc *ConfigMetaController) loadConfigs() {
	// validate
	if mc.ConfigPath == "" && mc.ConfigLoadFn == nil {
		mc.err = errors.Errorf(
			"Can't load config: Either ConfigPath or ConfigLoadFn is required",
		)
		return
	}
	// NOTE:
	// 	ConfigPath has **higher priority** to load GenericController
	// instance(s) as config(s) to run Metac
	if mc.ConfigPath != "" {
		mc.Configs, mc.err = mc.loadConfigsByPath()
	} else {
		mc.Configs, mc.err = mc.ConfigLoadFn()
	}
}

func (mc *ConfigMetaController) loadConfigsByPath() ([]*v1alpha1.GenericController, error) {
	configs, err := config.New(mc.ConfigPath).Load()
	if err != nil {
		return nil, err
	}
	return configs.ListGenericControllers()
}

// isDuplicateConfig returns error if any duplicate config
// is found
func (mc *ConfigMetaController) isDuplicateConfig() {
	var allconfigs = map[string]bool{}
	for _, conf := range mc.Configs {
		key := conf.AsNamespaceNameKey()
		if allconfigs[key] {
			mc.err = errors.Errorf(
				"Duplicate %s was found: %s",
				key,
				mc,
			)
			return
		}
		// add it to check for possible duplicates in
		// next iterations
		allconfigs[key] = true
	}
}

// Start generic meta controller by starting watch controllers
// corresponding to the provided config
func (mc *ConfigMetaController) Start() {
	mc.doneCh = make(chan struct{})

	go func() {
		defer close(mc.doneCh)
		defer utilruntime.HandleCrash()

		glog.Infof("Starting %s", mc)

		// we run this as a continuous process
		// until all the configs are loaded
		err := mc.wait(mc.startAllWatchControllers)
		if err != nil {
			glog.Fatalf("Failed to start %s: %+v", mc, err)
		}
	}()
}

// wait polls the condition until it's true, with a configured
// interval and timeout.
//
// The condition function returns a bool indicating whether it
// is satisfied, as well as an error which should be non-nil if
// and only if the function was unable to determine whether or
// not the condition is satisfied (for example if the check
// involves calling a remote server and the request failed).
func (mc *ConfigMetaController) wait(condition func() (bool, error)) error {
	// mark the start time
	start := time.Now()
	for {
		// check the condition
		done, err := condition()
		if err == nil && done {
			// returning nil implies the condition has completed
			return nil
		}
		if time.Since(start) > mc.WaitTimeoutForCondition {
			return errors.Wrapf(
				err,
				"Condition timed out after %s: %s",
				mc.WaitTimeoutForCondition,
				mc,
			)
		}
		if err != nil {
			// Log error, but keep trying until timeout.
			glog.V(7).Infof(
				"Condition failed: Will retry after %s: %s: %v",
				mc.WaitIntervalForCondition,
				mc,
				err,
			)
		} else {
			glog.V(7).Infof(
				"Waiting for condition to succeed: Will retry after %s: %s",
				mc.WaitIntervalForCondition,
				mc,
			)
		}
		// wait & then continue retrying
		time.Sleep(mc.WaitIntervalForCondition)
	}
}

// startAllWatchControllers starts all the watches configured
// in generic controllers that were provided as config to this
// binary
//
// NOTE:
//	This method is used as a condition and hence can be invoked
// more than once.
func (mc *ConfigMetaController) startAllWatchControllers() (bool, error) {
	// In this metacontroller, we are only responsible for
	// starting/stopping the relevant watch based controllers
	for _, conf := range mc.Configs {
		key := conf.AsNamespaceNameKey()
		if _, ok := mc.WatchControllers[key]; ok {
			// Already added; perhaps during earlier condition
			// checks
			continue
		}
		// watch controller i.e. a controller based on the resource
		// specified in the **watch field** of GenericController
		wc, err := NewWatchController(
			mc.ResourceManager,
			mc.DynClientset,
			mc.DynInformerFactory,
			conf,
		)
		if err != nil {
			return false, errors.Wrapf(
				err,
				"Failed to init %s: %s",
				key,
				mc,
			)
		}
		// start this watch controller
		wc.Start(mc.WorkerCount)
		mc.WatchControllers[key] = wc
	}
	return true, nil
}

// Stop stops this MetaController
func (mc *ConfigMetaController) Stop() {
	glog.Infof("Shutting down %s", mc)

	// Stop metacontroller first so there's no more changes
	// to watch controllers.
	<-mc.doneCh

	// Stop all its watch controllers
	var wg sync.WaitGroup
	for _, wCtl := range mc.WatchControllers {
		wg.Add(1)
		go func(ctl *WatchController) {
			defer wg.Done()
			ctl.Stop()
		}(wCtl)
	}
	// wait till all watch controllers are stopped
	wg.Wait()
}

// CRDMetaController represents a MetaController that
// is based on CustomResources of GenericController applied
// to the Kubernetes cluster
type CRDMetaController struct {
	BaseMetaController

	// To list GenericController CRs
	Lister metalisters.GenericControllerLister

	// To watch GenericController CR events
	Informer cache.SharedIndexInformer

	// To enqueue & dequeue GenericController CR events
	Queue workqueue.RateLimitingInterface

	// To stop watching GenericController CR events
	stopCh chan struct{}
}

// NewCRDMetaController returns a new instance of CRDMetaController
func NewCRDMetaController(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	dynClientset *dynamicclientset.Clientset,
	dynInformerFactory *dynamicinformer.SharedInformerFactory,
	metaInformerFactory metainformers.SharedInformerFactory,
	workerCount int,
) *CRDMetaController {
	// initialize
	mc := &CRDMetaController{
		BaseMetaController: BaseMetaController{
			ResourceManager:    resourceMgr,
			DynClientset:       dynClientset,
			DynInformerFactory: dynInformerFactory,
			WorkerCount:        workerCount,
			WatchControllers:   make(map[string]*WatchController),
		},
		Lister:   metaInformerFactory.Metacontroller().V1alpha1().GenericControllers().Lister(),
		Informer: metaInformerFactory.Metacontroller().V1alpha1().GenericControllers().Informer(),
		Queue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(), "CRD GenericController",
		),
	}
	// add event handlers to GenericController informer
	mc.Informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    mc.enqueueGenericController,
		UpdateFunc: mc.updateGenericController,
		DeleteFunc: mc.enqueueGenericController,
	})
	return mc
}

// String implements Stringer interface
func (mc *CRDMetaController) String() string {
	return "CRD GenericController"
}

// Start starts this MetaController
func (mc *CRDMetaController) Start() {
	mc.stopCh = make(chan struct{})
	mc.doneCh = make(chan struct{})

	go func() {
		defer close(mc.doneCh)
		defer utilruntime.HandleCrash()

		glog.Infof("Starting %s", mc)
		defer glog.Infof("Shutting down %s", mc)

		if !k8s.WaitForCacheSync(mc.String(), mc.stopCh, mc.Informer.HasSynced) {
			return
		}

		// Since we are only responsible for starting/stopping
		// the GenericController(s), so a single worker should be
		// enough
		for mc.processNextWorkItem() {
		}
	}()
}

// Stop stops this MetaController
func (mc *CRDMetaController) Stop() {
	// Stop this instance first so no changes to GenericController(s)
	// are processed
	close(mc.stopCh)
	mc.Queue.ShutDown()
	<-mc.doneCh

	// Stop all its watched resources i.e. controllers for every watch
	// specified in the GenericController(s)
	var wg sync.WaitGroup
	for _, wctl := range mc.WatchControllers {
		wg.Add(1)
		// stop the watch controller(s) in parallel via goroutines
		go func(wctl *WatchController) {
			defer wg.Done()
			wctl.Stop()
		}(wctl)
	}
	// wait till all watch controllers are stopped
	wg.Wait()
}

func (mc *CRDMetaController) processNextWorkItem() bool {
	key, quit := mc.Queue.Get()
	if quit {
		// stop the continuous reconciliation
		return false
	}
	defer mc.Queue.Done(key)

	// start reconciliation
	err := mc.sync(key.(string))
	if err != nil {
		utilruntime.HandleError(
			errors.Wrapf(
				err,
				"Failed to sync %q: Will re-queue: %s",
				key,
				mc,
			),
		)
		// requeue
		mc.Queue.AddRateLimited(key)
		return true
	}
	// reconcile was successful
	mc.Queue.Forget(key)
	return true
}

// sync reconciles the GenericController resource identified
// by the provided key
func (mc *CRDMetaController) sync(key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	glog.V(7).Infof("Will sync %s: %s", key, mc)

	ctrl, err := mc.Lister.GenericControllers(ns).Get(name)
	if apierrors.IsNotFound(err) {
		glog.V(5).Infof(
			"Resource %q not found: Will stop its controller: %s: %v",
			key,
			mc,
			err,
		)
		// cleanup this GenericController instance if exists
		if c, ok := mc.WatchControllers[key]; ok {
			c.Stop()
			delete(mc.WatchControllers, key)
		}
		// return as non error case
		return nil
	}
	// return as error for other cases
	if err != nil {
		return err
	}
	// run reconciliation on the found GenericController instance
	return mc.syncGenericController(ctrl)
}

// syncGenericController is all about starting individual
// generic controller resources
func (mc *CRDMetaController) syncGenericController(gctl *v1alpha1.GenericController) error {
	if c, ok := mc.WatchControllers[gctl.AsNamespaceNameKey()]; ok {
		// The controller was already started.
		if apiequality.Semantic.DeepEqual(gctl.Spec, c.GCtlConfig.Spec) {
			// Nothing has changed.
			return nil
		}
		// If changed, then apply this new desired state of GenericController
		// resource. In other words stop & recreate.
		c.Stop()
		delete(mc.WatchControllers, gctl.AsNamespaceNameKey())
	}

	// init the controller for the watch resource specified in
	// GenericController
	wc, err := NewWatchController(
		mc.ResourceManager,
		mc.DynClientset,
		mc.DynInformerFactory,
		gctl,
	)
	if err != nil {
		return err
	}
	// start this watch based controller
	wc.Start(mc.WorkerCount)
	// add to the registry of watch based controllers
	mc.WatchControllers[gctl.AsNamespaceNameKey()] = wc
	return nil
}

func (mc *CRDMetaController) enqueueGenericController(obj interface{}) {
	key, err := common.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(
			errors.Wrapf(
				err,
				"Enqueue failed: %s: %+v",
				obj,
				mc,
			),
		)
		return
	}
	mc.Queue.Add(key)
}

func (mc *CRDMetaController) updateGenericController(old, cur interface{}) {
	mc.enqueueGenericController(cur)
}
