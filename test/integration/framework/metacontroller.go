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

package framework

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/server"
	pointer "openebs.io/metac/third_party/kubernetes"
)

// CreateCompositeController generates a test CompositeController and installs
// it in the test API server.
func (f *Fixture) CreateCompositeController(
	name string,
	syncHookURL string,
	parentRule *v1alpha1.ResourceRule,
	childRule *v1alpha1.ResourceRule,
) *v1alpha1.CompositeController {

	childResources := []v1alpha1.CompositeControllerChildResourceRule{}
	if childRule != nil {
		childResources = append(
			childResources,
			v1alpha1.CompositeControllerChildResourceRule{ResourceRule: *childRule},
		)
	}

	cc := &v1alpha1.CompositeController{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: v1alpha1.CompositeControllerSpec{
			// Set a big resyncPeriod so tests can precisely control
			// when syncs happen.
			ResyncPeriodSeconds: pointer.Int32Ptr(3600),
			ParentResource: v1alpha1.CompositeControllerParentResourceRule{
				ResourceRule: *parentRule,
			},
			ChildResources: childResources,
			Hooks: &v1alpha1.CompositeControllerHooks{
				Sync: &v1alpha1.Hook{
					Webhook: &v1alpha1.Webhook{
						URL: &syncHookURL,
					},
				},
			},
		},
	}

	cc, err := f.metaClientset.MetacontrollerV1alpha1().
		CompositeControllers().
		Create(cc)
	if err != nil {
		f.t.Fatal(err)
	}

	f.addToTeardown(func() error {
		return f.metaClientset.MetacontrollerV1alpha1().
			CompositeControllers().
			Delete(cc.Name, nil)
	})

	return cc
}

// CreateDecoratorController generates a test DecoratorController and installs
// it in the test API server.
func (f *Fixture) CreateDecoratorController(
	name string,
	syncHookURL string,
	parentRule *v1alpha1.ResourceRule,
	childRule *v1alpha1.ResourceRule,
) *v1alpha1.DecoratorController {

	childResources := []v1alpha1.DecoratorControllerAttachmentRule{}
	if childRule != nil {
		childResources = append(
			childResources,
			v1alpha1.DecoratorControllerAttachmentRule{ResourceRule: *childRule},
		)
	}

	dc := &v1alpha1.DecoratorController{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: v1alpha1.DecoratorControllerSpec{
			// Set a big resyncPeriod so tests can precisely control
			// when syncs happen.
			ResyncPeriodSeconds: pointer.Int32Ptr(3600),
			Resources: []v1alpha1.DecoratorControllerResourceRule{
				{
					ResourceRule: *parentRule,
				},
			},
			Attachments: childResources,
			Hooks: &v1alpha1.DecoratorControllerHooks{
				Sync: &v1alpha1.Hook{
					Webhook: &v1alpha1.Webhook{
						URL: &syncHookURL,
					},
				},
			},
		},
	}

	dc, err := f.metaClientset.MetacontrollerV1alpha1().
		DecoratorControllers().
		Create(dc)
	if err != nil {
		f.t.Fatal(err)
	}

	f.addToTeardown(func() error {
		return f.metaClientset.MetacontrollerV1alpha1().
			DecoratorControllers().
			Delete(dc.Name, nil)
	})

	return dc
}

// CreateGenericController creates a GenericController and installs
// it in the API server.
func (f *Fixture) CreateGenericController(
	name string,
	namespace string,
	opts ...generic.Option,
) *v1alpha1.GenericController {

	// initialize the controller instance
	gc := &v1alpha1.GenericController{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: v1alpha1.GenericControllerSpec{
			// Set a big resyncPeriod so tests can precisely control
			// when syncs happen.
			ResyncPeriodSeconds: pointer.Int32Ptr(3600),
		},
	}

	// set other options if any
	for _, o := range opts {
		o(gc)
	}

	gc, err :=
		f.metaClientset.MetacontrollerV1alpha1().GenericControllers(namespace).Create(gc)
	if err != nil {
		f.t.Fatal(err)
	}

	f.addToTeardown(func() error {
		return f.metaClientset.MetacontrollerV1alpha1().
			GenericControllers(namespace).Delete(gc.Name, nil)
	})

	return gc
}

// CreateGenericControllerAsMetacConfig creates a new instance of
// GenericController instance
//
// NOTE:
// 	As this does not create this resource in the kubernetes
// api server this does not need a teadown callback
func (f *Fixture) CreateGenericControllerAsMetacConfig(
	name string, opts ...generic.Option,
) *v1alpha1.GenericController {

	// initialize the controller instance
	gc := &v1alpha1.GenericController{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.GenericControllerSpec{
			// Set a big resyncPeriod so tests can precisely control
			// when syncs happen.
			ResyncPeriodSeconds: pointer.Int32Ptr(3600),
		},
	}

	// set other options if any
	for _, o := range opts {
		o(gc)
	}

	return gc
}

// StartMetacFromGenericControllerConfig starts Metac based
// on the given config. It returns the stop function that
// should be invoked by the caller once caller's task is done.
func (f *Fixture) StartMetacFromGenericControllerConfig(
	gctlAsConfigFn func() ([]*v1alpha1.GenericController, error),
) (stop func()) {
	var mserver = server.Server{
		Config:            apiServerConfig,
		DiscoveryInterval: 500 * time.Millisecond,
		InformerRelist:    30 * time.Minute,
	}
	metacServer := &server.ConfigServer{
		Server:                        mserver,
		GenericControllerConfigLoadFn: gctlAsConfigFn,
	}
	stopMetacServer, err := metacServer.Start(5)
	if err != nil {
		f.t.Fatal(err)
	}
	return stopMetacServer
}
