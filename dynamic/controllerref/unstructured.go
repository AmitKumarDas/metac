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

package controllerref

import (
	"fmt"

	"github.com/golang/glog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	dynamicclientset "openebs.io/metac/dynamic/clientset"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// UnstructuredManager manages children of a parent resource
// by either adopting the children or releasing these children
type UnstructuredManager struct {
	k8s.BaseControllerRefManager
	parentKind schema.GroupVersionKind
	childKind  schema.GroupVersionKind
	client     *dynamicclientset.ResourceClient
}

// NewUnstructuredManager returns a new instance of UnstructuredManager
func NewUnstructuredManager(
	client *dynamicclientset.ResourceClient,
	parent metav1.Object,
	selector labels.Selector,
	parentKind,
	childKind schema.GroupVersionKind,
	canAdopt func() error,
) *UnstructuredManager {
	return &UnstructuredManager{
		BaseControllerRefManager: k8s.BaseControllerRefManager{
			Controller:   parent,
			Selector:     selector,
			CanAdoptFunc: canAdopt,
		},
		parentKind: parentKind,
		childKind:  childKind,
		client:     client,
	}
}

// ClaimChildren manages children of this manager instance by
// either adopting or releasing a child based on match or
// nomatch against the provided children instances
func (m *UnstructuredManager) ClaimChildren(
	children []*unstructured.Unstructured,
) ([]*unstructured.Unstructured, error) {
	var claimed []*unstructured.Unstructured
	var errlist []error

	match := func(obj metav1.Object) bool {
		return m.Selector.Matches(labels.Set(obj.GetLabels()))
	}
	adopt := func(obj metav1.Object) error {
		return m.adoptChild(obj.(*unstructured.Unstructured))
	}
	release := func(obj metav1.Object) error {
		return m.releaseChild(obj.(*unstructured.Unstructured))
	}

	for _, child := range children {
		ok, err := m.ClaimObject(child, match, adopt, release)
		if err != nil {
			errlist = append(errlist, err)
			continue
		}
		if ok {
			claimed = append(claimed, child)
		}
	}
	return claimed, utilerrors.NewAggregate(errlist)
}

func atomicUpdate(
	rc *dynamicclientset.ResourceClient,
	obj *unstructured.Unstructured,
	updateFunc func(obj *unstructured.Unstructured) bool,
) error {
	// We can't use strategic merge patch because we want this to work with custom resources.
	// We can't use merge patch because that would replace the whole list.
	// We can't use JSON patch ops because that wouldn't be idempotent.
	// The only option is GET/PUT with ResourceVersion.
	_, err := rc.Namespace(obj.GetNamespace()).AtomicUpdate(obj, updateFunc)
	return err
}

func (m *UnstructuredManager) adoptChild(obj *unstructured.Unstructured) error {
	if err := m.CanAdopt(); err != nil {
		return fmt.Errorf(
			"Failed to adopt child %v %v/%v (%v): %v",
			m.childKind.Kind,
			obj.GetNamespace(),
			obj.GetName(),
			obj.GetUID(),
			err,
		)
	}
	glog.Infof(
		"%v %v/%v: adopting %v %v",
		m.parentKind.Kind,
		m.Controller.GetNamespace(),
		m.Controller.GetName(),
		m.childKind.Kind,
		obj.GetName(),
	)
	controllerRef := metav1.OwnerReference{
		APIVersion:         m.parentKind.GroupVersion().String(),
		Kind:               m.parentKind.Kind,
		Name:               m.Controller.GetName(),
		UID:                m.Controller.GetUID(),
		Controller:         k8s.BoolPtr(true),
		BlockOwnerDeletion: k8s.BoolPtr(true),
	}
	return atomicUpdate(m.client, obj, func(obj *unstructured.Unstructured) bool {
		ownerRefs := addOwnerReference(obj.GetOwnerReferences(), controllerRef)
		obj.SetOwnerReferences(ownerRefs)
		return true
	})
}

func (m *UnstructuredManager) releaseChild(obj *unstructured.Unstructured) error {
	glog.Infof(
		"%v %v/%v: releasing %v %v",
		m.parentKind.Kind,
		m.Controller.GetNamespace(),
		m.Controller.GetName(),
		m.childKind.Kind,
		obj.GetName(),
	)
	err := atomicUpdate(m.client, obj, func(obj *unstructured.Unstructured) bool {
		ownerRefs := removeOwnerReference(obj.GetOwnerReferences(), m.Controller.GetUID())
		obj.SetOwnerReferences(ownerRefs)
		return true
	})
	if apierrors.IsNotFound(err) || apierrors.IsGone(err) {
		// If the original object is gone, that's fine
		// because we're giving up on this child anyway
		return nil
	}
	return err
}
