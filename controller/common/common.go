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

package common

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
	dynamicinformer "openebs.io/metac/dynamic/informer"
)

var (
	// KeyFunc checks for DeletedFinalStateUnknown objects
	// before calling MetaNamespaceKeyFunc.
	KeyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc
)

// AnyUnstructRegistry is the register that holds various
// unstructured instances grouped by
//
// 1/ unstruct's apiVersion & kind, and then by
// 2/ unstruct's namespace & name
//
// This registry is useful to store arbitary unstructured
// instances in a way that is easy to find / filter later.
type AnyUnstructRegistry map[string]map[string]*unstructured.Unstructured

// InitGroupByVK initialises (or re-initializes) a group within the
// registry. Group is initialized based on the provided apiVersion & kind.
//
// Caller can issue "InsertByReference" request if it is not sure about
// initializing a group in this registry.
func (m AnyUnstructRegistry) InitGroupByVK(apiVersion, kind string) {
	m[makeKeyFromAPIVersionKind(apiVersion, kind)] =
		make(map[string]*unstructured.Unstructured)
}

// InsertByReference adds the resource to appropriate group. Inserting into
// particular group is handled automatically.
func (m AnyUnstructRegistry) InsertByReference(
	ref metav1.Object, obj *unstructured.Unstructured,
) {
	key := makeKeyFromAPIVersionKind(obj.GetAPIVersion(), obj.GetKind())
	group := m[key]
	if group == nil {
		group = make(map[string]*unstructured.Unstructured)
		m[key] = group
	}
	name := relativeName(ref, obj)
	group[name] = obj
}

// ReplaceByReference replaces the unstruct instance having
// the same name & namespace as the given unstruct instance
// with the contents of the new unstruct instance. If no
// unstruct instance exists in the registry then no action
// is taken.
func (m AnyUnstructRegistry) ReplaceByReference(
	ref metav1.Object, obj *unstructured.Unstructured,
) {

	key := makeKeyFromAPIVersionKind(obj.GetAPIVersion(), obj.GetKind())
	children := m[key]
	if children == nil {
		// Nothing to do since instance does not exist
		return
	}

	name := relativeName(ref, obj)
	if _, found := children[name]; found {
		children[name] = obj
	}
}

// FindByGroupKindName finds the resource based on the given
// apigroup, kind and name
func (m AnyUnstructRegistry) FindByGroupKindName(
	apiGroup, kind, name string,
) *unstructured.Unstructured {
	// The registry is keyed by apiVersion & kind, but we don't know
	// the version. So, check inside any GVK that matches the group and
	// kind, ignoring version.
	for key, resources := range m {
		if apiVer, k := ParseKeyToAPIVersionKind(key); k == kind {
			if g, _ := ParseAPIVersionToGroupVersion(apiVer); g == apiGroup {
				for n, res := range resources {
					if n == name {
						return res
					}
				}
			}
		}
	}
	return nil
}

// relativeName returns the name of the attachment relative to the provided
// reference.
func relativeName(ref metav1.Object, obj *unstructured.Unstructured) string {
	if ref.GetNamespace() == "" && obj.GetNamespace() != "" {
		return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
	}
	return obj.GetName()
}

// namespacedNameOrName returns the name of the resource based on its
// scope
func namespacedNameOrName(obj *unstructured.Unstructured) string {
	if obj.GetNamespace() != "" {
		return fmt.Sprintf(
			"%s/%s", obj.GetNamespace(), obj.GetName(),
		)
	}
	return obj.GetName()
}

// describeObject returns a human-readable string to identify a given object.
func describeObject(obj *unstructured.Unstructured) string {
	if ns := obj.GetNamespace(); ns != "" {
		return fmt.Sprintf("%s/%s of %s", ns, obj.GetName(), obj.GetKind())
	}
	return fmt.Sprintf("%s of %s", obj.GetName(), obj.GetKind())
}

// List expands the registry map into a flat list of unstructured
// objects; in some random order.
func (m AnyUnstructRegistry) List() []*unstructured.Unstructured {
	var list []*unstructured.Unstructured
	for _, group := range m {
		for _, obj := range group {
			list = append(list, obj)
		}
	}
	return list
}

// MakeAnyUnstructRegistryByReference builds the registry of unstructured instances.
//
// This registry is suitable for use in the `children` field of a
// CompositeController or `attachments` field of DecoratorController or
// GenericController.
//
// This function returns a registry which is a map of maps. The outer most
// map is keyed using the unstruct's type and the inner map is keyed using the
// unstruct's name. If the parent resource is clustered and the child resource
// is namespaced the inner map's keys are prefixed by the namespace of the
// child resource.
//
// This function requires parent resources has the meta.Namespace set. If the
// namespace of the parent is empty it's considered a clustered resource.
func MakeAnyUnstructRegistryByReference(
	ref metav1.Object, objList []*unstructured.Unstructured,
) AnyUnstructRegistry {
	children := make(AnyUnstructRegistry)

	for _, child := range objList {
		children.InsertByReference(ref, child)
	}
	return children
}

// makeKeyFromAPIVersionKind returns the string format for the
// given version and kind
func makeKeyFromAPIVersionKind(apiVersion, kind string) string {
	return fmt.Sprintf("%s.%s", kind, apiVersion)
}

// ParseKeyToAPIVersionKind parses the given key into apiVersion
// and kind
func ParseKeyToAPIVersionKind(key string) (apiVersion, kind string) {
	parts := strings.SplitN(key, ".", 2)
	return parts[1], parts[0]
}

// ParseAPIVersionToGroupVersion parses the given version into
// respective group and version
func ParseAPIVersionToGroupVersion(apiVersion string) (group, version string) {
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) == 1 {
		// It's a core version.
		return "", parts[0]
	}
	return parts[0], parts[1]
}

// ResourceRegistryByGK is the registrar of server API resources
// anchored by group and kind
type ResourceRegistryByGK map[string]*dynamicdiscovery.APIResource

// Set registers the given API resource against the registrar based
// on the given group and kind
func (m ResourceRegistryByGK) Set(
	apiGroup, kind string, resource *dynamicdiscovery.APIResource,
) {

	m[makeKeyFromAPIGroupKind(apiGroup, kind)] = resource
}

// Get returns the API resource from the registrar based on the given
// group and kind
func (m ResourceRegistryByGK) Get(
	apiGroup, kind string,
) *dynamicdiscovery.APIResource {

	return m[makeKeyFromAPIGroupKind(apiGroup, kind)]
}

// makeKeyFromAPIGroupKind returns the string format of the given group
// and kind
func makeKeyFromAPIGroupKind(apiGroup, kind string) string {
	return fmt.Sprintf("%s.%s", kind, apiGroup)
}

// ResourceInformerRegistryByVR acts as the registrar of Informer instances
// anchored by apiversion and resource name (i.e. plural format of kind)
type ResourceInformerRegistryByVR map[string]*dynamicinformer.ResourceInformer

// Set registers the given informer object based on the given version
// and resource
func (m ResourceInformerRegistryByVR) Set(
	apiVersion, resource string, informer *dynamicinformer.ResourceInformer,
) {

	m[makeKeyFromAPIVersionResource(apiVersion, resource)] = informer
}

// Get returns the informer instance from the registrar based on the
// given version and resource
func (m ResourceInformerRegistryByVR) Get(
	apiVersion, resource string,
) *dynamicinformer.ResourceInformer {

	return m[makeKeyFromAPIVersionResource(apiVersion, resource)]
}

// makeKeyFromAPIVersionResource returns the string format of given
// apiVersion and resource
func makeKeyFromAPIVersionResource(apiVersion, resource string) string {
	return fmt.Sprintf("%s.%s", resource, apiVersion)
}
