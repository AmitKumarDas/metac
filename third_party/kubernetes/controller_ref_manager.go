/*
Copyright 2016 The Kubernetes Authors.
Copyright 2019 The MayaData Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubernetes

// This is copied from k8s.io/kubernetes to avoid a dependency on all of Kubernetes.
// TODO(enisoc): Move the upstream code to somewhere better.

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ClaimManager adopts orphans or releases owned
// resources. These owned or orphaned resources is
// referred to as attachments.
//
// NOTE:
//	Claiming resources is part of the overall reconciliation
// process to get to the desired state of the system.
type ClaimManager struct {
	// resource that is watched to have its attachments
	// claimed
	Watched metav1.Object

	// kind of the resource that is being watched
	WatchedKind schema.GroupVersionKind

	// selector to be used to filter the attachments that
	// belong / should belong to the watched resource
	Selector labels.Selector

	canAdoptErr  error
	canAdoptOnce sync.Once
	CanAdoptFunc func() error
}

func (m *ClaimManager) String() string {
	if m.Watched == nil {
		return fmt.Sprintf("ClaimManager %s", m.WatchedKind.Kind)
	}
	return fmt.Sprintf(
		"ClaimManager %s %s/%s",
		m.WatchedKind.Kind,
		m.Watched.GetNamespace(),
		m.Watched.GetName(),
	)
}

// CanAdopt runs this instance's adopt function and returns
// the corresponding error if any
func (m *ClaimManager) CanAdopt() error {
	m.canAdoptOnce.Do(func() {
		if m.CanAdoptFunc != nil {
			m.canAdoptErr = m.CanAdoptFunc()
		}
	})
	return m.canAdoptErr
}

// Claim tries to take ownership of an object for this watched resource.
//
// It will reconcile the following:
//   * Adopt orphans if the match function returns true.
//   * Release owned objects if the match function returns false.
//
// A non-nil error is returned if some form of reconciliation was attempted and
// failed. Usually, controllers should try again later in case reconciliation
// is still needed.
//
// If the error is nil, either the reconciliation succeeded, or no
// reconciliation was necessary. The returned boolean indicates whether you now
// own the object.
//
// No reconciliation will be attempted if the watched resource is being deleted.
func (m *ClaimManager) Claim(
	obj metav1.Object,
	match func(metav1.Object) bool,
	adopt, release func(metav1.Object) error,
) (bool, error) {
	controllerRef := metav1.GetControllerOf(obj)
	if controllerRef != nil {
		if controllerRef.UID != m.Watched.GetUID() {
			// Owned by someone else. Ignore.
			return false, nil
		}
		if match(obj) {
			// We already own it and the selector matches.
			// Return true (successfully claimed) before checking
			// deletion timestamp. We're still allowed to claim things
			// we already own while being deleted because doing so
			// requires taking no actions.
			return true, nil
		}
		// Owned by us but selector doesn't match.
		// Try to release, unless we're being deleted.
		if m.Watched.GetDeletionTimestamp() != nil {
			return false, nil
		}
		if err := release(obj); err != nil {
			// If the object no longer exists, ignore the error.
			if kerrors.IsNotFound(err) {
				return false, nil
			}
			// Either someone else released it, or there was a transient error.
			// The controller should requeue and try again if it's still stale.
			return false, err
		}
		// Successfully released.
		return false, nil
	}

	// It's an orphan.
	if m.Watched.GetDeletionTimestamp() != nil || !match(obj) {
		// Ignore if we're being deleted or selector doesn't match.
		return false, nil
	}
	if obj.GetDeletionTimestamp() != nil {
		// Ignore if the object is being deleted
		return false, nil
	}
	// Selector matches. Try to adopt.
	if err := adopt(obj); err != nil {
		// If the object no longer exists, ignore the error.
		if kerrors.IsNotFound(err) {
			return false, nil
		}
		// Either someone else claimed it first, or there was a transient error.
		// The controller should requeue and try again if it's still orphaned.
		return false, err
	}
	// Successfully adopted.
	return true, nil
}

// ErrorOnDeletionTimestamp returns a function that errors if
// provided object has a deletion timestamp on it
func ErrorOnDeletionTimestamp(getObject func() (metav1.Object, error)) func() error {
	return func() error {
		obj, err := getObject()
		if err != nil {
			return err
		}
		if obj.GetDeletionTimestamp() != nil {
			return errors.Errorf(
				"%s/%s has just been deleted at %s",
				obj.GetNamespace(),
				obj.GetName(),
				obj.GetDeletionTimestamp(),
			)
		}
		return nil
	}
}
