/*
Copyright 2017 Google Inc.

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

package hooks

import (
	"bytes"
	gojson "encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/json"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
)

func callWebhook(webhook *v1alpha1.Webhook, request interface{}, response interface{}) error {
	url, err := webhookURL(webhook)
	hookTimeout, err := webhookTimeout(webhook)
	if err != nil {
		return err
	}

	// Encode request.
	reqBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("Failed to execute webhook %q: %v", url, err)
	}
	if glog.V(6) {
		reqBodyIndent, _ := gojson.MarshalIndent(request, "", "  ")
		glog.Infof(
			"DEBUG: will execute webhook %q: timeout %q: request %q",
			url,
			hookTimeout,
			reqBodyIndent,
		)
	}

	// Send request.
	client := &http.Client{Timeout: hookTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("Failed to execute webhook %q: %v", url, err)
	}
	defer resp.Body.Close()

	// Read response.
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf(
			"Failed to execute webhook %q: can't read response body: %v",
			url,
			err,
		)
	}
	glog.V(6).Infof(
		"DEBUG: executed webhook %q: got response %q", url, respBody,
	)

	// Check status code.
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"Failed to execute webhook %q: remote error: %q",
			url,
			respBody,
		)
	}

	// Decode response.
	if err := json.Unmarshal(respBody, response); err != nil {
		return fmt.Errorf(
			"Failed to execute webhook %q: can't unmarshal response: %v",
			url,
			err,
		)
	}
	return nil
}

func webhookURL(webhook *v1alpha1.Webhook) (string, error) {
	if webhook.URL != nil {
		// Full URL overrides everything else.
		return *webhook.URL, nil
	}
	if webhook.Service == nil || webhook.Path == nil {
		return "", fmt.Errorf(
			"Invalid webhook: specify either full 'url', or both 'service' and 'path'",
		)
	}

	// For now, just use cluster DNS to resolve Services.
	// If necessary, we can use a Lister to get more info about Services.
	if webhook.Service.Name == "" || webhook.Service.Namespace == "" {
		return "", fmt.Errorf(
			"Invalid webhook service: must specify service 'name' and 'namespace'",
		)
	}
	port := int32(80)
	if webhook.Service.Port != nil {
		port = *webhook.Service.Port
	}
	protocol := "http"
	if webhook.Service.Protocol != nil {
		protocol = *webhook.Service.Protocol
	}
	return fmt.Sprintf(
		"%s://%s.%s:%v%s",
		protocol,
		webhook.Service.Name,
		webhook.Service.Namespace,
		port,
		*webhook.Path,
	), nil
}

func webhookTimeout(webhook *v1alpha1.Webhook) (time.Duration, error) {
	if webhook.Timeout == nil {
		// Defaults to 10 Seconds to preserve current behavior.
		return 10 * time.Second, nil
	}

	if webhook.Timeout.Duration <= 0 {
		// Defaults to 10 Seconds if invalid.
		return 10 * time.Second, fmt.Errorf(
			"Invalid webhook: timeout must be a non-zero positive duration",
		)
	}

	return webhook.Timeout.Duration, nil
}
