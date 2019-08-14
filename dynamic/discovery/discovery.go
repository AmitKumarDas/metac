/*
Copyright 2017 Google Inc.

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

// APIResource wraps the original API resource schema
// with additional info required during discovery
// process
type APIResource struct {
	metav1.APIResource
	APIVersion     string
	subresourceMap map[string]bool
}

// GroupVersion returns the GroupVersion of this resource
func (r *APIResource) GroupVersion() schema.GroupVersion {
	gv, err := schema.ParseGroupVersion(r.APIVersion)
	if err != nil {
		// This shouldn't happen because we get this value from discovery.
		panic(fmt.Sprintf(
			"Failed to parse group/version %q: %v", r.APIVersion, err,
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

// HasSubresource indicates if the provided subresource is
// available in this resource
func (r *APIResource) HasSubresource(subresourceKey string) bool {
	return r.subresourceMap[subresourceKey]
}

// groupVersionEntry acts as a registry of maps for
// convenient lookup by either Group-Version-Kind or
// Group-Version-Resource.
type groupVersionEntry struct {
	resources, kinds, subresources map[string]*APIResource
}

// ResourceMap provides operations against API resources
// that are discovered from K8s cluster. It also exposes
// function to discover these resources.
type ResourceMap struct {
	mutex sync.RWMutex

	// map of resources anchored by apiVersion
	groupVersions map[string]groupVersionEntry

	discoveryClient discovery.DiscoveryInterface
	stopCh, doneCh  chan struct{}
}

// Get returns the API resource based on the provided
// version and resource (this is typically the plural
// notation of kind)
func (rm *ResourceMap) Get(apiVersion, resource string) (result *APIResource) {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	gv, ok := rm.groupVersions[apiVersion]
	if !ok {
		return nil
	}
	return gv.resources[resource]
}

// GetKind returns the API resource based on the provided
// version and kind
func (rm *ResourceMap) GetKind(apiVersion, kind string) (result *APIResource) {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	gv, ok := rm.groupVersions[apiVersion]
	if !ok {
		return nil
	}
	return gv.kinds[kind]
}

// refresh fetches all API Group-Versions and their resources
// from the server.
//
// NOTE:
// 	We do this before acquiring the lock so we don't block readers.
func (rm *ResourceMap) refresh() {

	glog.V(7).Info("Refreshing API discovery info")
	groups, err := rm.discoveryClient.ServerResources()
	if err != nil {
		glog.Errorf("Failed to fetch discovery info: %v", err)
		return
	}

	// Denormalize resource lists into maps for convenient lookup
	// by either Group-Version-Kind or Group-Version-Resource
	groupVersions := make(map[string]groupVersionEntry, len(groups))
	for _, group := range groups {
		gv, err := schema.ParseGroupVersion(group.GroupVersion)
		if err != nil {
			// This shouldn't happen because we get these values
			// from the server.
			panic(fmt.Errorf(
				"Failed to fetch discovery info: invalid group version: %v",
				err,
			))
		}
		gve := groupVersionEntry{
			resources:    make(map[string]*APIResource, len(group.APIResources)),
			kinds:        make(map[string]*APIResource, len(group.APIResources)),
			subresources: make(map[string]*APIResource, len(group.APIResources)),
		}

		for i := range group.APIResources {
			apiResource := &APIResource{
				APIResource: group.APIResources[i],
				APIVersion:  group.GroupVersion,
			}
			// Materialize default values from the list into each entry
			if apiResource.Group == "" {
				apiResource.Group = gv.Group
			}
			if apiResource.Version == "" {
				apiResource.Version = gv.Version
			}
			gve.resources[apiResource.Name] = apiResource

			// Remember which resources are subresources, and map the kind
			// to the main resource. This is different from what RESTMapper
			// provides because we already know the full GroupVersionKind
			// and just need the resource name.
			if strings.ContainsRune(apiResource.Name, '/') {
				gve.subresources[apiResource.Name] = apiResource
			} else {
				gve.kinds[apiResource.Kind] = apiResource
			}
		}

		// Group all subresources for a resource.
		for apiSubresourceName := range gve.subresources {
			arr := strings.Split(apiSubresourceName, "/")
			apiResourceName := arr[0]
			subresourceKey := arr[1]
			apiResource := gve.resources[apiResourceName]
			if apiResource == nil {
				continue
			}
			if apiResource.subresourceMap == nil {
				apiResource.subresourceMap = make(map[string]bool)
			}
			apiResource.subresourceMap[subresourceKey] = true
		}

		groupVersions[group.GroupVersion] = gve
	}

	// Replace the local cache.
	rm.mutex.Lock()
	rm.groupVersions = groupVersions
	rm.mutex.Unlock()
}

// Start executes resource discovery in the given interval
func (rm *ResourceMap) Start(refreshInterval time.Duration) {
	rm.stopCh = make(chan struct{})
	rm.doneCh = make(chan struct{})

	go func() {
		defer close(rm.doneCh)

		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		for {
			rm.refresh()

			select {
			case <-rm.stopCh:
				return
			case <-ticker.C:
			}
		}
	}()
}

// Stop stops resource discovery ticker
func (rm *ResourceMap) Stop() {
	close(rm.stopCh)
	<-rm.doneCh
}

// HasSynced flags if any resources were discovered
func (rm *ResourceMap) HasSynced() bool {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()
	return rm.groupVersions != nil
}

// NewResourceMap returns a new instance of ResourceMap
func NewResourceMap(dc discovery.DiscoveryInterface) *ResourceMap {
	return &ResourceMap{
		discoveryClient: dc,
	}
}
