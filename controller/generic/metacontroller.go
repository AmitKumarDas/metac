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
	"openebs.io/metac/controller/common"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	dynamicinformer "openebs.io/metac/dynamic/informer"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// MetaController abstracts Kubernetes informers and listers
// to execute reconcile logic declared in various GenericController
// resources.
type MetaController struct {
	resourceManager    *dynamicdiscovery.APIResourceManager
	dynClientset       *dynamicclientset.Clientset
	dynInformerFactory *dynamicinformer.SharedInformerFactory

	lister   metalisters.GenericControllerLister
	informer cache.SharedIndexInformer

	queue              workqueue.RateLimitingInterface
	genericControllers map[string]*genericControllerManager

	stopCh, doneCh chan struct{}
}

// NewMetacontroller returns a new instance of MetaController
func NewMetacontroller(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	dynClientset *dynamicclientset.Clientset,
	dynInformerFactory *dynamicinformer.SharedInformerFactory,
	metaInformerFactory metainformers.SharedInformerFactory,
) *MetaController {

	mc := &MetaController{
		resourceManager:    resourceMgr,
		dynClientset:       dynClientset,
		dynInformerFactory: dynInformerFactory,

		lister:   metaInformerFactory.Metacontroller().V1alpha1().GenericControllers().Lister(),
		informer: metaInformerFactory.Metacontroller().V1alpha1().GenericControllers().Informer(),

		queue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(), "GenericMetaController",
		),
		genericControllers: make(map[string]*genericControllerManager),
	}

	mc.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    mc.enqueueGenericController,
		UpdateFunc: mc.updateGenericController,
		DeleteFunc: mc.enqueueGenericController,
	})

	return mc
}

// String implements Stringer interface
func (mc *MetaController) String() string {
	return "GenericMetaController"
}

// Start starts this MetaController
func (mc *MetaController) Start() {
	mc.stopCh = make(chan struct{})
	mc.doneCh = make(chan struct{})

	go func() {
		defer close(mc.doneCh)
		defer utilruntime.HandleCrash()

		glog.Infof("Starting %s", mc)
		defer glog.Infof("Shutting down %s", mc)

		if !k8s.WaitForCacheSync(mc.String(), mc.stopCh, mc.informer.HasSynced) {
			return
		}

		// In the metacontroller, we are only responsible for starting/stopping
		// the watched resources i.e. controllers, so a single worker should be
		// enough.
		for mc.processNextWorkItem() {
		}
	}()
}

// Stop stops this MetaController
func (mc *MetaController) Stop() {
	// Stop metacontroller first so there's no more changes
	// to watched controllers.
	close(mc.stopCh)
	mc.queue.ShutDown()
	<-mc.doneCh

	// Stop all its watched resources i.e. controllers
	var wg sync.WaitGroup
	for _, c := range mc.genericControllers {
		wg.Add(1)
		go func(c *genericControllerManager) {
			defer wg.Done()
			c.Stop()
		}(c)
	}
	wg.Wait()
}

func (mc *MetaController) processNextWorkItem() bool {
	key, quit := mc.queue.Get()
	if quit {
		return false
	}
	defer mc.queue.Done(key)

	err := mc.sync(key.(string))
	if err != nil {
		utilruntime.HandleError(
			errors.Wrapf(err, "%s: Failed to sync %q: Will re-queue", mc, key),
		)
		// requeue
		mc.queue.AddRateLimited(key)
		return true
	}

	mc.queue.Forget(key)
	return true
}

// sync reconciles GenericMetaController resources
func (mc *MetaController) sync(key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	glog.V(4).Infof("%s: Will sync %s/%s of GenericController", mc, ns, name)

	ctrl, err := mc.lister.GenericControllers(ns).Get(name)
	if apierrors.IsNotFound(err) {
		glog.V(4).Infof(
			"%s: Ignore sync %s/%s of GenericController: No longer exist",
			mc, ns, name,
		)

		// cleanup this GenericController instance if exists
		if c, ok := mc.genericControllers[key]; ok {
			c.Stop()
			delete(mc.genericControllers, key)
		}
		return nil
	}
	if err != nil {
		return err
	}

	return mc.syncGenericController(ctrl)
}

// syncGenericController is all about starting individual
// generic controller resources
func (mc *MetaController) syncGenericController(ctrl *v1alpha1.GenericController) error {
	if c, ok := mc.genericControllers[ctrl.Key()]; ok {
		// The controller was already started.
		if apiequality.Semantic.DeepEqual(ctrl.Spec, c.Controller.Spec) {
			// Nothing has changed.
			return nil
		}

		// Applying desired state of GenericController resource implies
		// stop & recreate.
		c.Stop()
		delete(mc.genericControllers, ctrl.Key())
	}

	// watched resource / controller
	wc, err := newGenericControllerManager(
		mc.resourceManager,
		mc.dynClientset,
		mc.dynInformerFactory,
		ctrl,
	)
	if err != nil {
		return err
	}

	wc.Start()
	mc.genericControllers[ctrl.Key()] = wc
	return nil
}

func (mc *MetaController) enqueueGenericController(obj interface{}) {
	key, err := common.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(
			errors.Wrapf(err, "%s: Enqueue failed for %+v", mc, obj),
		)
		return
	}

	mc.queue.Add(key)
}

func (mc *MetaController) updateGenericController(old, cur interface{}) {
	mc.enqueueGenericController(cur)
}
