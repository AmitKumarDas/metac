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

package selector

import "testing"

func TestValidateLabelKey(t *testing.T) {
	var tests = map[string]struct {
		key   string
		isErr bool
	}{
		"simple key": {
			key:   "app",
			isErr: false,
		},
		"dns key": {
			key:   "app.mayadata.io/name",
			isErr: false,
		},
		"hyphen in key": {
			key:   "app-mayadata-io/name",
			isErr: false,
		},
		"space in key": {
			key:   "app mayadata io/name",
			isErr: true,
		},
		"key starts with number": {
			key:   "101.app.mayadata.io/name",
			isErr: false,
		},
		"key ends with uid format": {
			key:   "uid.app.mayadata.io/101211-a23dda-010101-zd2we1",
			isErr: false,
		},
	}
	for name, mock := range tests {
		name := name
		mock := mock
		t.Run(name, func(t *testing.T) {
			err := validateLabelKey(mock.key)
			if mock.isErr && err == nil {
				t.Fatalf("Expected error got none")
			}
			if !mock.isErr && err != nil {
				t.Fatalf("Expected no error got [%+v]", err)
			}
		})
	}
}

func TestValidateLabelValue(t *testing.T) {
	var tests = map[string]struct {
		key   string
		value string
		isErr bool
	}{
		"simple value": {
			key:   "simple",
			value: "cstor",
			isErr: false,
		},
		"value with hyphen": {
			key:   "hyphen",
			value: "my-cstor",
			isErr: false,
		},
		"value with dots": {
			key:   "dots",
			value: "my.cstor.org",
			isErr: false,
		},
		"value with forward slash": {
			key:   "fwd-slash",
			value: "my/cstor",
			isErr: true,
		},
		"value with spaces": {
			key:   "spaces",
			value: "my cstor",
			isErr: true,
		},
		"value with curly braces": {
			key:   "curly-braces",
			value: `{"my": "cstor"}`,
			isErr: true,
		},
	}
	for name, mock := range tests {
		name := name
		mock := mock
		t.Run(name, func(t *testing.T) {
			err := validateLabelValue(mock.key, mock.value)
			if mock.isErr && err == nil {
				t.Fatalf("Expected error got none")
			}
			if !mock.isErr && err != nil {
				t.Fatalf("Expected no error got [%+v]", err)
			}
		})
	}
}
