package main

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/start"
)

// sync executes the reconciliation logic for the watched resource.
// In this case resource under watch is a custom resource with kind
// Helloworld.
//
// NOTE:
// 	SyncHookRequest is the payload received as part of reconcile
// request. Similarly, SyncHookResponse is the payload sent as a
// response as part of reconcile request.
//
// NOTE:
//	SyncHookRequest has the resource that is under watch (here
// Helloworld custom resource)
//
// NOTE:
//  Both SyncHookRequest & SyncHookResponse have the resources that
// form the desired state w.r.t the watched resource. These desired
// resources are termed as attachments by metac. SyncHookRequest has
// these attachments filled up by metac based on what is observed
// in the k8s cluster. SyncHookResponse's attachments needs to be
// filled up with what is desired by this controller logic.
func sync(
	request *generic.SyncHookRequest,
	response *generic.SyncHookResponse,
) error {
	glog.Infof("Starting hello world sync")
	defer glog.Infof("Completed hello world sync")

	// extract spec.who from Helloworld
	who, found, err := unstructured.NestedString(
		request.Watch.UnstructuredContent(),
		"spec",
		"who",
	)
	if err != nil {
		return err
	}
	if !found {
		return errors.Errorf("Can't sync: spec.who is not set")
	}
	// build the desired Pod
	desired := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "Pod",
			"apiVersion": "v1",
			"metadata": map[string]interface{}{
				"name":      request.Watch.GetName(),
				"namespace": request.Watch.GetNamespace(),
				"annotations": map[string]interface{}{
					// this annotation helps in associating the watch
					// with its corresponding attachment
					"helloworld/uid": string(request.Watch.GetUID()),
				},
			},
			"spec": map[string]interface{}{
				"restartPolicy": "OnFailure",
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "hello",
						"image": "busybox",
						"command": []interface{}{
							"echo",
							fmt.Sprintf("Hello, %s", who),
						},
					},
				},
			},
		},
	}
	// add this desired Pod instance to response
	// to let metac create this in the k8s cluster
	response.Attachments = append(
		response.Attachments,
		desired,
	)
	return nil
}

// finalize will delete the attachment(s) created during
// reconciliation of the watch
//
// NOTE:
//	Presence of finalize in the config automatically adds
// finalizers against the watch
//
// NOTE:
// 	Once attachments are deleted from the cluster,
// 'response.Finalized' is set to true which in turn removes
// this finalizers from the watch
func finalize(
	request *generic.SyncHookRequest,
	response *generic.SyncHookResponse,
) error {
	glog.Infof("Starting hello world finalize")
	defer glog.Infof("Completed hello world finalize")

	if request.Attachments.IsEmpty() {
		response.Finalized = true
	}
	return nil
}

func main() {
	generic.AddToInlineRegistry("sync/helloworld", sync)
	generic.AddToInlineRegistry("finalize/helloworld", finalize)
	start.Start()
}
