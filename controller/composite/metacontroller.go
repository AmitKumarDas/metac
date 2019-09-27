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

package composite

import (
	"fmt"
	"sync"

	"github.com/golang/glog"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	metaclientset "openebs.io/metac/client/generated/clientset/versioned"
	metainformers "openebs.io/metac/client/generated/informers/externalversions"
	metalisters "openebs.io/metac/client/generated/listers/metacontroller/v1alpha1"
	"openebs.io/metac/controller/common"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	dynamicinformer "openebs.io/metac/dynamic/informer"
	k8s "openebs.io/metac/third_party/kubernetes"
)

type Metacontroller struct {
	resourceManager        *dynamicdiscovery.APIResourceManager
	metaClientset          metaclientset.Interface
	dynamicClientset       *dynamicclientset.Clientset
	dynamicInformerFactory *dynamicinformer.SharedInformerFactory

	lister           metalisters.CompositeControllerLister
	informer         cache.SharedIndexInformer
	revisionLister   metalisters.ControllerRevisionLister
	revisionInformer cache.SharedIndexInformer

	queue             workqueue.RateLimitingInterface
	parentControllers map[string]*parentController

	stopCh, doneCh chan struct{}
}

func NewMetacontroller(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	dynamicClientset *dynamicclientset.Clientset,
	dynamicInformerFactory *dynamicinformer.SharedInformerFactory,
	metaInformerFactory metainformers.SharedInformerFactory,
	metaClientset metaclientset.Interface,
) *Metacontroller {

	mc := &Metacontroller{
		resourceManager:        resourceMgr,
		metaClientset:          metaClientset,
		dynamicClientset:       dynamicClientset,
		dynamicInformerFactory: dynamicInformerFactory,

		lister:           metaInformerFactory.Metacontroller().V1alpha1().CompositeControllers().Lister(),
		informer:         metaInformerFactory.Metacontroller().V1alpha1().CompositeControllers().Informer(),
		revisionLister:   metaInformerFactory.Metacontroller().V1alpha1().ControllerRevisions().Lister(),
		revisionInformer: metaInformerFactory.Metacontroller().V1alpha1().ControllerRevisions().Informer(),

		queue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "CompositeController"),
		parentControllers: make(map[string]*parentController),
	}

	mc.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    mc.enqueueCompositeController,
		UpdateFunc: mc.updateCompositeController,
		DeleteFunc: mc.enqueueCompositeController,
	})

	return mc
}

func (mc *Metacontroller) Start() {
	mc.stopCh = make(chan struct{})
	mc.doneCh = make(chan struct{})

	go func() {
		defer close(mc.doneCh)
		defer utilruntime.HandleCrash()

		glog.Info("Starting CompositeController metacontroller")
		defer glog.Info("Shutting down CompositeController metacontroller")

		if !k8s.WaitForCacheSync("CompositeController", mc.stopCh, mc.informer.HasSynced) {
			return
		}

		// In the metacontroller, we are only responsible for starting/stopping
		// the actual controllers, so a single worker should be enough.
		for mc.processNextWorkItem() {
		}
	}()
}

func (mc *Metacontroller) Stop() {
	// Stop metacontroller first so there's no more changes to controllers.
	close(mc.stopCh)
	mc.queue.ShutDown()
	<-mc.doneCh

	// Stop all controllers.
	var wg sync.WaitGroup
	for _, pc := range mc.parentControllers {
		wg.Add(1)
		go func(pc *parentController) {
			defer wg.Done()
			pc.Stop()
		}(pc)
	}
	wg.Wait()
}

func (mc *Metacontroller) processNextWorkItem() bool {
	key, quit := mc.queue.Get()
	if quit {
		return false
	}
	defer mc.queue.Done(key)

	err := mc.sync(key.(string))
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to sync CompositeController %q: %v", key, err))
		mc.queue.AddRateLimited(key)
		return true
	}

	mc.queue.Forget(key)
	return true
}

func (mc *Metacontroller) sync(key string) error {
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	glog.V(4).Infof("sync CompositeController %v", name)

	cc, err := mc.lister.Get(name)
	if apierrors.IsNotFound(err) {
		glog.V(4).Infof("CompositeController %v has been deleted", name)
		// Stop and remove the controller if it exists.
		if pc, ok := mc.parentControllers[name]; ok {
			pc.Stop()
			delete(mc.parentControllers, name)
		}
		return nil
	}
	if err != nil {
		return err
	}
	return mc.syncCompositeController(cc)
}

func (mc *Metacontroller) syncCompositeController(cc *v1alpha1.CompositeController) error {
	if pc, ok := mc.parentControllers[cc.Name]; ok {
		// The controller was already started.
		if apiequality.Semantic.DeepEqual(cc.Spec, pc.api.Spec) {
			// Nothing has changed.
			return nil
		}
		// Stop and remove the controller so it can be recreated.
		pc.Stop()
		delete(mc.parentControllers, cc.Name)
	}

	pc, err := newParentController(mc.resourceManager, mc.dynamicClientset, mc.dynamicInformerFactory, mc.metaClientset, mc.revisionLister, cc)
	if err != nil {
		return err
	}
	pc.Start()
	mc.parentControllers[cc.Name] = pc
	return nil
}

func (mc *Metacontroller) enqueueCompositeController(obj interface{}) {
	key, err := common.KeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %+v: %v", obj, err))
		return
	}
	mc.queue.Add(key)
}

func (mc *Metacontroller) updateCompositeController(old, cur interface{}) {
	mc.enqueueCompositeController(cur)
}
