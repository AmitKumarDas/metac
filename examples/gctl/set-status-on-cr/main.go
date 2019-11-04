package main

import (
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/start"
)

// sync implements the idempotent logic to get to the desired
// state
//
// NOTE:
// 	SyncHookRequest is the payload received as part of reconcile
// request. Similarly, SyncHookResponse is the payload sent as a
// response as part of reconcile request.
//
// NOTE:
//	SyncHookRequest has the resource that is under watch, whereas
// Both SyncHookRequest & SyncHookResponse have the resources that
// form the desired state w.r.t the watched resource. These
// resources are known as attachments.
func sync(req *generic.SyncHookRequest, resp *generic.SyncHookResponse) error {
	if resp == nil {
		resp = &generic.SyncHookResponse{}
	}
	// Compute status based on latest observed state.
	if req == nil || req.Watch == nil {
		resp.ResyncAfterSeconds = 2
		resp.SkipReconcile = true
		return nil
	}

	resp.Status = map[string]interface{}{
		"phase": "Active",
		"conditions": []string{
			"Golang",
			"InlineHook",
			"GCtl",
		},
	}
	return nil
}

func main() {
	generic.AddToInlineRegistry("sync/cool-nerd", sync)
	start.Start()
}
