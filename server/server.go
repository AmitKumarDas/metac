/*
Copyright 2019 Google Inc.
Copyright 2020 The MayaData Authors.

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

// KubeDetails provides the config & discovery related
// information about the connected kubernetes cluster
type KubeDetails struct {
	Config               *rest.Config
	NewAPIDiscovery      func() *dynamicdiscovery.APIResourceDiscovery
	GetMetacAPIDiscovery func() *dynamicdiscovery.APIResourceDiscovery
}

// controller abstracts the contracts exposed by all metac
// controllers
//
// NOTE:
//	Current metac controllers like GenericController,
// CompositeController & DecoratorController adhere to the
// contracts exposed by this interface
type controller interface {
	Start()
	Stop()
}

// Server represents the Metac server
type Server struct {
	// Kubernetes config required to make API calls
	Config *rest.Config

	// How often to refresh discovery cache to pick
	// up newly-installed resources
	DiscoveryInterval time.Duration

	// How often to flush local caches and relist
	// objects from the API server
	InformerRelist time.Duration

	// api discovery instance used by metac's metacontrollers
	apiDiscovery *dynamicdiscovery.APIResourceDiscovery
}

// GetKubeDetails returns information about the connected
// kubernetes cluster. These details are helpful when custom
// controllers import metac & in-turn want to invoke kubernetes
// apis directly without tying into metacontrollers' reconciliation
// process.
//
// This helps resulting binaries _(read custom controllers)_
// to access copies of kubernetes config as well kubernetes api
// discovery utility.
func (s *Server) GetKubeDetails() *KubeDetails {
	return &KubeDetails{
		Config: s.Config,
		// This returns a new instance of api discovery instance.
		//
		// NOTE:
		// 	This needs to be started with appropriate refresh interval
		// before being used.
		NewAPIDiscovery: func() *dynamicdiscovery.APIResourceDiscovery {
			client := discovery.NewDiscoveryClientForConfigOrDie(
				s.Config,
			)
			return dynamicdiscovery.NewAPIResourceDiscoverer(client)
		},
		// This returns the api discovery instance that is currently
		// being used by the metacontrollers.
		//
		// NOTE:
		//	This does not need to be started to be used.
		GetMetacAPIDiscovery: func() *dynamicdiscovery.APIResourceDiscovery {
			return s.apiDiscovery
		},
	}
}

// CRDServer represents metac server based on metac's CRDs.
// In other words, this is about running Kubernetes controllers
// against various MetaControllers. MetaControllers
// are applied as Kubernetes CustomResourceDefinition(s).
type CRDServer struct {
	*Server
}

func (s *CRDServer) String() string {
	return "CRD Metac Server"
}

// Start metac server
func (s *CRDServer) Start(workerCount int) (stop func(), err error) {
	// refresh discovery cache to pick up newly-installed resources.
	discoveryClient :=
		discovery.NewDiscoveryClientForConfigOrDie(s.Config)
	s.apiDiscovery =
		dynamicdiscovery.NewAPIResourceDiscoverer(discoveryClient)
	// We don't care about stopping this cleanly since it has no
	// external effects.
	s.apiDiscovery.Start(s.DiscoveryInterval)

	// init the clientset
	metaClientset, err := metaclientset.NewForConfig(s.Config)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"Failed to start %s: Can't create clientset: %s",
			s,
			v1alpha1.SchemeGroupVersion,
		)
	}

	// informer factory for metacontroller objects.
	metaInformerFactory := metainformers.NewSharedInformerFactory(
		metaClientset,
		s.InformerRelist,
	)

	// Create dynamic clientset (factory for dynamic clients).
	dynamicClientset, err := dynamicclientset.New(s.Config, s.apiDiscovery)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"Failed to start %s: Can't create dynamic clientset: %s",
			s,
			v1alpha1.SchemeGroupVersion,
		)
	}

	// Create dynamic informer factory (for sharing dynamic informers).
	dynamicInformerFactory := dynamicinformer.NewSharedInformerFactory(
		dynamicClientset,
		s.InformerRelist,
	)

	// Start various metacontrollers (controllers that spawn controllers).
	// Each one requests the informers it needs from the factory.
	metaControllers := []controller{
		composite.NewMetacontroller(
			s.apiDiscovery,
			dynamicClientset,
			dynamicInformerFactory,
			metaInformerFactory,
			metaClientset,
			workerCount,
		),
		decorator.NewMetacontroller(
			s.apiDiscovery,
			dynamicClientset,
			dynamicInformerFactory,
			metaInformerFactory,
			workerCount,
		),
		generic.NewCRDMetaController(
			s.apiDiscovery,
			dynamicClientset,
			dynamicInformerFactory,
			metaInformerFactory,
			workerCount,
		),
	}

	// Start all requested informers.
	// We don't care about stopping this cleanly since it has no external effects.
	metaInformerFactory.Start(nil)

	// Start all controllers.
	for _, c := range metaControllers {
		c.Start()
	}

	// Return stop function that can be used by the clients
	// of this method to stop all meta controllers that were
	// started here
	return func() {
		var wg sync.WaitGroup
		for _, mctl := range metaControllers {
			wg.Add(1)
			go func(mctl controller) {
				defer wg.Done()
				mctl.Stop()
			}(mctl)
		}
		// wait till all meta controllers are stopped
		wg.Wait()
	}, nil
}

// ConfigServer represents metac server based on metac
// configs. In other words, this is about loading various
// MetaController custom resources as config files.
// MetaControllers are **not run** as Kubernetes
// CustomResourceDefinition(s).
type ConfigServer struct {
	*Server

	// Path that has the config files(s) to run Metac
	ConfigPath string

	// Function that fetches GenericController instances to
	// be used as configs to run Metac
	//
	// NOTE:
	//	One may use ConfigPath **or** this function.
	//
	// NOTE:
	//  ConfigPath has higher priority
	GenericControllerConfigLoadFn func() ([]*v1alpha1.GenericController, error)

	// Number of workers per watch controller
	workerCount int
}

func (s *ConfigServer) String() string {
	return "Config Metac Server"
}

// Start metac server
func (s *ConfigServer) Start(workerCount int) (stop func(), err error) {
	// refresh discovery cache to pick up newly-installed resources.
	discoveryClient :=
		discovery.NewDiscoveryClientForConfigOrDie(s.Config)
	s.apiDiscovery =
		dynamicdiscovery.NewAPIResourceDiscoverer(discoveryClient)
	// We don't care about stopping this cleanly since it
	// has no external effects
	s.apiDiscovery.Start(s.DiscoveryInterval)

	// Create dynamic clientset (factory for dynamic clients).
	dynamicClientset, err := dynamicclientset.New(s.Config, s.apiDiscovery)
	if err != nil {
		return nil, err
	}

	// Create dynamic informer factory (for sharing dynamic informers).
	dynamicInformerFactory := dynamicinformer.NewSharedInformerFactory(
		dynamicClientset,
		s.InformerRelist,
	)

	// various generic meta controller options to setup meta controller
	configOpts := []generic.ConfigMetaControllerOption{
		generic.SetMetaControllerConfigLoadFn(
			s.GenericControllerConfigLoadFn,
		),
		generic.SetMetaControllerConfigPath(s.ConfigPath),
	}

	genericMetac, err := generic.NewConfigMetaController(
		s.apiDiscovery,
		dynamicClientset,
		dynamicInformerFactory,
		workerCount,
		configOpts...,
	)
	if err != nil {
		return nil, err
	}

	// Start various metacontrollers (controllers that spawn controllers).
	// Each one requests the informers it needs from the factory.
	metaControllers := []controller{
		// TODO (@amitkumardas):
		//
		// Currently GenericController is the only meta controller
		// supported to run using config i.e. runaslocal flag.
		// Support for other controllers needs be introduced.
		genericMetac,
	}

	// Start all controllers.
	for _, c := range metaControllers {
		c.Start()
	}

	// Return a function that will stop all controllers.
	return func() {
		var wg sync.WaitGroup
		for _, mctl := range metaControllers {
			wg.Add(1)
			go func(mctl controller) {
				defer wg.Done()
				mctl.Stop()
			}(mctl)
		}
		// wait till all meta controllers are stopped
		wg.Wait()
	}, nil
}
