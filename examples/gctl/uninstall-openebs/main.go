package main

import (
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/start"
)

// finalizeNamespace implements the idempotent logic to advance to
// the desired state.
//
// NOTE:
// 	This reconcile logic should be set against finalize hook only.
// Since finalize hook gets invoked when the watched resource get
// deleted.
//
// NOTE:
//	Metac automatically adds a metac finalizer to the watched resource
// if metac finds presence of finalize hook.
//
// NOTE:
//	If response.Attachments is set to nil, then all the attachments
// found in request get deleted by metac. If response.Attachments
// does not include any specific attachment(s) found in request then
// that specific attachment(s) gets deleted.
//
// NOTE:
// 	SyncHookRequest is the payload received as part of reconcile
// request. Similarly, SyncHookResponse is the payload sent as a
// response as part of reconcile request.
//
// NOTE:
//	SyncHookRequest has the resource that is under watch (here
// openebs namespace), whereas both SyncHookRequest &
// SyncHookResponse have the resources that form the desired state
// w.r.t the watched resource. These resources are known as
// attachments.
func finalizeNamespace(request *generic.SyncHookRequest, response *generic.SyncHookResponse) error {
	var hasAtLeastOneCustomResource bool
	if response == nil {
		response = &generic.SyncHookResponse{}
	}

	if request.Attachments.IsEmpty() {
		// setting finalized to true indicates metac to complete
		// this reconcilation i.e. remove metac finalizer from watch
		// resource
		response.Finalized = true
		return nil
	}

	for _, attachment := range request.Attachments.List() {
		if attachment.GetKind() == "CustomResourceDefinition" {
			// maintain all CRDs till no custom resources are observed
			response.Attachments = append(response.Attachments, attachment)
			continue
		}

		if len(attachment.GetFinalizers()) != 0 {
			attachmentCopy := attachment

			// remove all the finalizers
			attachmentCopy.SetFinalizers([]string{})
			response.Attachments = append(response.Attachments, attachmentCopy)
		}

		hasAtLeastOneCustomResource = true
	}

	// verify if there are no observed custom resources
	if !hasAtLeastOneCustomResource {
		// Setting attachments to nil will delete all the observed
		// attachments.
		//
		// In other words, this deletes OpenEBS CRDs.
		response.Attachments = nil
	}

	return nil
}

func main() {
	generic.AddToInlineRegistry("finalize/namespace", finalizeNamespace)
	start.Start()
}
