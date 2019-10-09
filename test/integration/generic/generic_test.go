/*
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

package generic

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/test/integration/framework"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// TestGCtlSyncWebhook tests that the sync webhook triggers and
// passes the request/response properly.
func TestGCtlSyncWebhook(t *testing.T) {
	// TODO (@amitkumardas):
	// ReFactor this test to structure & methods
	// One _test.go inside integration/generic/ will have only one usecase

	nsNamePrefix := "gctl-test"
	testName := "sync-webhook"
	labels := map[string]string{
		"test":                  "test",
		"metac/controller-name": "generic",
		"metac/resource-type":   "cr-to-cr",
		"metac/test-category":   "sync-webhook",
	}

	// fixture provides the common test logic including a way to
	// invoke teardown after completion of this particular test
	f := framework.NewFixture(t)
	defer f.TearDown()

	// create namespace
	ns := f.CreateNamespaceGen(nsNamePrefix)

	// create namespace scoped custom resource definitions
	// 1. a CRD for watch
	// 2. a CRD for attachment
	watchCRD, watchClient := f.SetupCRD(
		"GTSWPrimary", apiextensions.NamespaceScoped,
	)
	attachmentCRD, attachmentClient := f.SetupCRD(
		"GTSWSecondary", apiextensions.NamespaceScoped,
	)

	// define the "reconcile logic" i.e. sync hook logic here
	hook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		// As a simple test of request/response content,
		// just build a child with the **same name** as the parent.
		//
		// Note that this does not create the child in kubernetes.
		// Creation of child in kubernetes is done by generic
		// controller on creation of parent resource.
		child := framework.BuildUnstructObjFromCRD(attachmentCRD, req.Watch.GetName())
		child.SetLabels(labels)

		resp := generic.SyncHookResponse{
			Attachments: []*unstructured.Unstructured{child},
		}
		return json.Marshal(resp)
	})

	// create generic metacontroller with same name & namespace
	// of that of the watch & attachment resources
	f.CreateGenericController(
		testName,
		ns.Name,
		generic.WithWebhookSyncURL(k8s.StringPtr(hook.URL)),
		generic.WithAttachmentRules(
			[]*v1alpha1.ResourceRule{
				framework.BuildResourceRuleFromCRD(attachmentCRD),
			},
		),
		generic.WithWatchRule(framework.BuildResourceRuleFromCRD(watchCRD)),
	)

	watchResource := framework.BuildUnstructObjFromCRD(watchCRD, testName)
	unstructured.SetNestedStringMap(
		watchResource.Object, labels, "spec", "selector", "matchLabels",
	)

	t.Logf(
		"Creating %s/%s of kind:%s",
		watchResource.GetNamespace(),
		watchResource.GetName(),
		watchResource.GetKind(),
	)
	_, err :=
		watchClient.Namespace(ns.Name).Create(watchResource, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"Created %s/%s of kind:%s",
		watchResource.GetNamespace(),
		watchResource.GetName(),
		watchResource.GetKind(),
	)

	t.Logf("Waiting for attachment sync")
	err = f.Wait(func() (bool, error) {
		_, err := attachmentClient.Namespace(ns.Name).Get(testName, metav1.GetOptions{})
		return err == nil, err
	})
	if err != nil {
		t.Errorf("Attachment sync failed: %v", err)
	}
	t.Logf("Attachment sync was successful")
}

// TestGCtlCascadingDelete tests that we request cascading deletion of children,
// even if the server-side default for that child type is non-cascading.
func TestGCtlCascadingDelete(t *testing.T) {
	nsNamePrefix := "gctl-test"
	controllerName := "cascading-delete"
	resourceName := "myres"
	labels := map[string]string{
		"test":                  "test",
		"metac/controller-type": "gctl",
		"metac/resource-type":   "cr-to-job",
		"metac/test-category":   "cascading-delete",
	}

	f := framework.NewFixture(t)
	defer f.TearDown()

	// create namespace
	ns := f.CreateNamespaceGen(nsNamePrefix)

	// get required clients
	watchCRD, watchClient := f.SetupCRD("GTCDPrimary", apiextensions.NamespaceScoped)
	jobChildClient := f.GetTypedClientset().BatchV1().Jobs(ns.Name)

	// define the "reconcile logic" i.e. sync hook logic here
	hook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		resp := generic.SyncHookResponse{}
		replicas, _, _ :=
			unstructured.NestedInt64(req.Watch.Object, "spec", "replicas")
		if replicas > 0 {
			// Create one attachment of type batch/v1 Job if replicas > 0.
			attachment := framework.BuildUnstructuredObjFromJSON(
				"batch/v1",
				"Job",
				resourceName,
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
			attachment.SetLabels(labels)

			// note that resp.Attachments is nil if this if-block
			// is not executed
			resp.Attachments = append(resp.Attachments, attachment)
		}
		return json.Marshal(resp)
	})

	f.CreateGenericController(
		controllerName,
		ns.Name,
		generic.WithWebhookSyncURL(k8s.StringPtr(hook.URL)),
		generic.WithWatchRule(
			framework.BuildResourceRuleFromCRD(watchCRD),
		),
		generic.WithAttachmentRules(
			[]*v1alpha1.ResourceRule{
				&v1alpha1.ResourceRule{APIVersion: "batch/v1", Resource: "jobs"},
			},
		),
	)

	watchResource := framework.BuildUnstructObjFromCRD(watchCRD, resourceName)
	unstructured.SetNestedStringMap(
		watchResource.Object, labels, "spec", "selector", "matchLabels",
	)
	// set watch spec with "replicas" property and
	// assign it with value=1
	unstructured.SetNestedField(watchResource.Object, int64(1), "spec", "replicas")

	t.Logf(
		"Creating %s %s/%s",
		watchResource.GetKind(),
		watchResource.GetNamespace(),
		watchResource.GetName(),
	)
	var err error
	watchResource, err =
		watchClient.Namespace(ns.Name).Create(watchResource, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"Created %s %s/%s",
		watchResource.GetKind(),
		watchResource.GetNamespace(),
		watchResource.GetName(),
	)

	t.Logf("Waiting for attachment job creation")
	err = f.Wait(func() (bool, error) {
		_, err := jobChildClient.Get(resourceName, metav1.GetOptions{})
		return err == nil, err
	})
	if err != nil {
		t.Fatalf("Attachment job create failed: %v", err)
	}
	t.Logf("Attachment job was created successfully")

	// Now that child exists, tell parent to delete it.
	t.Logf("Updating watch with replicas=0")
	_, err =
		watchClient.Namespace(ns.Name).AtomicUpdate(watchResource, func(obj *unstructured.Unstructured) bool {
			unstructured.SetNestedField(obj.Object, int64(0), "spec", "replicas")
			return true
		})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Updated watch with replicas=0")

	// Make sure the attachment gets actually deleted
	t.Logf("Waiting for attachment job to be synced i.e. delete")
	var child *batchv1.Job
	err = f.Wait(func() (bool, error) {
		var getErr error
		child, getErr = jobChildClient.Get(resourceName, metav1.GetOptions{})
		return apierrors.IsNotFound(getErr), nil
	})
	if err != nil {
		out, _ := json.Marshal(child)
		t.Errorf("Attachment job delete failed: %v; object: %s", err, out)
	}
	t.Logf("Attachment job synced / deleted successfully")
}

// TestGCtlResyncAfter tests that the resyncAfterSeconds field works.
func TestGCtlResyncAfter(t *testing.T) {
	nsNamePrefix := "gct-test"
	testName := "resync-after"
	labels := map[string]string{
		"test":                  "test",
		"metac/controller-name": "generic",
		"metac/resource-type":   "only-watch",
		"metac/test-category":   "resync-after",
	}

	f := framework.NewFixture(t)
	defer f.TearDown()

	// create namespace
	ns := f.CreateNamespaceGen(nsNamePrefix)
	watchCRD, watchClient := f.SetupCRD(
		"GTRAPrimary", apiextensions.NamespaceScoped,
	)

	var lastSync time.Time
	done := false

	// reconcile logic i.e. sync hook logic is here
	hook := f.ServeWebhook(func(body []byte) ([]byte, error) {
		req := generic.SyncHookRequest{}
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}

		resp := generic.SyncHookResponse{}
		if req.Watch.Object["status"] == nil {
			// If status hasn't been set yet, set it. This is the "zeroth" sync.
			// Metacontroller will set our status and then the object should quiesce.
			resp.Status = map[string]interface{}{}
		} else if lastSync.IsZero() {
			// This should be the final sync before quiescing (i.e. pausing).
			// Do nothing except request a resync. Other than our resyncAfter
			// request, there should be nothing that causes our object to get
			// resynced.
			lastSync = time.Now()
			resp.ResyncAfterSeconds = 0.1
		} else if !done {
			done = true
			// This is the second sync. Report how much time elapsed.
			// Consider this to be a one time thaw that calculates the
			// elapsed sync time & sets this new field called
			// elapsedSeconds
			resp.Status = map[string]interface{}{
				"elapsedSeconds": time.Since(lastSync).Seconds(),
			}
		} else {
			// If we're done, just **freeze** the status. In other words
			// set the response with watch's current status. This in
			// turn implies watch's status will never change even with
			// future reconciliations.
			resp.Status = req.Watch.Object["status"].(map[string]interface{})
		}
		return json.Marshal(resp)
	})

	f.CreateGenericController(
		testName,
		ns.Name,
		generic.WithWebhookSyncURL(k8s.StringPtr(hook.URL)),
		generic.WithWatchRule(
			framework.BuildResourceRuleFromCRD(watchCRD),
		),
	)

	watchResource := framework.BuildUnstructObjFromCRD(watchCRD, testName)
	unstructured.SetNestedStringMap(
		watchResource.Object, labels, "spec", "selector", "matchLabels",
	)

	t.Logf(
		"Creating %s %s/%s",
		watchResource.GetKind(),
		watchResource.GetNamespace(),
		watchResource.GetName(),
	)
	_, err :=
		watchClient.Namespace(ns.Name).Create(watchResource, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"Created %s %s/%s",
		watchResource.GetKind(),
		watchResource.GetNamespace(),
		watchResource.GetName(),
	)

	t.Logf("Waiting for status.elaspedSeconds to be reported")
	var elapsedSeconds float64
	err = f.Wait(func() (bool, error) {
		parentResource, err :=
			watchClient.Namespace(ns.Name).Get(testName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		val, found, err := unstructured.NestedFloat64(
			parentResource.Object, "status", "elapsedSeconds",
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
	t.Logf("status.elapsedSeconds was reported as %v", elapsedSeconds)

	if elapsedSeconds > 1.0 {
		t.Errorf(
			"ResyncAfter didn't occur in time: want '0.1' got: %v",
			elapsedSeconds,
		)
	}
}
