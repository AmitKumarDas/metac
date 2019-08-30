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

package common

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/hooks"
)

// HookCallerOptionSchema sets the HookCaller instance with appropriate
// caller logic based on the provided schema
//
// NOTE:
//	This logic is expected to have multiple if conditions to support
// different hook types e.g. configHook, gRPCHook etc when they are
// supported in future
func HookCallerOptionSchema(schema *v1alpha1.Hook) hooks.HookCallerOption {
	return func(caller *hooks.HookCaller) error {
		if schema.Webhook != nil {
			wbCaller, err := hooks.NewWebhookCaller(
				WebhookCallerOptionURL(schema.Webhook),
				WebhookCallerOptionTimeout(schema.Webhook),
			)
			if err != nil {
				return err
			}

			caller.CallFunc = wbCaller.Call
		}
		return nil
	}
}

// WebhookCallerOptionURL evaluates provided webhook's url and sets
// the evaluated url against the WebhookCaller instance
func WebhookCallerOptionURL(schema *v1alpha1.Webhook) hooks.WebhookCallerOption {
	return func(caller *hooks.WebhookCaller) error {
		if schema.URL != nil {
			// Full URL overrides everything else.
			caller.URL = *schema.URL
			return nil
		}

		if schema.Service == nil || schema.Path == nil {
			return errors.Errorf(
				"Invalid webhook: Specify either full 'URL', or both 'Service' and 'Path'",
			)
		}

		// For now, just use cluster DNS to resolve Services.
		// If necessary, we can use a Lister to get more info about Services.
		if schema.Service.Name == "" || schema.Service.Namespace == "" {
			return errors.Errorf(
				"Invalid webhook service: Must specify service 'Name' and 'Namespace'",
			)
		}

		port := int32(80)
		if schema.Service.Port != nil {
			port = *schema.Service.Port
		}

		protocol := "http"
		if schema.Service.Protocol != nil {
			protocol = *schema.Service.Protocol
		}

		caller.URL = fmt.Sprintf(
			"%s://%s.%s:%v%s",
			protocol, schema.Service.Name, schema.Service.Namespace, port, *schema.Path,
		)
		return nil
	}
}

// WebhookCallerOptionTimeout evaluates webhook timeout and sets the evaluated
// timeout against WebhookCaller instance
func WebhookCallerOptionTimeout(schema *v1alpha1.Webhook) hooks.WebhookCallerOption {
	return func(caller *hooks.WebhookCaller) error {
		if schema.Timeout == nil {
			// Defaults to 10 Seconds to preserve current behavior.
			caller.Timeout = 10 * time.Second
			return nil
		}

		if schema.Timeout.Duration <= 0 {
			// Defaults to 10 Seconds if invalid.
			return errors.Errorf(
				"Invalid webhook: Timeout must be a non-zero positive duration",
			)
		}
		t := *(schema.Timeout)
		caller.Timeout = t.Duration
		return nil
	}
}

// CallHook invokes appropriate hook
func CallHook(schema *v1alpha1.Hook, request, response interface{}) error {
	caller, err := hooks.NewHookCaller(HookCallerOptionSchema(schema))
	if err != nil {
		return err
	}
	return caller.Call(request, response)
}
