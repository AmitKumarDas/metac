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
	"github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/klog"
)

// APIResource represents the discovered resources at kubernetes
// cluster
type APIResource struct {
	metav1.APIResource
	APIVersion            string
	supportedSubResources map[string]bool
}

// GetGroupVersion returns the GroupVersion of this resource
func (r *APIResource) GetGroupVersion() schema.GroupVersion {
	gv, err := schema.ParseGroupVersion(r.APIVersion)
	if err != nil {
		// this shouldn't happen since this is a discovered resource
		panic(
			fmt.Sprintf(
				"Failed to parse GroupVersion from %q: %+v",
				r.APIVersion,
				err,
			),
		)
	}
	return gv
}

// GetGroupVersionKind returns the GroupVersionKind of this resource
func (r *APIResource) GetGroupVersionKind() schema.GroupVersionKind {
	return r.GetGroupVersion().WithKind(r.Kind)
}

// GetGroupVersionResource returns the GroupVersionResource of this
// resource
func (r *APIResource) GetGroupVersionResource() schema.GroupVersionResource {
	return r.GetGroupVersion().WithResource(r.Name)
}

// GetGroupResource returns the GroupResource of this resource
func (r *APIResource) GetGroupResource() schema.GroupResource {
	return schema.GroupResource{
		Group:    r.Group,
		Resource: r.Name,
	}
}

// HasSubresource returns true if the provided subresource is
// available in this resource
func (r *APIResource) HasSubresource(subResourceName string) bool {
	return r.supportedSubResources[subResourceName]
}

// apiResourceRegistry is the registry of resources, kinds &
// sub resources discovered at kubernetes server
//
// Resources are anchored by appropriate keys for easy lookup.
// A resource can be looked up either by its kind, resource name
// or sub-resource name.
type apiResourceRegistry struct {
	resources    map[string]*APIResource
	kinds        map[string]*APIResource
	subresources map[string]*APIResource
}

// APIResourceDiscovery discovers kubernetes server supported
// API groups, versions & resources
type APIResourceDiscovery struct {
	// DiscoveryClient to discover API resource
	DiscoveryClient discovery.DiscoveryInterface

	// GetForAPIVersionResourceFn is a functional type to get
	// discovered API resource from provided apiVersion & resource
	// name
	//
	// NOTE:
	//	This can be used to mock GetForAPIVersionResource during
	// unit test
	GetAPIForAPIVersionAndResourceFn func(apiVersion, resource string) *APIResource

	mutex sync.RWMutex

	// discovered resources anchored by **apiVersion**
	//
	// NOTE:
	//	This map of registry works well, since the registry
	// itself is a map of discovered resources anchored by
	// resource name. Hence, this combination of apiVersion
	// and resource name is sufficient to find a particular
	// API resource.
	discoveredResources map[string]apiResourceRegistry

	// isStarted is true if api discovery process has been
	// started to discover server resources at a specified
	// interval
	isStarted bool

	stopCh, doneCh chan struct{}
}

// NewAPIResourceDiscoverer returns a new instance of APIResourceDiscoverer
func NewAPIResourceDiscoverer(client discovery.DiscoveryInterface) *APIResourceDiscovery {
	return &APIResourceDiscovery{
		DiscoveryClient: client,
	}
}

// GetAPIForAPIVersionAndResource returns the API resource based on the provided
// api version and resource
//
// NOTE:
//	resource implies the name of resource which is also the plural
// notation of kind
func (d *APIResourceDiscovery) GetAPIForAPIVersionAndResource(
	apiVersion string,
	resource string,
) *APIResource {
	if d.GetAPIForAPIVersionAndResourceFn != nil {
		return d.GetAPIForAPIVersionAndResourceFn(apiVersion, resource)
	}
	return d.getAPIForAPIVersionAndResource(apiVersion, resource)
}

// getForAPIVersionResource returns the discovered API resource
// corresponding to the provided api version and resource
//
// NOTE:
//	resource implies the name of resource which is also the plural
// notation of kind
func (d *APIResourceDiscovery) getAPIForAPIVersionAndResource(
	apiVersion string,
	resource string,
) *APIResource {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	registry, ok := d.discoveredResources[apiVersion]
	if !ok {
		return nil
	}
	return registry.resources[resource]
}

// GetAPIForAPIVersionAndKind returns the discovered resource
// corresponding to the provided api version and kind
func (d *APIResourceDiscovery) GetAPIForAPIVersionAndKind(
	apiVersion string,
	kind string,
) *APIResource {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	registry, ok := d.discoveredResources[apiVersion]
	if !ok {
		return nil
	}
	return registry.kinds[kind]
}

// refresh discovers all Kubernetes server resources
//
// NOTE:
// 	We do this before acquiring the lock so we don't block readers.
func (d *APIResourceDiscovery) refresh() {
	var err error
	glog.V(7).Info("Discovering API resources")
	defer func() {
		if err == nil {
			glog.V(7).Info("API resources discovery completed")
		}
	}()
	// fetch resources for all groups & versions
	allGVResourceList, err := d.DiscoveryClient.ServerResources()
	if err != nil {
		if apierrors.IsNotFound(err) {
			glog.Warningf("Can't discover API resources: %+v", err)
			return
		}
		glog.Errorf("Failed to discover API resources: %+v", err)
		return
	}

	// Denormalize resources into map for convenient lookup
	// by either Group-Version-Kind or Group-Version-Resource
	groupVersions :=
		make(map[string]apiResourceRegistry, len(allGVResourceList))
	for _, resourceList := range allGVResourceList {
		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			// this shouldn't happen for discovered resources
			panic(errors.Errorf(
				"API resource discovery failed for group version %q: %+v",
				resourceList.GroupVersion,
				err,
			))
		}
		registrySet := apiResourceRegistry{
			resources:    make(map[string]*APIResource, len(resourceList.APIResources)),
			kinds:        make(map[string]*APIResource, len(resourceList.APIResources)),
			subresources: make(map[string]*APIResource, len(resourceList.APIResources)),
		}
		for i := range resourceList.APIResources {
			apiResource := &APIResource{
				APIResource: resourceList.APIResources[i],
				APIVersion:  resourceList.GroupVersion,
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
			if apiResource.supportedSubResources == nil {
				apiResource.supportedSubResources = make(map[string]bool)
			}
			apiResource.supportedSubResources[subResName] = true
		}
		groupVersions[resourceList.GroupVersion] = registrySet
	}
	// Replace the local cache.
	d.mutex.Lock()
	d.discoveredResources = groupVersions
	d.mutex.Unlock()
}

// Start starts resource discovery process in the given interval
func (d *APIResourceDiscovery) Start(refreshInterval time.Duration) {
	if d.isStarted || !d.StartIfNotAlready() {
		klog.V(5).Infof(
			"Won't start api discovery: Already started",
		)
		return
	}

	d.stopCh = make(chan struct{})
	d.doneCh = make(chan struct{})

	go func() {
		defer close(d.doneCh)

		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()

		for {
			d.refresh()
			select {
			case <-d.stopCh: // blocks till stop channel is closed
				// return / exit from this anonymous func
				return
			case <-ticker.C:
			}
		}
	}()
}

// Stop stops resource discovery ticker
func (d *APIResourceDiscovery) Stop() {
	close(d.stopCh)
	<-d.doneCh // blocks till done channel is closed

	// set started flag to false
	d.isStarted = false
}

// HasSynced flags if any resources were discovered
func (d *APIResourceDiscovery) HasSynced() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.discoveredResources != nil
}

// StartIfNotAlready sets started flag to true if resource discovery
// process has not been started. It returns false if api discovery
// was started previously.
func (d *APIResourceDiscovery) StartIfNotAlready() bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.isStarted {
		return false
	}
	d.isStarted = true
	return true
}

// GetAPIResourcesForKind returns the list of discovered API resources
// corresponding to the provided kind
func (d *APIResourceDiscovery) GetAPIResourcesForKind(kind string) []*metav1.APIResource {
	var apiResources []*metav1.APIResource
	d.mutex.Lock()
	defer d.mutex.Unlock()

	for _, apiResourceRegistery := range d.discoveredResources {
		apiResource, isExist := apiResourceRegistery.kinds[kind]
		if isExist {
			apiResources = append(apiResources, &apiResource.APIResource)
		}
	}
	return apiResources
}
