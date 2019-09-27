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

package framework

import (
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"openebs.io/metac/apis/metacontroller/v1alpha1"
)

// BuildUnstructObjFromCRD builds an unstructured instance
// from the given CRD instance
//
// This unstructured instance has only its name set
func BuildUnstructObjFromCRD(
	crd *apiextensions.CustomResourceDefinition, name string,
) *unstructured.Unstructured {

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(crd.Spec.Group + "/" + crd.Spec.Versions[0].Name)
	obj.SetKind(crd.Spec.Names.Kind)

	// resource set is only set
	obj.SetName(name)
	return obj
}

// BuildUnstructuredObjFromJSON creates a new Unstructured instance
// from the given JSON string. It panics on a decode error because
// it's meant for use with hard-coded test data.
func BuildUnstructuredObjFromJSON(
	apiVersion, kind, name, jsonStr string,
) *unstructured.Unstructured {

	obj := map[string]interface{}{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
		panic(err)
	}

	u := &unstructured.Unstructured{Object: obj}
	u.SetAPIVersion(apiVersion)
	u.SetKind(kind)
	u.SetName(name)

	return u
}

// BuildResourceRuleFromCRD returns a new instance of ResourceRule
func BuildResourceRuleFromCRD(
	crd *apiextensions.CustomResourceDefinition,
) *v1alpha1.ResourceRule {

	return &v1alpha1.ResourceRule{
		APIVersion: crd.Spec.Group + "/" + crd.Spec.Versions[0].Name,
		Resource:   crd.Spec.Names.Plural,
	}
}
