/*
Copyright 2016 The Kubernetes Authors.
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
	"time"

	"github.com/golang/glog"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
)

// WaitForCacheSync is a wrapper around cache.WaitForCacheSync that
// generates log messages indicating that the controller identified
// by controllerName is waiting for syncs, followed by either a
// successful or failed sync.
func WaitForCacheSync(
	controllerName string,
	stopCh <-chan struct{},
	cacheSyncs ...cache.InformerSynced,
) bool {
	glog.Infof("Waiting for caches to sync for controller %q", controllerName)

	if !cache.WaitForCacheSync(stopCh, cacheSyncs...) {
		utilruntime.HandleError(fmt.Errorf(
			"Unable to sync caches for controller %q", controllerName,
		))
		return false
	}

	glog.Infof("Caches are synced for controller %q", controllerName)
	return true
}

// WaitForCacheSyncFn is a typed function that adheres to
// cache.WaitForCacheSync signature
type WaitForCacheSyncFn func(
	stop <-chan struct{}, isSyncFns ...cache.InformerSynced,
) bool

// CacheSyncTimeTaken is a decorator around WaitForCacheSyncFn
// that logs the time taken for all caches to sync for the
// given controller
func CacheSyncTimeTaken(
	controllerName string,
	fn WaitForCacheSyncFn,
) WaitForCacheSyncFn {
	return func(stop <-chan struct{}, isSyncFns ...cache.InformerSynced) bool {
		start := time.Now()
		defer glog.Infof(
			"Controller %s cache sync took %s",
			controllerName,
			time.Now().Sub(start),
		)

		return fn(stop, isSyncFns...)
	}
}

// CacheSyncFailureAsError is a decorator around WaitForCacheSyncFn
// that logs an error if all caches could not be sync-ed for the
// given controller
func CacheSyncFailureAsError(
	controllerName string,
	fn WaitForCacheSyncFn,
) WaitForCacheSyncFn {
	return func(stop <-chan struct{}, isSyncFns ...cache.InformerSynced) bool {
		synced := fn(stop, isSyncFns...)
		if !synced {
			utilruntime.HandleError(fmt.Errorf(
				"Unable to sync caches for controller %s", controllerName,
			))
		}
		return synced
	}
}
