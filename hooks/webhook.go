/*
Copyright 2017 Google Inc.
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

package hooks

import (
	"bytes"
	gojson "encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/json"
)

// WebhookCaller manages invocation of webhook
type WebhookCaller struct {
	// webhook URL
	URL string

	// webhook invocation timeout
	Timeout time.Duration
}

// WebhookCallerOption is a typed function that is meant to work
// with *WebhookCaller as value
//
// NOTE:
//	This follows the pattern called "functional options"
type WebhookCallerOption func(*WebhookCaller) error

// NewWebhookCaller returns a new instance of WebhookCaller
// based on an optional list of WebhookCallerOptions
func NewWebhookCaller(options ...WebhookCallerOption) (*WebhookCaller, error) {
	caller := &WebhookCaller{}
	for _, o := range options {
		err := o(caller)
		if err != nil {
			return nil, err
		}
	}
	return caller, nil
}

// String implements Stringer interface
func (c *WebhookCaller) String() string {
	cstr := []string{"Webhook"}
	if c.URL != "" {
		cstr = append(cstr, fmt.Sprintf("url=%s", c.URL))
	}
	if c.Timeout != 0 {
		cstr = append(cstr, fmt.Sprintf("timeout=%d", c.Timeout))
	}
	return strings.Join(cstr, " ")
}

// Call invokes this webhook by passing the given request
// and filling up with the received response
func (c *WebhookCaller) Call(request, response interface{}) error {
	// Encode request.
	reqBody, err := json.Marshal(request)
	if err != nil {
		return errors.Wrapf(err, "%s: Failed to marshal", c)
	}
	if glog.V(6) {
		reqBodyIndent, _ := gojson.MarshalIndent(request, "", "  ")
		glog.Infof(
			"DEBUG: %s: Invoking %q", c, reqBodyIndent,
		)
	}

	// Send request.
	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Post(c.URL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return errors.Wrapf(err, "%s: Failed to invoke", c)
	}
	defer resp.Body.Close()

	// Read response.
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "%s: Failed to read response", c)
	}
	glog.V(6).Infof("DEBUG: %s: Got response %q", c, respBody)

	// Check status code.
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf(
			"%s: Got error %d: %q", c, resp.StatusCode, respBody,
		)
	}

	// Decode response.
	if err := json.Unmarshal(respBody, response); err != nil {
		return errors.Wrapf(err, "%s: Failed to unmarshal response", c)
	}
	return nil
}
