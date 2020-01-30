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
	"openebs.io/metac/hooks/webhook"
)

// InvokeHook invokes the given hook with the given request
func InvokeHook(schema *v1alpha1.Hook, request, response interface{}) error {
	i, err := hooks.NewInvoker(WithHookSchema(schema))
	if err != nil {
		return err
	}
	return i.Invoke(request, response)
}

// WithHookSchema sets the hook invoker instance with appropriate
// invoke function based on the provided schema
//
// NOTE:
//	This logic is expected to have multiple **if conditions** to support
// different hook types e.g. webhook, inline hook, etc
func WithHookSchema(schema *v1alpha1.Hook) hooks.InvokerOption {
	return func(invoker *hooks.Invoker) error {
		// webhook is the only commonly supported hook for
		// all meta controllers
		if schema.Webhook == nil {
			return errors.Errorf("Unsupported hook %v", schema)
		}
		// Since this is webhook set the webhook call func
		// create a new instance of webhook invoker
		whi, err := webhook.NewInvoker(
			// set various webhook options
			SetWebhookURLFromSchema(schema.Webhook),
			SetWebhookTimeoutFromSchemaOrDefault(schema.Webhook),
			SetWebhookCABundleFromSchemaOrDefault(schema.Webhook),
		)
		if err != nil {
			return err
		}
		invoker.InvokeFn = whi.Invoke
		return nil
	}
}

// SetWebhookURLFromSchema evaluates provided webhook's url and sets
// the evaluated url against the WebhookCaller instance
func SetWebhookURLFromSchema(schema *v1alpha1.Webhook) webhook.InvokerOption {
	return func(caller *webhook.Invoker) error {
		if schema.URL != nil {
			// Full URL overrides everything else.
			caller.URL = *schema.URL
			return nil
		}

		if schema.Service == nil || schema.Path == nil {
			return errors.Errorf(
				"Invalid webhook: Specify either full 'URL', or both 'Service' & 'Path': %v",
				schema,
			)
		}

		// For now, just use cluster DNS to resolve Services.
		// If necessary, we can use a Lister to get more info about Services.
		if schema.Service.Name == "" || schema.Service.Namespace == "" {
			return errors.Errorf(
				"Invalid webhook service: Specify service 'Name' & 'Namespace': %v",
				schema,
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

		// set the evaluated URL to be used to invoke webhook
		caller.URL = fmt.Sprintf(
			"%s://%s.%s:%v%s",
			protocol, schema.Service.Name, schema.Service.Namespace, port, *schema.Path,
		)
		return nil
	}
}

// SetWebhookTimeoutFromSchemaOrDefault evaluates webhook timeout and sets the
// evaluated timeout against WebhookCaller instance
func SetWebhookTimeoutFromSchemaOrDefault(schema *v1alpha1.Webhook) webhook.InvokerOption {
	return func(caller *webhook.Invoker) error {
		if schema.Timeout == nil {
			// Defaults to 10 Seconds to preserve current behavior.
			caller.Timeout = 10 * time.Second
			return nil
		}

		if schema.Timeout.Duration <= 0 {
			// Defaults to 10 Seconds if invalid.
			return errors.Errorf(
				"Invalid webhook timeout: Must be > 0: %v",
				schema,
			)
		}

		t := *(schema.Timeout)
		caller.Timeout = t.Duration
		return nil
	}
}

// SetWebhookCABundleFromSchemaOrDefault evaluates webhook timeout and sets the
// evaluated timeout against WebhookCaller instance
func SetWebhookCABundleFromSchemaOrDefault(schema *v1alpha1.Webhook) webhook.InvokerOption {
	return func(caller *webhook.Invoker) error {
		caller.CABundle = *schema.CABundle
		return nil
	}
}
