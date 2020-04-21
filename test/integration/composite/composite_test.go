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

package composite

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/klog"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/composite"
	"openebs.io/metac/test/integration/framework"
)

// TestSyncWebhook tests that the sync webhook triggers and passes the
// request/response properly.
func TestSyncWebhook(t *testing.T) {
	testName := "cctl-test-sync-webhook"
	labels := map[string]string{
		"test":                  "test",
		"metac/controller-name": "composite",
		"metac/resource-type":   "customresource",
		"metac/test-category":   "sync-webhook",
	}

	// fixture provides the common test logic including a way to
	// invoke teardown after completion of this particular test
	f := framework.NewFixture(t)
	defer f.TearDown()

	f.CreateNamespace(testName)
	parentCRD, parentClient := f.SetupCRD(
		"CCtlSyncParent", apiextensions.NamespaceScoped,
	)
	childCRD, childClient := f.SetupCRD(
		"CCtlSyncChild", apiextensions.NamespaceScoped,
	)

	// define the "reconcile logic" i.e. sync hook logic here
	hook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := composite.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		// As a simple test of request/response content,
		// just build a child with the same name as the parent.
		//
		// Note that this does not create the child in kubernetes.
		// Creation of child in kubernetes is done by composite
		// controller on creation of parent resource.
		child := framework.BuildUnstructObjFromCRD(childCRD, req.Parent.GetName())
		child.SetLabels(labels)
		resp := composite.SyncHookResponse{
			Children: []*unstructured.Unstructured{child},
		}
		return json.Marshal(resp)
	})

	f.CreateCompositeController(
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
		_, err := childClient.
			Namespace(testName).
			Get(
				testName,
				metav1.GetOptions{},
			)
		return err == nil, err
	})
	if err != nil {
		t.Errorf("Child sync failed: %v", err)
	}
	klog.Infof("Child sync was successful")
}

// TestCascadingDelete tests that we request cascading deletion of children,
// even if the server-side default for that child type is non-cascading.
func TestCacadingDelete(t *testing.T) {
	testName := "cctl-test-cascading-delete"
	labels := map[string]string{
		"test":                  "test",
		"metac/controller-name": "composite",
		"metac/resource-type":   "customresource-job",
		"metac/test-category":   "cascading-delete",
	}

	f := framework.NewFixture(t)
	defer f.TearDown()

	f.CreateNamespace(testName)
	parentCRD, parentClient := f.SetupCRD(
		"CCtlCascadingDeleteParent",
		apiextensions.NamespaceScoped,
	)
	jobChildClient := f.GetTypedClientset().BatchV1().Jobs(testName)

	// define the "reconcile logic" i.e. sync hook logic here
	hook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := composite.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		resp := composite.SyncHookResponse{}
		replicas, _, _ := unstructured.NestedInt64(
			req.Parent.Object,
			"spec",
			"replicas",
		)
		if replicas > 0 {
			// Create a child batch/v1 Job if requested.
			// For backward compatibility, the server-side default on that API is
			// non-cascading deletion (don't delete Pods).
			// So we can use this as a test case for whether we are correctly requesting
			// cascading deletion.
			child := framework.BuildUnstructuredObjFromJSON(
				"batch/v1",
				"Job",
				testName,
				`{
					"spec": {
						"template": {
							"spec": {
								"restartPolicy": "Never",
								"containers": [
									{
										"name": "pi",
										"image": "perl"
									}
								]
							}
						}
					}
				}`,
			)
			child.SetLabels(labels)
			resp.Children = append(resp.Children, child)
		}
		return json.Marshal(resp)
	})

	f.CreateCompositeController(
		testName,
		hook.URL,
		framework.BuildResourceRuleFromCRD(parentCRD),
		&v1alpha1.ResourceRule{
			APIVersion: "batch/v1",
			Resource:   "jobs",
		},
	)

	parentResource := framework.BuildUnstructObjFromCRD(parentCRD, testName)
	unstructured.SetNestedStringMap(
		parentResource.Object,
		labels,
		"spec",
		"selector",
		"matchLabels",
	)
	unstructured.SetNestedField(
		parentResource.Object,
		int64(1),
		"spec",
		"replicas",
	)

	klog.Infof(
		"Creating %s %s/%s",
		parentResource.GetKind(),
		parentResource.GetNamespace(),
		parentResource.GetName(),
	)
	var err error
	parentResource, err = parentClient.
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

	klog.Infof("Waiting for child job creation")
	err = f.Wait(func() (bool, error) {
		_, err := jobChildClient.Get(testName, metav1.GetOptions{})
		return err == nil, err
	})
	if err != nil {
		t.Errorf("Child job create failed: %v", err)
	}
	klog.Infof("Child job was created successfully")

	// Now that child exists, tell parent to delete it.
	klog.Infof("Updating parent with replicas=0")
	_, err =
		parentClient.Namespace(testName).AtomicUpdate(parentResource, func(obj *unstructured.Unstructured) bool {
			unstructured.SetNestedField(obj.Object, int64(0), "spec", "replicas")
			return true
		})
	if err != nil {
		t.Fatal(err)
	}
	klog.Infof("Updated parent with replicas=0")

	// Make sure the child gets actually deleted, which means no GC finalizers got
	// added to it. Note that we don't actually run the GC in this integration
	// test env, so we don't need to worry about the GC racing us to process the
	// finalizers.
	klog.Infof("Waiting for child job to be deleted")
	var child *batchv1.Job
	err = f.Wait(func() (bool, error) {
		var getErr error
		child, getErr = jobChildClient.Get(testName, metav1.GetOptions{})
		return apierrors.IsNotFound(getErr), nil
	})
	if err != nil {
		out, _ := json.Marshal(child)
		t.Errorf("Child job delete failed: %v; object: %s", err, out)
	}
	klog.Infof("Child job deleted successfully")
}

// TestResyncAfter tests that the resyncAfterSeconds field works.
func TestResyncAfter(t *testing.T) {
	testName := "cctl-test-resync-after"
	labels := map[string]string{
		"test":                  "test",
		"metac/controller-name": "composite",
		"metac/resource-type":   "customresource-job",
		"metac/test-category":   "resync-after",
	}

	f := framework.NewFixture(t)
	defer f.TearDown()

	f.CreateNamespace(testName)
	parentCRD, parentClient := f.SetupCRD(
		"CCtlResyncAfterParent",
		apiextensions.NamespaceScoped,
	)

	var lastSync time.Time
	done := false

	// reconcile logic i.e. sync hook logic is here
	hook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := composite.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		resp := composite.SyncHookResponse{}
		if req.Parent.Object["status"] == nil {
			// If status hasn't been set yet, set it. This is the "zeroth" sync.
			// Metacontroller will set our status and then the object should quiesce.
			resp.Status = map[string]interface{}{}
		} else if lastSync.IsZero() {
			// This should be the final sync before quiescing. Do nothing except
			// request a resync. Other than our resyncAfter request, there should be
			// nothing that causes our object to get resynced.
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
			resp.Status = req.Parent.Object["status"].(map[string]interface{})
		}
		return json.Marshal(resp)
	})

	f.CreateCompositeController(
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
		"selector",
		"matchLabels",
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

	klog.Infof("Waiting for status.elaspedSeconds to be reported")
	var elapsedSeconds float64
	err = f.Wait(func() (bool, error) {
		parentResource, err :=
			parentClient.Namespace(testName).Get(testName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		val, found, err := unstructured.NestedFloat64(
			parentResource.Object,
			"status",
			"elapsedSeconds",
		)
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
