/*
Copyright 2018 Google Inc.
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
// 1/ unstruct's **apiVersion** & **kind**, and then by
// 2/ unstruct's namespace & name
//
// This registry is useful to store arbitary unstructured
// instances in a way that is easy to find / filter later.
type AnyUnstructRegistry map[string]map[string]*unstructured.Unstructured

// String implements Stringer interface
func (m AnyUnstructRegistry) String() string {
	var details []string
	var count int
	for vk, group := range m {
		for nsname := range group {
			details = append(
				details,
				fmt.Sprintf(
					"- %s: %s",
					vk,
					nsname,
				),
			)
			count++
		}
	}
	title := fmt.Sprintf("%d objects found", count)
	return fmt.Sprintf(
		"%s\n\t%s",
		title,
		strings.Join(details, "\n"),
	)
}

// IsEmpty returns true if this registry is empty
func (m AnyUnstructRegistry) IsEmpty() bool {
	for _, group := range m {
		for _, obj := range group {
			if obj != nil && obj.Object != nil {
				// if atleast one obj exists then
				// registry is not empty
				return false
			}
		}
	}
	return true
}

// Len returns count of not nil items in this registry
func (m AnyUnstructRegistry) Len() int {
	var count int
	for _, group := range m {
		for _, obj := range group {
			if obj != nil && obj.Object != nil {
				// count if the obj is not nil
				count++
			}
		}
	}
	return count
}

// Init initialises (or re-initializes) a group within the
// registry. Group is initialized based on the provided
// apiVersion & kind.
//
// Caller can issue "InsertByReference" request if it is not sure about
// initializing a group in this registry.
func (m AnyUnstructRegistry) Init(apiVersion, kind string) {
	m[makeKeyFromAPIVersionKind(apiVersion, kind)] =
		make(map[string]*unstructured.Unstructured)
}

// InsertByReference adds the resource to appropriate group
func (m AnyUnstructRegistry) InsertByReference(
	reference metav1.Object,
	obj *unstructured.Unstructured,
) {
	key := makeKeyFromAPIVersionKind(
		obj.GetAPIVersion(),
		obj.GetKind(),
	)
	// get the group based on apiVersion & kind
	group := m[key]
	if group == nil {
		// initialise group placeholder
		group = make(map[string]*unstructured.Unstructured)
		m[key] = group
	}
	// get the relative name of the object
	// i.e. name based on the reference's namespace scope
	name := relativeName(reference, obj)
	// store the object in the group mapped against the
	// relative name
	group[name] = obj
}

// Insert adds the resource to appropriate group
func (m AnyUnstructRegistry) Insert(obj *unstructured.Unstructured) {
	key := makeKeyFromAPIVersionKind(
		obj.GetAPIVersion(),
		obj.GetKind(),
	)
	// get the group based on apiVersion & kind
	group := m[key]
	if group == nil {
		// initialise group placeholder
		group = make(map[string]*unstructured.Unstructured)
		m[key] = group
	}
	// get the namespaced name of the object
	nsName := namespaceNameOrName(obj)
	// use namespaced name as the key
	group[nsName] = obj
}

// ReplaceByReference replaces the unstruct instance having
// the same name & namespace as the given unstruct instance
// with the contents of the new unstruct instance. If no
// unstruct instance exists in the registry then no action
// is taken.
func (m AnyUnstructRegistry) ReplaceByReference(
	reference metav1.Object,
	obj *unstructured.Unstructured,
) {
	key := makeKeyFromAPIVersionKind(
		obj.GetAPIVersion(),
		obj.GetKind(),
	)
	// get all children by apiVersion & kind
	children := m[key]
	if children == nil {
		// Nothing to do since instance does not exist
		return
	}
	// build object name in the format it is saved
	// in the registry
	name := relativeName(reference, obj)
	if _, found := children[name]; found {
		children[name] = obj
	}
}

// Replace replaces the unstruct instance having the same
// name & namespace as the given unstruct instance
// with the contents of the new unstruct instance. If no
// unstruct instance exists in the registry then no action
// is taken.
func (m AnyUnstructRegistry) Replace(obj *unstructured.Unstructured) {
	key := makeKeyFromAPIVersionKind(
		obj.GetAPIVersion(),
		obj.GetKind(),
	)
	// get all children by apiVersion & kind
	children := m[key]
	if children == nil {
		// Nothing to do since instance does not exist
		return
	}
	// get namespaced name of the object
	nsName := namespaceNameOrName(obj)
	if _, found := children[nsName]; found {
		children[nsName] = obj
	}
}

// FindByGroupKindName finds the resource based on the given
// apigroup, kind and name*
//
// NOTE:
//	Provided name can either be based on:
//
// [1] Namespace & name of the object, **or**
// [2] Relative name of the object i.e. depends on
// whether the parent/reference is namespaced scope or not
func (m AnyUnstructRegistry) FindByGroupKindName(
	apiGroup string,
	kind string,
	name string,
) *unstructured.Unstructured {
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

// relativeName returns the name of the attachment relative to the
// provided reference.
func relativeName(
	reference metav1.Object,
	object *unstructured.Unstructured,
) string {
	if reference.GetNamespace() == "" && object.GetNamespace() != "" {
		// with object namespace
		return fmt.Sprintf(
			"%s/%s",
			object.GetNamespace(),
			object.GetName(),
		)
	}
	// just object name
	return object.GetName()
}

// namespaceNameOrName returns the name of the resource based on its
// scope
func namespaceNameOrName(obj *unstructured.Unstructured) string {
	if obj.GetNamespace() != "" {
		return fmt.Sprintf(
			"%s/%s", obj.GetNamespace(), obj.GetName(),
		)
	}
	return obj.GetName()
}

func namespaceNameOrNameFromMeta(obj metav1.Object) string {
	if obj.GetNamespace() != "" {
		// with namespace
		return fmt.Sprintf(
			"%s/%s",
			obj.GetNamespace(),
			obj.GetName(),
		)
	}
	// without namespace
	return obj.GetName()
}

// describeObject returns a human-readable string to identify a
// given object.
func describeObject(obj *unstructured.Unstructured) string {
	if ns := obj.GetNamespace(); ns != "" {
		// with namespace
		return fmt.Sprintf(
			"%s/%s of kind %s",
			ns,
			obj.GetName(),
			obj.GetKind(),
		)
	}
	// without namespace
	return fmt.Sprintf(
		"%s of kind %s",
		obj.GetName(),
		obj.GetKind(),
	)
}

// sanitiseAPIVersion will make the apiVersion suitable to be used
// as value field in labels or annotations
func sanitiseAPIVersion(version string) string {
	return strings.ReplaceAll(version, "/", "-")
}

// DescObjAsSanitisedNSName will return the sanitised namespace name
// format corresponding to the given object
func DescObjAsSanitisedNSName(obj *unstructured.Unstructured) string {
	return strings.ReplaceAll(namespaceNameOrName(obj), "/", "-")
}

// DescMetaAsSanitisedNSName returns the sanitised namespace name
// format corresponding to the given meta object
func DescMetaAsSanitisedNSName(obj metav1.Object) string {
	return strings.ReplaceAll(namespaceNameOrNameFromMeta(obj), "/", "-")
}

// DescObjectAsKey returns a machine readable string of the provided
// object. It can be used to identify the given object.
func DescObjectAsKey(obj *unstructured.Unstructured) string {
	ns := obj.GetNamespace()
	if ns != "" {
		// with namespace
		return fmt.Sprintf(
			"%s:%s:%s:%s",
			obj.GetAPIVersion(),
			obj.GetKind(),
			ns,
			obj.GetName(),
		)
	}
	// without namespace
	return fmt.Sprintf(
		"%s:%s:%s",
		obj.GetAPIVersion(),
		obj.GetKind(),
		obj.GetName(),
	)
}

// DescObjectAsSanitisedKey returns a sanitised name from the
// given object that can be used in annotation as key or value
func DescObjectAsSanitisedKey(obj *unstructured.Unstructured) string {
	ver := sanitiseAPIVersion(obj.GetAPIVersion())
	ns := obj.GetNamespace()
	if ns != "" {
		// with namespace
		return fmt.Sprintf(
			"%s-%s-%s-%s",
			ver,
			obj.GetKind(),
			ns,
			obj.GetName(),
		)
	}
	// without namespace
	return fmt.Sprintf(
		"%s-%s-%s",
		ver,
		obj.GetKind(),
		obj.GetName(),
	)
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

// MakeAnyUnstructRegistryByReference builds the registry of provided
// objects.
//
// This registry is suitable for use in the **children** field of a
// CompositeController or **attachments** field of DecoratorController or
// GenericController.
//
// This function returns a registry which is a map of maps. The outer most
// map is keyed using the object i.e. child's api version & kind and then
// the inner map is keyed using the object's name. However, if the parent
// resource is clustered and the child resource is namespaced the inner
// map's key is the namespace & name of the child resource.
func MakeAnyUnstructRegistryByReference(
	reference metav1.Object,
	objects []*unstructured.Unstructured,
) AnyUnstructRegistry {
	registry := make(AnyUnstructRegistry)
	for _, obj := range objects {
		registry.InsertByReference(reference, obj)
	}
	return registry
}

// MakeAnyUnstructRegistry builds the registry of provided objects.
//
// This registry is suitable for use in the **children** field of a
// CompositeController or **attachments** field of DecoratorController or
// GenericController.
//
// This function returns a registry which is a map of maps. The outer most
// map is keyed using the object i.e. child's api version & kind and then
// the inner map is keyed using the object's namespace & name.
func MakeAnyUnstructRegistry(objects []*unstructured.Unstructured) AnyUnstructRegistry {
	registry := make(AnyUnstructRegistry)
	for _, obj := range objects {
		registry.Insert(obj)
	}
	return registry
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

// ResourceRegistrar is the registrar of server api resources
// anchored by **group** and **kind**
type ResourceRegistrar map[string]*dynamicdiscovery.APIResource

// Set registers the given API resource against the registrar based
// on the given group and kind
func (m ResourceRegistrar) Set(
	apiGroup string,
	kind string,
	resource *dynamicdiscovery.APIResource,
) {
	m[makeKeyFromAPIGroupKind(apiGroup, kind)] = resource
}

// Get returns the API resource from the registrar based on the
// given group and kind
func (m ResourceRegistrar) Get(
	apiGroup string,
	kind string,
) *dynamicdiscovery.APIResource {
	return m[makeKeyFromAPIGroupKind(apiGroup, kind)]
}

// makeKeyFromAPIGroupKind returns the string format of the given group
// and kind
func makeKeyFromAPIGroupKind(apiGroup, kind string) string {
	return fmt.Sprintf("%s.%s", kind, apiGroup)
}

// ResourceInformerRegistrar acts as the registrar of Informer instances
// anchored by apiversion and resource name (i.e. plural format of kind)
type ResourceInformerRegistrar map[string]*dynamicinformer.ResourceInformer

// Set registers the given informer by mapping it against the
// given version and resource
func (m ResourceInformerRegistrar) Set(
	apiVersion string,
	resource string,
	informer *dynamicinformer.ResourceInformer,
) {
	m[makeKeyFromAPIVersionResource(apiVersion, resource)] = informer
}

// Get returns the informer corresponding to given version and resource
func (m ResourceInformerRegistrar) Get(
	apiVersion string,
	resource string,
) *dynamicinformer.ResourceInformer {
	return m[makeKeyFromAPIVersionResource(apiVersion, resource)]
}

// build the string key from given apiVersion and resource
func makeKeyFromAPIVersionResource(apiVersion, resource string) string {
	return fmt.Sprintf("%s.%s", resource, apiVersion)
}
