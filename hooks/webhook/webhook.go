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

package webhook

import (
	"bytes"
	gojson "encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/json"
)

// Invoker manages invocation of webhook
type Invoker struct {
	// webhook URL
	URL string

	// webhook invocation timeout
	Timeout time.Duration
}

// InvokerOption is a typed function that is used
// to build *Invoker instance
//
// NOTE:
//	This follows "functional options" pattern
type InvokerOption func(*Invoker) error

// NewInvoker returns a new instance of Invoker
// based on an optional list of InvokerOptions
func NewInvoker(opts ...InvokerOption) (*Invoker, error) {
	i := &Invoker{}
	for _, o := range opts {
		err := o(i)
		if err != nil {
			return nil, err
		}
	}
	return i, nil
}

// String implements Stringer interface
func (i *Invoker) String() string {
	return fmt.Sprintf("Webhook Invoker: URL=%s: Timeout=%s", i.URL, i.Timeout)
}

// Invoke this webhook by passing the given request
// and fill up the given response with the webhook response
func (i *Invoker) Invoke(request, response interface{}) error {
	// Encode request.
	reqBody, err := json.Marshal(request)
	if err != nil {
		return errors.Wrapf(err, "%s: Failed to marshal", i)
	}
	if glog.V(6) {
		reqBodyIndent, _ := gojson.MarshalIndent(request, "", "  ")
		glog.Infof("%s: Will invoke %q", i, reqBodyIndent)
	}

	// Send request.
	client := &http.Client{Timeout: i.Timeout}
	resp, err := client.Post(i.URL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return errors.Wrapf(err, "%s: Failed to invoke", i)
	}
	defer resp.Body.Close()

	// Read response.
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrapf(err, "%s: Failed to read response", i)
	}
	glog.V(6).Infof("%s: Got response %q", i, respBody)

	// Check status code.
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf(
			"%s: Response status is not OK: Got %d: Response %q", i, resp.StatusCode, respBody,
		)
	}

	// Decode response.
	if err := json.Unmarshal(respBody, response); err != nil {
		return errors.Wrapf(err, "%s: Failed to unmarshal response", i)
	}

	glog.V(6).Infof("%s: Invoked successfully", i)
	return nil
}
