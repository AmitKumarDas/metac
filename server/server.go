/*
Copyright 2019 Google Inc.

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

package server

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	metaclientset "openebs.io/metac/client/generated/clientset/versioned"
	metainformers "openebs.io/metac/client/generated/informers/externalversions"
	"openebs.io/metac/controller/composite"
	"openebs.io/metac/controller/decorator"
	"openebs.io/metac/controller/generic"
	dynamicclientset "openebs.io/metac/dynamic/clientset"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	dynamicinformer "openebs.io/metac/dynamic/informer"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type controller interface {
	Start()
	Stop()
}

// Start will start this controller service
func Start(
	config *rest.Config,
	discoveryInterval time.Duration,
	informerRelist time.Duration,
) (stop func(), err error) {

	// Periodically refresh discovery to pick up newly-installed resources.
	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(config)
	resourceMgr := dynamicdiscovery.NewAPIResourceManager(discoveryClient)

	// We don't care about stopping this cleanly since it has no external effects.
	resourceMgr.Start(discoveryInterval)

	// Create informer factory for metacontroller API objects.
	metaClientset, err := metaclientset.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"%s: Start server failed: Can't create clientset",
			v1alpha1.SchemeGroupVersion,
		)
	}
	metaInformerFactory :=
		metainformers.NewSharedInformerFactory(metaClientset, informerRelist)

	// Create dynamic clientset (factory for dynamic clients).
	dynamicClientset, err := dynamicclientset.New(config, resourceMgr)
	if err != nil {
		return nil, err
	}
	// Create dynamic informer factory (for sharing dynamic informers).
	dynamicInformerFactory :=
		dynamicinformer.NewSharedInformerFactory(dynamicClientset, informerRelist)

	// Start various metacontrollers (controllers that spawn controllers).
	// Each one requests the informers it needs from the factory.
	metaControllers := []controller{
		composite.NewMetacontroller(
			resourceMgr,
			dynamicClientset,
			dynamicInformerFactory,
			metaInformerFactory,
			metaClientset,
		),
		decorator.NewMetacontroller(
			resourceMgr,
			dynamicClientset,
			dynamicInformerFactory,
			metaInformerFactory,
		),
		generic.NewMetacontroller(
			resourceMgr,
			dynamicClientset,
			dynamicInformerFactory,
			metaInformerFactory,
		),
	}

	// Start all requested informers.
	// We don't care about stopping this cleanly since it has no external effects.
	metaInformerFactory.Start(nil)

	// Start all controllers.
	for _, c := range metaControllers {
		c.Start()
	}

	// Return a function that will stop all controllers.
	return func() {
		var wg sync.WaitGroup
		for _, c := range metaControllers {
			wg.Add(1)
			go func(c controller) {
				defer wg.Done()
				c.Stop()
			}(c)
		}
		wg.Wait()
	}, nil
}
