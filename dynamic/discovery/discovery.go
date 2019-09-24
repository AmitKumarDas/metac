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

package discovery

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// APIResource wraps the original server API resource
// with additional info
type APIResource struct {
	metav1.APIResource

	// TODO (@amitkumardas):
	// Why is this required? is this not same as metav1.APIResource.Version
	APIVersion string

	hasSubresource map[string]bool
}

// GroupVersion returns the GroupVersion of this resource
func (r *APIResource) GroupVersion() schema.GroupVersion {
	gv, err := schema.ParseGroupVersion(r.APIVersion)
	if err != nil {
		// This shouldn't happen because we get this value from discovery.
		panic(fmt.Sprintf(
			"Parse GroupVersion %q failed: %v", r.APIVersion, err,
		))
	}
	return gv
}

// GroupVersionKind returns the GroupVersionKind of this resource
func (r *APIResource) GroupVersionKind() schema.GroupVersionKind {
	return r.GroupVersion().WithKind(r.Kind)
}

// GroupVersionResource returns the GroupVersionResource of this
// resource
func (r *APIResource) GroupVersionResource() schema.GroupVersionResource {
	return r.GroupVersion().WithResource(r.Name)
}

// GroupResource returns the GroupResource of this resource
func (r *APIResource) GroupResource() schema.GroupResource {
	return schema.GroupResource{Group: r.Group, Resource: r.Name}
}

// HasSubresource flags if the provided subresource is
// available in this resource
func (r *APIResource) HasSubresource(subresourceKey string) bool {
	return r.hasSubresource[subresourceKey]
}

// apiResourceRegistry is a registry of Kubernetes server
// supported API groups, versions & resources.
//
// Resources are anchored by appropriate keys for easy
// lookup. A resource can be looked up based on kind or
// resource name or sub-resource name
type apiResourceRegistry struct {
	resources, kinds, subresources map[string]*APIResource
}

// APIResourceManager helps discovering Kubernetes server
// supported API groups, versions & resources.
type APIResourceManager struct {
	mutex sync.RWMutex

	// discovered resources that are anchored by apiVersion
	//
	// NOTE:
	//	This map of registry works well, since the registry
	// itself is a map of resources anchored by resource name.
	// Hence, this combination of apiVersion and resource name
	// is sufficient to find a particular API resource.
	resources map[string]apiResourceRegistry

	// Client to discover API resource
	Client discovery.DiscoveryInterface

	stopCh, doneCh chan struct{}
}

// NewAPIResourceManager returns a new instance of
// APIResourceManager
func NewAPIResourceManager(c discovery.DiscoveryInterface) *APIResourceManager {
	return &APIResourceManager{Client: c}
}

// GetByResource returns the API resource based on the provided
// version and resource (this is typically the plural
// notation of kind)
func (mgr *APIResourceManager) GetByResource(
	apiVersion, resource string,
) *APIResource {

	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()

	registry, ok := mgr.resources[apiVersion]
	if !ok {
		return nil
	}
	return registry.resources[resource]
}

// GetByKind returns the API resource based on the provided
// version and kind
func (mgr *APIResourceManager) GetByKind(
	apiVersion, kind string,
) *APIResource {

	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()

	registry, ok := mgr.resources[apiVersion]
	if !ok {
		return nil
	}
	return registry.kinds[kind]
}

// refresh discovers all Kubernetes server resources
//
// NOTE:
// 	We do this before acquiring the lock so we don't block readers.
func (mgr *APIResourceManager) refresh() {

	glog.V(7).Info("Discovering API resources")
	apiResourceSetList, err := mgr.Client.ServerResources()
	if err != nil {
		glog.Errorf("API resource discovery failed: %v", err)
		return
	}

	// Denormalize resourceset list into map for convenient lookup
	// by either Group-Version-Kind or Group-Version-Resource
	groupVersions := make(map[string]apiResourceRegistry, len(apiResourceSetList))
	for _, apiResourceSet := range apiResourceSetList {
		gv, err := schema.ParseGroupVersion(apiResourceSet.GroupVersion)
		if err != nil {
			// This shouldn't happen because we get these values
			// from the server.
			panic(fmt.Errorf(
				"API resource discovery failed: Invalid group version %q: %v",
				apiResourceSet.GroupVersion, err,
			))
		}
		registrySet := apiResourceRegistry{
			resources:    make(map[string]*APIResource, len(apiResourceSet.APIResources)),
			kinds:        make(map[string]*APIResource, len(apiResourceSet.APIResources)),
			subresources: make(map[string]*APIResource, len(apiResourceSet.APIResources)),
		}

		for i := range apiResourceSet.APIResources {
			apiResource := &APIResource{
				APIResource: apiResourceSet.APIResources[i],
				APIVersion:  apiResourceSet.GroupVersion,
			}
			// Materialize default values from the list into each entry
			if apiResource.Group == "" {
				apiResource.Group = gv.Group
			}
			if apiResource.Version == "" {
				apiResource.Version = gv.Version
			}
			registrySet.resources[apiResource.Name] = apiResource

			// Remember which resources are subresources, and map the kind
			// to the main resource. This is different from what RESTMapper
			// provides because we already know the full GroupVersionKind
			// and just need the resource name.
			if strings.ContainsRune(apiResource.Name, '/') {
				registrySet.subresources[apiResource.Name] = apiResource
			} else {
				registrySet.kinds[apiResource.Kind] = apiResource
			}
		}

		// Flag each resource with all its supported sub resources
		for subResourceNamedPath := range registrySet.subresources {
			arr := strings.Split(subResourceNamedPath, "/")
			resName := arr[0]
			subResName := arr[1]
			apiResource := registrySet.resources[resName]
			if apiResource == nil {
				continue
			}
			if apiResource.hasSubresource == nil {
				apiResource.hasSubresource = make(map[string]bool)
			}
			apiResource.hasSubresource[subResName] = true
		}

		groupVersions[apiResourceSet.GroupVersion] = registrySet
	}

	// Replace the local cache.
	mgr.mutex.Lock()
	mgr.resources = groupVersions
	mgr.mutex.Unlock()
}

// Start executes resource discovery in the given interval
func (mgr *APIResourceManager) Start(refreshInterval time.Duration) {
	mgr.stopCh = make(chan struct{})
	mgr.doneCh = make(chan struct{})

	go func() {
		defer close(mgr.doneCh)

		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		for {
			mgr.refresh()

			select {
			case <-mgr.stopCh:
				// return / exit from this anonymous func
				return
			case <-ticker.C:
			}
		}
	}()
}

// Stop stops resource discovery ticker
func (mgr *APIResourceManager) Stop() {
	close(mgr.stopCh)
	<-mgr.doneCh
}

// HasSynced flags if any resources were discovered
func (mgr *APIResourceManager) HasSynced() bool {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()

	return mgr.resources != nil
}
