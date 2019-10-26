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

package kubernetes

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type mockYmlMap map[string]interface{}

// TestAnyYamlDoc should work. This is basically
// a test of yaml.NewYAMLOrJSONDecoder only & nothing
// related to Metac.
func TestAnyYamlDoc(t *testing.T) {
	u, err := YAMLToUnstructured([]byte(`
kind: Test
stuff: 1
test-foo: 2
`))
	if err != nil {
		t.Fatalf("Expected no error: Got %s", err.Error())
	}
	if u.UnstructuredContent() == nil {
		t.Fatalf("Expected yaml content: Got none")
	}
	stuff, _, _ := unstructured.NestedInt64(u.Object, "stuff")
	if stuff != 1 {
		t.Fatalf("Expected 1 Got %d", stuff)
	}
	testfoo, _, _ := unstructured.NestedInt64(u.Object, "test-foo")
	if testfoo != 2 {
		t.Fatalf("Expected 2 Got %d", testfoo)
	}
}

// TestAnyMultiYamlDocs should work. This is basically
// a test of yaml.NewYAMLOrJSONDecoder only & nothing
// related to Metac.
func TestAnyMultiYamlDocs(t *testing.T) {
	ul, err := YAMLToUnstructuredSlice([]byte(`---
kind: YourTest
stuff: 1
test-foo: 2
---
---
kind: MyTest
stuff: 2
test-foo: 3
---
`))
	if err != nil {
		t.Fatalf("Expected no error: Got %s", err.Error())
	}

	if len(ul) != 2 {
		t.Fatalf("Expected yaml count 2: Got %d", len(ul))
	}

	yaml1, yaml2 := ul[0], ul[1]
	yml1stuff, _, _ :=
		unstructured.NestedInt64(yaml1.UnstructuredContent(), "stuff")
	yml1testfoo, _, _ :=
		unstructured.NestedInt64(yaml1.UnstructuredContent(), "test-foo")
	if yml1stuff != 1 || yml1testfoo != 2 {
		t.Fatalf(
			"Expected stuff 1 Got %d: Expected test-foo 2 Got %d",
			yml1stuff, yml1testfoo,
		)
	}

	yml2stuff, _, _ :=
		unstructured.NestedInt64(yaml2.UnstructuredContent(), "stuff")
	yml2testfoo, _, _ :=
		unstructured.NestedInt64(yaml2.UnstructuredContent(), "test-foo")
	if yml2stuff != 2 || yml2testfoo != 3 {
		t.Fatalf(
			"Expected stuff 2 Got %d: Expected test-foo 3 Got %d",
			yml2stuff, yml2testfoo,
		)
	}
}

// TestKubernetesYamlDocToUnstruct should work.
//
// NOTE:
//	Keep a note on the tab vs. spaces while writing below
// yaml. Stick to spaces.
func TestKubernetesYamlDocToUnstruct(t *testing.T) {
	u, err := YAMLToUnstructured([]byte(`---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: genericcontrollers.metac.openebs.io
spec:
  group: metac.openebs.io
  version: v1alpha1
  scope: Namespaced
  names:
    plural: genericcontrollers
    singular: genericcontroller
    kind: GenericController
    shortNames:
    - gctl
---
`))
	if err != nil {
		t.Fatalf("Expected no error: Got %v", err)
	}
	if u.GetAPIVersion() != "apiextensions.k8s.io/v1beta1" {
		t.Fatalf("Expected apiVersion 'apiextensions.k8s.io/v1beta1' Got %q", u.GetAPIVersion())
	}
	if u.GetKind() != "CustomResourceDefinition" {
		t.Fatalf("Expected kind 'CustomResourceDefinition' Got %q", u.GetKind())
	}
	if u.GetName() != "genericcontrollers.metac.openebs.io" {
		t.Fatalf("Expected name 'genericcontrollers.metac.openebs.io' Got %q", u.GetName())
	}
	scope, _, _ :=
		unstructured.NestedString(u.UnstructuredContent(), "spec", "scope")
	if scope != "Namespaced" {
		t.Fatalf("Expected scope 'Namespaced' Got %q", scope)
	}
	shortNames, _, _ :=
		unstructured.NestedStringSlice(u.UnstructuredContent(), "spec", "names", "shortNames")
	if len(shortNames) != 1 || strings.Join(shortNames, "") != "gctl" {
		t.Fatalf("Expected shortnames 'gctl' Got %v", shortNames)
	}
}

// TestKubernetesYamlDocsToUnstructs should work.
//
// NOTE:
//	Keep a note on the tab vs. spaces while writing below
// yaml. Stick to spaces.
func TestKubernetesYamlDocsToUnstructs(t *testing.T) {
	ul, err := YAMLToUnstructuredSlice([]byte(`---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: genericcontrollers.metac.openebs.io
spec:
  group: metac.openebs.io
  version: v1alpha1
  scope: Namespaced
  names:
    plural: genericcontrollers
    singular: genericcontroller
    kind: GenericController
    shortNames:
    - gctl
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: controllerrevisions.metac.openebs.io
spec:
  group: metac.openebs.io
  version: v1alpha1
  scope: Namespaced
  names:
    plural: controllerrevisions
    singular: controllerrevision
    kind: ControllerRevision
---
`))

	if err != nil {
		t.Fatalf("Expected no error: Got %v", err)
	}

	if len(ul) != 2 {
		t.Fatalf("Expected yaml count 2: Got %d", len(ul))
	}

	yaml1, yaml2 := ul[0], ul[1]
	if yaml1.GetName() != "genericcontrollers.metac.openebs.io" {
		t.Fatalf("Expected name 'genericcontrollers.metac.openebs.io' got %q", yaml1.GetName())
	}
	if yaml2.GetName() != "controllerrevisions.metac.openebs.io" {
		t.Fatalf("Expected name 'controllerrevisions.metac.openebs.io' got %q", yaml2.GetName())
	}
}

// TestMetaControllerDocsToUnstructs should work.
//
// NOTE:
//	Keep a note on the tab vs. spaces while writing below
// yaml. Stick to spaces.
func TestMetaControllerDocsToUnstructs(t *testing.T) {
	ul, err := YAMLToUnstructuredSlice([]byte(`---
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
apiVersion: metacontroller.k8s.io/v1alpha1
kind: CompositeController
metadata:
  name: daemonjob-controller
spec:
  generateSelector: true
  parentResource:
    apiVersion: ctl.example.com/v1
    resource: daemonjobs
  childResources:
  - apiVersion: apps/v1
    resource: daemonsets
  hooks:
    sync:
      webhook:
        url: http://daemonjob-controller.metacontroller/sync
---
`))

	if err != nil {
		t.Fatalf("Expected no error: Got %v", err)
	}

	if len(ul) != 2 {
		t.Fatalf("Expected yaml count 2: Got %d", len(ul))
	}

	yaml1, yaml2 := ul[0], ul[1]
	if yaml1.GetKind() != "GenericController" {
		t.Fatalf("Expected kind 'GenericController' got %q", yaml1.GetKind())
	}
	if yaml1.GetName() != "install-un-crd" {
		t.Fatalf("Expected name 'install-un-crd' got %q", yaml1.GetName())
	}

	if yaml2.GetKind() != "CompositeController" {
		t.Fatalf("Expected kind 'CompositeController' got %q", yaml2.GetKind())
	}
	if yaml2.GetName() != "daemonjob-controller" {
		t.Fatalf("Expected name 'daemonjob-controller' got %q", yaml2.GetName())
	}
}
