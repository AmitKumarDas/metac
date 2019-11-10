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

package config

import (
	"testing"

	k8s "openebs.io/metac/third_party/kubernetes"
)

func TestConfigLoad(t *testing.T) {
	config := &Config{
		Path: "testdata/",
	}
	mConfigs, err := config.Load()
	if err != nil {
		t.Fatalf("Expected no error: Got %v", err)
	}
	if len(mConfigs) != 3 {
		t.Fatalf("Expected metac config count 3: Got %d", len(mConfigs))
	}
	gctls, err := mConfigs.ListGenericControllers()
	if err != nil {
		t.Fatalf("Expected no error while listing gctls: Got %v", err)
	}
	if len(gctls) != 1 {
		t.Fatalf("Expected gctl count 1: Got %d", len(gctls))
	}
}

func TestMetacConfigsListGeneric(t *testing.T) {
	ul, err := k8s.YAMLToUnstructuredSlice([]byte(`
---
---
---
apiVersion: metac.openebs.io/v1alpha1
kind: GenericController
metadata:
  name: install-un-crd
spec:
  watch:
    apiVersion: v1
    resource: namespaces
    nameSelector:
    # we are interested in amitd namespace only
    - amitd
  attachments:
  - apiVersion: apiextensions.k8s.io/v1beta1
    resource: customresourcedefinitions
    nameSelector:
    # we are interested in storages CRD only
    - storages.dao.amitd.io
  hooks:
    sync:
      webhook:
        url: http://jsonnetd.metac/sync-crd
    finalize:
      webhook:
        url: http://jsonnetd.metac/finalize-crd
---
---
---
kind: Test
stuff: 1
test-foo: 2
---
`))
	if err != nil {
		t.Fatalf("Expected no error got %v", err)
	}
	if len(ul) == 0 {
		t.Fatalf("Expected multiple unstruct instances got %d", len(ul))
	}

	var mc MetacConfigs
	mc = append(mc, ul...)
	gctls, err := mc.ListGenericControllers()
	if err != nil {
		t.Fatalf("Expected no error got %v", err)
	}
	if len(gctls) != 1 {
		t.Fatalf("Expected one generic controller config count got %d", len(gctls))
	}
	gctl := gctls[0]
	if gctl.GetName() != "install-un-crd" {
		t.Fatalf("Expected gctl name 'install-un-crd' got %q", gctl.GetName())
	}
	if len(gctl.Spec.Attachments) != 1 {
		t.Fatalf("Expected one attachment got %d", len(gctl.Spec.Attachments))
	}
	if *gctl.Spec.Hooks.Sync.Webhook.URL != "http://jsonnetd.metac/sync-crd" {
		t.Fatalf(
			"Expected sync 'http://jsonnetd.metac/sync-crd' got %s",
			*gctl.Spec.Hooks.Sync.Webhook.URL,
		)
	}
	if *gctl.Spec.Hooks.Finalize.Webhook.URL != "http://jsonnetd.metac/finalize-crd" {
		t.Fatalf(
			"Expected sync 'http://jsonnetd.metac/finalize-crd' got %s",
			*gctl.Spec.Hooks.Finalize.Webhook.URL,
		)
	}
}
