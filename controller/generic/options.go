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
	"openebs.io/metac/apis/metacontroller/v1alpha1"
)

// Option represents the functional way to construct an
// instance of v1alpha1.GenericController
//
// This follows a functional option pattern
type Option func(*v1alpha1.GenericController)

// WithReadOnly sets the GenericController instance's
// ReadOnly option with the provided boolean
func WithReadOnly(b *bool) Option {
	return func(ctl *v1alpha1.GenericController) {
		ctl.Spec.ReadOnly = b
	}
}

// WithUpdateAny sets the GenericController instance's
// UpdateAny option with the provided boolean
func WithUpdateAny(b *bool) Option {
	return func(ctl *v1alpha1.GenericController) {
		ctl.Spec.UpdateAny = b
	}
}

// WithDeleteAny sets the GenericController instance's
// DeleteAny option with the provided boolean
func WithDeleteAny(b *bool) Option {
	return func(ctl *v1alpha1.GenericController) {
		ctl.Spec.DeleteAny = b
	}
}

// WithWebhookSyncURL sets the GenericController instance's
// Webhook sync url
func WithWebhookSyncURL(url *string) Option {
	return func(ctl *v1alpha1.GenericController) {
		if url == nil {
			return
		}
		if ctl.Spec.Hooks == nil {
			ctl.Spec.Hooks = &v1alpha1.GenericControllerHooks{}
		}
		if ctl.Spec.Hooks.Sync == nil {
			ctl.Spec.Hooks.Sync = &v1alpha1.Hook{}
		}
		ctl.Spec.Hooks.Sync.Webhook = &v1alpha1.Webhook{
			URL: url,
		}
	}
}

// WithWebhookFinalizeURL sets the GenericController instance's
// Webhook sync url
func WithWebhookFinalizeURL(url *string) Option {
	return func(ctl *v1alpha1.GenericController) {
		if url == nil {
			return
		}
		if ctl.Spec.Hooks == nil {
			ctl.Spec.Hooks = &v1alpha1.GenericControllerHooks{}
		}
		if ctl.Spec.Hooks.Finalize == nil {
			ctl.Spec.Hooks.Finalize = &v1alpha1.Hook{}
		}
		ctl.Spec.Hooks.Finalize.Webhook = &v1alpha1.Webhook{
			URL: url,
		}
	}
}

// WithWatch sets the provided watch against the GenericController
// instance
func WithWatch(watch *v1alpha1.GenericControllerResource) Option {
	return func(ctl *v1alpha1.GenericController) {
		if watch == nil {
			return
		}
		ctl.Spec.Watch = *watch
	}
}

// WithWatchRule sets the watch along with its rules against the
// provided GenericController instance
func WithWatchRule(rule *v1alpha1.ResourceRule) Option {
	return func(ctl *v1alpha1.GenericController) {
		if rule == nil {
			return
		}
		ctl.Spec.Watch = v1alpha1.GenericControllerResource{
			ResourceRule: *rule,
		}
	}
}

// WithAttachmentRules sets the watch along with its rules against the
// provided GenericController instance
func WithAttachmentRules(rules []*v1alpha1.ResourceRule) Option {
	return func(ctl *v1alpha1.GenericController) {
		attachments := []v1alpha1.GenericControllerAttachment{}
		for _, rule := range rules {
			if rule == nil {
				continue
			}
			attachments = append(
				attachments,
				v1alpha1.GenericControllerAttachment{
					GenericControllerResource: v1alpha1.GenericControllerResource{
						ResourceRule: *rule,
					},
				},
			)
		}

		ctl.Spec.Attachments = attachments
	}
}

// WithAttachments sets the watch along with its rules against the
// provided GenericController instance
func WithAttachments(atts []*v1alpha1.GenericControllerAttachment) Option {
	return func(ctl *v1alpha1.GenericController) {
		for _, att := range atts {
			if att == nil {
				continue
			}
			ctl.Spec.Attachments = append(ctl.Spec.Attachments, *att)
		}
	}
}
