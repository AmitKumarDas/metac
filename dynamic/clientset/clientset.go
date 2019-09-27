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

package clientset

import (
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"

	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	dynamicobject "openebs.io/metac/dynamic/object"
)

// Clientset manages various k8s client related operations against one
// or more discovered API resource(s). Clientset has the ability to
// provide dynamic client for specific resource.
type Clientset struct {
	config          rest.Config
	resourceManager *dynamicdiscovery.APIResourceManager
	dynamicClient   dynamic.Interface
}

// New returns a new instance of Clientset
func New(
	config *rest.Config, resourceMgr *dynamicdiscovery.APIResourceManager,
) (*Clientset, error) {

	dc, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrapf(err, "New clientset failed")
	}

	return &Clientset{
		config:          *config,
		resourceManager: resourceMgr,
		dynamicClient:   dc,
	}, nil
}

// HasSynced flags if resources managed by this clientset have
// synced i.e. refreshed from the API server
func (cs *Clientset) HasSynced() bool {
	return cs.resourceManager.HasSynced()
}

// GetClientByResource returns the resource client corresponding to
// the given version & resource name (i.e. plural of kind)
func (cs *Clientset) GetClientByResource(apiVersion, resource string) (*ResourceClient, error) {
	// Look up the requested resource in discovery.
	apiResource := cs.resourceManager.GetByResource(apiVersion, resource)
	if apiResource == nil {
		return nil, errors.Errorf(
			"Failed to initialise dynamic client for resource %q in apiVersion %q",
			resource,
			apiVersion,
		)
	}
	return cs.resource(apiResource), nil
}

// GetClientByKind returns the specific dynamic client of the given
// resource by its version & kind
func (cs *Clientset) GetClientByKind(apiVersion, kind string) (*ResourceClient, error) {
	// Look up the requested resource in discovery.
	apiResource := cs.resourceManager.GetByKind(apiVersion, kind)
	if apiResource == nil {
		return nil, fmt.Errorf(
			"Failed to initialise dynamic client for kind %q in apiVersion %q",
			kind,
			apiVersion,
		)
	}
	return cs.resource(apiResource), nil
}

// resource returns a new dynamic client instance of the given resource
//
// NOTE:
//	The returned client instance is specific to the given resource
func (cs *Clientset) resource(apiResource *dynamicdiscovery.APIResource) *ResourceClient {
	client := cs.dynamicClient.Resource(apiResource.GroupVersionResource())
	return &ResourceClient{
		ResourceInterface: client,
		APIResource:       apiResource,
		rootClient:        client,
	}
}

// ResourceClient is a combination of APIResource and a dynamic Client.
//
// Passing this around makes it easier to write code that deals with arbitrary
// resource types and often needs to know the API discovery details.
// This wrapper also adds convenience functions that are useful for any client.
//
// It can be used on either namespaced or cluster-scoped resources.
// Call Namespace() to return a client that's scoped down to a given namespace.
type ResourceClient struct {
	dynamic.ResourceInterface
	*dynamicdiscovery.APIResource

	rootClient dynamic.NamespaceableResourceInterface
}

// Namespace returns a copy of the ResourceClient with the client namespace set.
//
// This can be chained to set the namespace to something else.
// Pass "" to return a client with the namespace cleared.
// If the resource is cluster-scoped, this is a no-op.
func (rc *ResourceClient) Namespace(namespace string) *ResourceClient {
	// Ignore the namespace if the resource is cluster-scoped.
	if !rc.Namespaced {
		return rc
	}
	// Reset to cluster-scoped if provided namespace is empty.
	ri := dynamic.ResourceInterface(rc.rootClient)
	if namespace != "" {
		ri = rc.rootClient.Namespace(namespace)
	}
	return &ResourceClient{
		ResourceInterface: ri,
		APIResource:       rc.APIResource,
		rootClient:        rc.rootClient,
	}
}

// AtomicUpdate performs an atomic read-modify-write loop, retrying on
// optimistic concurrency conflicts.
//
// It only uses the identity (name/namespace/uid) of the provided 'orig' object,
// not the contents. The object passed to the update() func will be from a live
// GET against the API server.
//
// This should only be used for unconditional writes, as in, "I want to make
// this change right now regardless of what else may have changed since I last
// read the object."
//
// The update() func should modify the passed object and return true to go ahead
// with the update, or false if no update is required.
func (rc *ResourceClient) AtomicUpdate(
	orig *unstructured.Unstructured,
	updateFunc func(obj *unstructured.Unstructured) bool,
) (result *unstructured.Unstructured, err error) {
	name := orig.GetName()

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current, err := rc.Get(name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if current.GetUID() != orig.GetUID() {
			// The original object was deleted and replaced with a new one.
			return apierrors.NewNotFound(rc.GroupResource(), name)
		}
		if changed := updateFunc(current); !changed {
			// There's nothing to do.
			result = current
			return nil
		}
		result, err = rc.Update(current, metav1.UpdateOptions{})
		return err
	})
	return result, err
}

// AddFinalizer adds the given finalizer to the list, if it isn't there already.
func (rc *ResourceClient) AddFinalizer(
	orig *unstructured.Unstructured,
	name string,
) (*unstructured.Unstructured, error) {
	return rc.AtomicUpdate(orig, func(obj *unstructured.Unstructured) bool {
		if dynamicobject.HasFinalizer(obj, name) {
			// Nothing to do. Abort update.
			return false
		}
		dynamicobject.AddFinalizer(obj, name)
		return true
	})
}

// RemoveFinalizer removes the given finalizer from the list, if it's there.
func (rc *ResourceClient) RemoveFinalizer(
	orig *unstructured.Unstructured,
	name string,
) (*unstructured.Unstructured, error) {
	return rc.AtomicUpdate(orig, func(obj *unstructured.Unstructured) bool {
		if !dynamicobject.HasFinalizer(obj, name) {
			// Nothing to do. Abort update.
			return false
		}
		dynamicobject.RemoveFinalizer(obj, name)
		return true
	})
}

// AtomicStatusUpdate is similar to AtomicUpdate, except that it updates status.
func (rc *ResourceClient) AtomicStatusUpdate(
	orig *unstructured.Unstructured,
	update func(obj *unstructured.Unstructured) bool,
) (result *unstructured.Unstructured, err error) {
	name := orig.GetName()

	// We should call GetStatus (if it HasSubresource) to respect subresource
	// RBAC rules, but the dynamic client does not support this yet.
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current, err := rc.Get(name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if current.GetUID() != orig.GetUID() {
			// The original object was deleted and replaced with a new one.
			return apierrors.NewNotFound(rc.GroupResource(), name)
		}
		if changed := update(current); !changed {
			// There's nothing to do.
			result = current
			return nil
		}

		if rc.HasSubresource("status") {
			result, err = rc.UpdateStatus(current, metav1.UpdateOptions{})
		} else {
			result, err = rc.Update(current, metav1.UpdateOptions{})
		}
		return err
	})
	return result, err
}
