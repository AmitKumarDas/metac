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

// WebhookManager manages invocation of webhook
type WebhookManager struct {
	// webhook URL
	URL string

	// webhook invocation timeout
	Timeout time.Duration
}

// WebhookManagerOption is a typed function that is used
// to build *WebhookCaller instance
//
// NOTE:
//	This follows "functional options" pattern
type WebhookManagerOption func(*WebhookManager) error

// NewWebhookManager returns a new instance of WebhookManager
// based on an optional list of WebhookCallerOptions
func NewWebhookManager(opts ...WebhookManagerOption) (*WebhookManager, error) {
	mgr := &WebhookManager{}
	for _, o := range opts {
		err := o(mgr)
		if err != nil {
			return nil, err
		}
	}
	return mgr, nil
}

// String implements Stringer interface
func (mgr *WebhookManager) String() string {
	cstr := []string{"Webhook"}
	if mgr.URL != "" {
		cstr = append(cstr, fmt.Sprintf("url=%s", mgr.URL))
	}
	if mgr.Timeout != 0 {
		cstr = append(cstr, fmt.Sprintf("timeout=%d", mgr.Timeout))
	}
	return strings.Join(cstr, " ")
}

// Invoke this webhook by passing the given request
// and fill up the given response with the webhook response
func (mgr *WebhookManager) Invoke(request, response interface{}) error {
	// Encode request.
	reqBody, err := json.Marshal(request)
	if err != nil {
		return errors.Wrapf(err, "%s: Marshal failed", mgr)
	}
	if glog.V(6) {
		reqBodyIndent, _ := gojson.MarshalIndent(request, "", "  ")
		glog.Infof("%s: Invoking %q", mgr, reqBodyIndent)
	}

	// Send request.
	client := &http.Client{Timeout: mgr.Timeout}
	resp, err := client.Post(mgr.URL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return errors.Wrapf(err, "%s: Invoke failed", mgr)
	}
	defer resp.Body.Close()

	// Read response.
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "%s: Read response failed", mgr)
	}
	glog.V(6).Infof("%s: Got response %q", mgr, respBody)

	// Check status code.
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf(
			"%s: Response status %d is not OK: %q", mgr, resp.StatusCode, respBody,
		)
	}

	// Decode response.
	if err := json.Unmarshal(respBody, response); err != nil {
		return errors.Wrapf(err, "%s: UnMarshal response failed", mgr)
	}
	return nil
}
