package main

import (
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/start"
)

// sync executes the reconciliation logic for the watched resource.
// In this case resource under watch is a custom resource with kind
// **DynamicConfig**.
//
// NOTE:
// 	SyncHookRequest is the payload received as part of reconcile
// request. Similarly, SyncHookResponse is the payload sent as a
// response as part of reconcile request.
//
// NOTE:
//	SyncHookRequest has the resource that is under watch (here
// DynamicConfig custom resource)
//
// NOTE:
//  Both SyncHookRequest & SyncHookResponse have the resources that
// form the desired state based on the watched resource's specs.
// These desired resources are termed as attachments by metac.
// SyncHookRequest has these attachments filled up by metac based
// on what is observed in the k8s cluster. SyncHookResponse's
// attachments needs to be filled up with what is desired by this
// controller logic.
func sync(
	request *generic.SyncHookRequest,
	response *generic.SyncHookResponse,
) error {
	glog.Infof("Starting DynamicConfig sync")
	defer glog.Infof("Completed DynamicConfig sync")

	desired := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "IAmError",
			"apiVersion": "notsure.com/v1",
			"metadata": map[string]interface{}{
				"name":      request.Watch.GetName(),
				"namespace": request.Watch.GetNamespace(),
				"annotations": map[string]interface{}{
					// this annotation helps in associating the watch
					// with its corresponding attachment
					"dontpanic/uid": string(request.Watch.GetUID()),
				},
			},
			"spec": map[string]interface{}{
				"message": "My CRD is not installed!",
			},
		},
	}
	// add this desired instance to response to let metac apply
	// it in the k8s cluster
	response.Attachments = append(
		response.Attachments,
		desired,
	)
	return nil
}

func main() {
	generic.AddToInlineRegistry("sync/dynamicconfig", sync)
	start.Start()
}
