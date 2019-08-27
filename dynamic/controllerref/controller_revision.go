/*
Copyright 2018 Google Inc.
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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	client "openebs.io/metac/client/generated/clientset/versioned/typed/metacontroller/v1alpha1"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// ControllerRevisionClaimManager claims controller revision
// resources against the watched resource
type ControllerRevisionClaimManager struct {
	k8s.ClaimManager

	// dynamic client for controller revision resource
	ctrlRevisionClient client.ControllerRevisionInterface
}

// NewControllerRevisionClaimManager returns a new instance of
// ControllerRevisionClaimManager
func NewControllerRevisionClaimManager(
	ctrlRevisionClient client.ControllerRevisionInterface,
	watched metav1.Object,
	selector labels.Selector,
	watchedKind schema.GroupVersionKind,
	canAdopt func() error,
) *ControllerRevisionClaimManager {
	return &ControllerRevisionClaimManager{
		ClaimManager: k8s.ClaimManager{
			Watched:      watched,
			WatchedKind:  watchedKind,
			Selector:     selector,
			CanAdoptFunc: canAdopt,
		},
		ctrlRevisionClient: ctrlRevisionClient,
	}
}

// String implements Stringer interface
func (m *ControllerRevisionClaimManager) String() string {
	return m.ClaimManager.String()
}

// BulkClaim either adopts or releases controller
// revision instances against the watched resource based on
// match or no match
func (m *ControllerRevisionClaimManager) BulkClaim(
	ctrlRevisions []*v1alpha1.ControllerRevision,
) ([]*v1alpha1.ControllerRevision, error) {
	var claimed []*v1alpha1.ControllerRevision
	var errlist []error

	match := func(ctrlRevision metav1.Object) bool {
		return m.Selector.Matches(labels.Set(ctrlRevision.GetLabels()))
	}
	adopt := func(ctrlRevision metav1.Object) error {
		return m.adopt(ctrlRevision.(*v1alpha1.ControllerRevision))
	}
	release := func(ctrlRevision metav1.Object) error {
		return m.release(ctrlRevision.(*v1alpha1.ControllerRevision))
	}

	for _, revision := range ctrlRevisions {
		ok, err := m.Claim(revision, match, adopt, release)
		if err != nil {
			errlist = append(errlist, err)
			continue
		}
		if ok {
			claimed = append(claimed, revision)
		}
	}
	return claimed, utilerrors.NewAggregate(errlist)
}

func (m *ControllerRevisionClaimManager) adopt(
	revision *v1alpha1.ControllerRevision,
) error {
	if err := m.CanAdopt(); err != nil {
		return errors.Wrapf(
			err,
			"%s: Failed to adopt ControllerRevision %s/%s (%v)",
			m,
			revision.GetNamespace(),
			revision.GetName(),
			revision.GetUID(),
		)
	}
	glog.Infof(
		"%s: adopting ControllerRevision %s/%s",
		m,
		revision.GetNamespace(),
		revision.GetName(),
	)
	controllerRef := metav1.OwnerReference{
		APIVersion:         m.WatchedKind.GroupVersion().String(),
		Kind:               m.WatchedKind.Kind,
		Name:               m.Watched.GetName(),
		UID:                m.Watched.GetUID(),
		Controller:         k8s.BoolPtr(true),
		BlockOwnerDeletion: k8s.BoolPtr(true),
	}

	// We can't use strategic merge patch because we want this to work
	// with custom resources.
	// We can't use merge patch because that would replace the whole list.
	// We can't use JSON patch ops because that wouldn't be idempotent.
	// The only option is GET/PUT with ResourceVersion.
	_, err := m.updateWithRetries(revision, func(obj *v1alpha1.ControllerRevision) bool {
		ownerRefs := addOwnerReference(obj.GetOwnerReferences(), controllerRef)
		obj.SetOwnerReferences(ownerRefs)
		return true
	})
	return err
}

func (m *ControllerRevisionClaimManager) release(
	revision *v1alpha1.ControllerRevision,
) error {
	glog.Infof(
		"%s: releasing ControllerRevision %s/%s",
		m,
		revision.GetNamespace(),
		revision.GetName(),
	)
	_, err := m.updateWithRetries(revision, func(obj *v1alpha1.ControllerRevision) bool {
		ownerRefs := removeOwnerReference(obj.GetOwnerReferences(), m.Watched.GetUID())
		obj.SetOwnerReferences(ownerRefs)
		return true
	})
	if apierrors.IsNotFound(err) || apierrors.IsGone(err) {
		// If the original object is gone, that's fine
		// because we're giving up on this child anyway.
		return nil
	}
	return err
}

func (m *ControllerRevisionClaimManager) updateWithRetries(
	orig *v1alpha1.ControllerRevision,
	update func(obj *v1alpha1.ControllerRevision) bool,
) (result *v1alpha1.ControllerRevision, err error) {
	name := orig.GetName()

	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current, err := m.ctrlRevisionClient.Get(name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if current.GetUID() != orig.GetUID() {
			// The original object was deleted and replaced with a new one.
			gk := schema.
				FromAPIVersionAndKind(orig.APIVersion, orig.Kind).GroupKind()
			return apierrors.NewNotFound(
				schema.GroupResource{
					Group:    gk.Group,
					Resource: gk.Kind,
				}, name)
		}
		if changed := update(current); !changed {
			// There's nothing to do.
			result = current
			return nil
		}
		return err
	})
	return result, err
}
