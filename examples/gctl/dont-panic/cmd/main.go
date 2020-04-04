package main

import (
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/start"
)

// sync executes the reconciliation logic for the watched resource.
// In this case resource under watch is a custom resource with kind
// **DontPanic**.
//
// NOTE:
// 	SyncHookRequest is the payload received as part of reconcile
// request. Similarly, SyncHookResponse is the payload sent as a
// response as part of reconcile request.
//
// NOTE:
//	SyncHookRequest has the resource that is under watch (here
// DontPanic custom resource)
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
	glog.Infof("Starting DontPanic sync")
	defer glog.Infof("Completed DontPanic sync")

	// reconciliation returns a desired CR whose definition
	// i.e. CRD is not applied to the k8s cluster
	//
	// In other words, k8s cluster does not understand
	// IAmError kind
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
	// add this desired instance to response to let metac create
	// it in the k8s cluster
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
	glog.Infof("Starting DontPanic finalize")
	defer glog.Infof("Completed DontPanic finalize")

	if request.Attachments.IsEmpty() {
		response.Finalized = true
	}
	return nil
}

func main() {
	generic.AddToInlineRegistry("sync/dontpanic", sync)
	generic.AddToInlineRegistry("finalize/dontpanic", finalize)
	start.Start()
}
