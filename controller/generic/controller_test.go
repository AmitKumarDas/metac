/*
Copyright 2020 The MayaData Authors.

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
	"testing"

	k8s "openebs.io/metac/third_party/kubernetes"
)

func TestUpdateStringMap(t *testing.T) {
	var tests = map[string]struct {
		dest     map[string]string
		updates  map[string]*string
		isChange bool
		expect   map[string]string
	}{
		"nil dest & nil updates": {},
		"nil dest & not nil updates": {
			updates: map[string]*string{
				"hi": k8s.StringPtr("hello"),
			},
			isChange: false,
			expect: map[string]string{
				"hi": "hello",
			},
		},
		"dest & nil updates": {
			dest: map[string]string{
				"hi": "hello",
			},
			isChange: false,
			expect: map[string]string{
				"hi": "hello",
			},
		},
		"just change": {
			dest: map[string]string{
				"hi": "hello",
			},
			updates: map[string]*string{
				"hi": k8s.StringPtr("there"),
			},
			isChange: true,
			expect: map[string]string{
				"hi": "there",
			},
		},
		"remove existing": {
			dest: map[string]string{
				"hi": "hello",
			},
			updates: map[string]*string{
				"hi": nil,
			},
			isChange: true,
			expect:   map[string]string{},
		},
		"remove everything": {
			dest: map[string]string{
				"hi":       "hello",
				"namaskar": "all",
			},
			updates: map[string]*string{
				"hi":       nil,
				"namaskar": nil,
			},
			isChange: true,
			expect:   map[string]string{},
		},
		"remove existing & new addition": {
			dest: map[string]string{
				"hi": "hello",
			},
			updates: map[string]*string{
				"hi":       nil,
				"namaskar": k8s.StringPtr("all"),
			},
			isChange: true,
			expect: map[string]string{
				"namaskar": "all",
			},
		},
		"keep existing & new addition": {
			dest: map[string]string{
				"hi": "hello",
			},
			updates: map[string]*string{
				"namaskar": k8s.StringPtr("all"),
			},
			isChange: true,
			expect: map[string]string{
				"hi":       "hello",
				"namaskar": "all",
			},
		},
	}
	for name, mock := range tests {
		name := name
		mock := mock
		t.Run(name, func(t *testing.T) {
			got := updateStringMap(mock.dest, mock.updates)
			if mock.isChange != got {
				t.Fatalf(
					"Expected change %t got %t",
					mock.isChange,
					got,
				)
			}
			if mock.dest == nil {
				return
			}
			if len(mock.expect) != len(mock.dest) {
				t.Fatalf(
					"Expected dest count %d got %d",
					len(mock.expect),
					len(mock.dest),
				)
			}
			for ek, ev := range mock.expect {
				if mock.dest[ek] != ev {
					t.Fatalf(
						"Expected %s for key %s got %s",
						ev,
						ek,
						mock.dest[ek],
					)
				}
			}
		})
	}
}
