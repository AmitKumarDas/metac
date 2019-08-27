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

package controllerref

import (
	"github.com/golang/glog"
	"github.com/pkg/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	dynamicclientset "openebs.io/metac/dynamic/clientset"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// UnstructClaimManager manages children of a watched resource
// by either adopting the children or releasing these children
//
// This instance provides the logic of `how to adopt` or `how to
// release`. `When to adopt` & `when to release` is implemented at
// the embedded ClaimManager.
type UnstructClaimManager struct {
	k8s.ClaimManager

	// kind of attachment resources that needs to be adopted
	// or released
	attachmentKind schema.GroupVersionKind

	// dynamic client associated with attachment resource kind
	attachmentClient *dynamicclientset.ResourceClient
}

// NewUnstructClaimManager returns a new instance of UnstructClaimManager
func NewUnstructClaimManager(
	attachmentClient *dynamicclientset.ResourceClient,
	watched metav1.Object,
	selector labels.Selector,
	watchedKind,
	attachmentKind schema.GroupVersionKind,
	canAdopt func() error,
) *UnstructClaimManager {
	return &UnstructClaimManager{
		ClaimManager: k8s.ClaimManager{
			Watched:      watched,
			WatchedKind:  watchedKind,
			Selector:     selector,
			CanAdoptFunc: canAdopt,
		},
		attachmentKind:   attachmentKind,
		attachmentClient: attachmentClient,
	}
}

// String implements Stringer interface
func (m *UnstructClaimManager) String() string {
	return m.ClaimManager.String()
}

// BulkClaim claims the provided list of attachments against
// this manager instance by either adopting or releasing each
// attachment based on match or nomatch w.r.t the watched
// resource.
func (m *UnstructClaimManager) BulkClaim(
	attachments []*unstructured.Unstructured,
) ([]*unstructured.Unstructured, error) {
	var claimed []*unstructured.Unstructured
	var errlist []error

	match := func(attachment metav1.Object) bool {
		return m.Selector.Matches(labels.Set(attachment.GetLabels()))
	}
	adopt := func(attachment metav1.Object) error {
		return m.adopt(attachment.(*unstructured.Unstructured))
	}
	release := func(attachment metav1.Object) error {
		return m.release(attachment.(*unstructured.Unstructured))
	}

	for _, attachment := range attachments {
		ok, err := m.Claim(attachment, match, adopt, release)
		if err != nil {
			errlist = append(errlist, err)
			continue
		}
		if ok {
			claimed = append(claimed, attachment)
		}
	}
	return claimed, utilerrors.NewAggregate(errlist)
}

// adopt is the logic to adopt the provided attachment to the
// watched resource of this instance
func (m *UnstructClaimManager) adopt(attachment *unstructured.Unstructured) error {
	if err := m.CanAdopt(); err != nil {
		return errors.Wrapf(
			err,
			"%s: Failed to adopt child %s/%s (%v)",
			m,
			attachment.GetNamespace(),
			attachment.GetName(),
			attachment.GetUID(),
		)
	}
	glog.Infof(
		"%s %s/%s: adopting %s/%s (%v)",
		m,
		m.Watched.GetNamespace(),
		m.Watched.GetName(),
		attachment.GetNamespace(),
		attachment.GetName(),
		attachment.GetUID(),
	)
	controllerRef := metav1.OwnerReference{
		APIVersion:         m.WatchedKind.GroupVersion().String(),
		Kind:               m.WatchedKind.Kind,
		Name:               m.Watched.GetName(),
		UID:                m.Watched.GetUID(),
		Controller:         k8s.BoolPtr(true),
		BlockOwnerDeletion: k8s.BoolPtr(true),
	}
	return atomicUpdate(m.attachmentClient, attachment, func(obj *unstructured.Unstructured) bool {
		ownerRefs := addOwnerReference(obj.GetOwnerReferences(), controllerRef)
		obj.SetOwnerReferences(ownerRefs)
		return true
	})
}

// release is the logic to release the provided attachment to the
// watched resource of this instance
func (m *UnstructClaimManager) release(attachment *unstructured.Unstructured) error {
	glog.Infof(
		"%s %s/%s: releasing %s/%s",
		m,
		m.Watched.GetNamespace(),
		m.Watched.GetName(),
		attachment.GetNamespace(),
		attachment.GetName(),
	)
	err := atomicUpdate(m.attachmentClient, attachment, func(obj *unstructured.Unstructured) bool {
		ownerRefs := removeOwnerReference(obj.GetOwnerReferences(), m.Watched.GetUID())
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

func atomicUpdate(
	resourceClient *dynamicclientset.ResourceClient,
	resource *unstructured.Unstructured,
	updateFunc func(obj *unstructured.Unstructured) bool,
) error {
	// We can't use strategic merge patch because we want this to work with
	// custom resources.
	// We can't use merge patch because that would replace the whole list.
	// We can't use JSON patch ops because that wouldn't be idempotent.
	// The only option is GET/PUT with ResourceVersion.
	_, err := resourceClient.Namespace(resource.GetNamespace()).AtomicUpdate(
		resource, updateFunc,
	)
	return err
}
