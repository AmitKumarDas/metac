package main

import (
	"openebs.io/metac/controller/generic"
	"openebs.io/metac/start"
)

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
