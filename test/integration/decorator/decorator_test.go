/*
Copyright 2019 Google Inc.
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

package decorator

import (
	"testing"
	"time"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog"

	"openebs.io/metac/controller/decorator"
	"openebs.io/metac/test/integration/framework"
)

// TestSyncWebhook tests that the sync webhook triggers and passes the
// request/response properly.
func TestSyncWebhook(t *testing.T) {
	testName := "dctl-test-sync-webhook"
	labels := map[string]string{
		"test":                  "test",
		"metac/controller-name": "decorator",
		"metac/resource-type":   "customresource",
		"metac/test-category":   "sync-webhook",
	}

	f := framework.NewFixture(t)
	defer f.TearDown()

	f.CreateNamespace(testName)

	// Setup  namespace scoped CRDs
	parentCRD, parentClient := f.SetupCRD(
		"DCtlSyncParent",
		apiextensions.NamespaceScoped,
	)
	childCRD, childClient := f.SetupCRD(
		"DCtlSyncChild",
		apiextensions.NamespaceScoped,
	)

	// define the "reconcile logic" i.e. sync hook logic here
	hook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := decorator.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		// As a simple test of request/response content,
		// just build a child with the same name as the parent.
		//
		// Note that this does not create the child in kubernetes.
		// Creation of child in kubernetes is done by decorator
		// controller on creation of parent resource.
		childResource := framework.BuildUnstructObjFromCRD(
			childCRD, req.Object.GetName(),
		)
		childResource.SetLabels(labels)

		resp := decorator.SyncHookResponse{
			Attachments: []*unstructured.Unstructured{childResource},
		}
		return json.Marshal(resp)
	})

	f.CreateDecoratorController(
		testName,
		hook.URL,
		framework.BuildResourceRuleFromCRD(parentCRD),
		framework.BuildResourceRuleFromCRD(childCRD),
	)

	parentResource := framework.BuildUnstructObjFromCRD(parentCRD, testName)
	unstructured.SetNestedStringMap(
		parentResource.Object,
		labels,
		"spec",
		"selector",
		"matchLabels",
	)

	klog.Infof(
		"Creating %s %s/%s",
		parentResource.GetKind(),
		parentResource.GetNamespace(),
		parentResource.GetName(),
	)
	_, err := parentClient.
		Namespace(testName).
		Create(
			parentResource,
			metav1.CreateOptions{},
		)
	if err != nil {
		t.Fatal(err)
	}
	klog.Infof(
		"Created %s %s/%s",
		parentResource.GetKind(),
		parentResource.GetNamespace(),
		parentResource.GetName(),
	)

	klog.Infof("Waiting for child sync")
	err = f.Wait(func() (bool, error) {
		_, err =
			childClient.Namespace(testName).Get(testName, metav1.GetOptions{})
		return err == nil, err
	})
	if err != nil {
		t.Errorf("Child sync failed: %v", err)
	}
	klog.Infof("Child sync was successful")
}

// TestResyncAfter tests that the resyncAfterSeconds field works.
func TestResyncAfter(t *testing.T) {
	testName := "dctl-test-resync-after"
	labels := map[string]string{
		"test":                  "test",
		"metac/controller-name": "decorator",
		"metac/resource-type":   "customresource",
		"metac/test-category":   "resync-after",
	}

	f := framework.NewFixture(t)
	defer f.TearDown()

	f.CreateNamespace(testName)
	parentCRD, parentClient := f.SetupCRD(
		"DCtlResyncAfterParent",
		apiextensions.NamespaceScoped,
	)

	var lastSync time.Time
	done := false

	// write the reconcile logic i.e. webhook sync logic here
	hook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := decorator.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		resp := decorator.SyncHookResponse{}
		if req.Object.Object["status"] == nil {
			// If status hasn't been set yet, set it. This is the
			// "zeroth" sync. Metacontroller will set our status
			// and then the object should quiesce.
			resp.Status = map[string]interface{}{}
		} else if lastSync.IsZero() {
			// This should be the final sync before quiescing. Do
			// nothing except request a resync. Other than our
			// resyncAfter request, there should be nothing that
			// causes our object to get resynced.
			lastSync = time.Now()
			resp.ResyncAfterSeconds = 0.1
		} else if !done {
			done = true
			// This is the second sync. Report how much time elapsed.
			resp.Status = map[string]interface{}{
				"elapsedSeconds": time.Since(lastSync).Seconds(),
			}
		} else {
			// If we're done, just freeze the status.
			resp.Status = req.Object.Object["status"].(map[string]interface{})
		}
		return json.Marshal(resp)
	})

	f.CreateDecoratorController(
		testName,
		hook.URL,
		framework.BuildResourceRuleFromCRD(parentCRD),
		nil,
	)

	parentResource := framework.BuildUnstructObjFromCRD(parentCRD, testName)
	unstructured.SetNestedStringMap(
		parentResource.Object,
		labels,
		"spec",
		"selector", "matchLabels",
	)

	klog.Infof(
		"Creating %s %s/%s",
		parentResource.GetKind(),
		parentResource.GetNamespace(),
		parentResource.GetName(),
	)
	_, err :=
		parentClient.Namespace(testName).Create(parentResource, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	klog.Infof(
		"Created %s %s/%s",
		parentResource.GetKind(),
		parentResource.GetNamespace(),
		parentResource.GetName(),
	)

	klog.Infof("Waiting for status.elapsedSeconds to be reported")
	var elapsedSeconds float64
	err = f.Wait(func() (bool, error) {
		parentResource, err :=
			parentClient.Namespace(testName).Get(testName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		val, found, err :=
			unstructured.NestedFloat64(parentResource.Object, "status", "elapsedSeconds")
		if err != nil || !found {
			// The value hasn't been populated. Keep waiting.
			return false, err
		}

		elapsedSeconds = val
		return true, nil
	})
	if err != nil {
		t.Fatalf("Didn't find status.elapsedSeconds: %v", err)
	}
	klog.Infof("status.elapsedSeconds is %v", elapsedSeconds)

	if elapsedSeconds > 1.0 {
		t.Errorf(
			"Requested resyncAfter did not occur in time; elapsedSeconds: %v",
			elapsedSeconds,
		)
	}
}
