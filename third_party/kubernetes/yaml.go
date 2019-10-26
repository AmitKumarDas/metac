/*
Copyright 2019 The Kubernetes Authors.
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
	"bytes"

	"github.com/pkg/errors"
	goyaml "gopkg.in/yaml.v2"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// I do not remember the exact project from where this has been
// borrowed. The only thing I can remember is, this was from some
// Kubernetes sig or related project.

const (
	yamlSeparator = "\n---\n"
)

// UnstructList mimics a Kubernetes List object which
// can contain multiple other resources in its Items
// slice
type UnstructList struct {
	// ApiVersion represent the lists ApiVersion
	//
	// NOTE:
	//	This should always be "v1" that marks it as
	// a stable api and hence hopefully avoids
	// incompatibilities w.r.t Kubernetes conversions.
	APIVersion string `yaml:"apiVersion"`

	// Kind represents the lists Kind
	//
	// NOTE:
	//	This should always be set to "List" to hopefully
	// avoid breaking things from Kubernetes convertions
	Kind string `yaml:"kind"`

	// Items represents the list of items that this
	// List holds.
	//
	// NOTE:
	//	This field is important to represent this
	// List as unstructured.UnstructuredList
	//
	// NOTE:
	//	Convert all items to `map[string]interface{}`
	// for correct yaml marshalling
	//
	// NOTE:
	//	Each item corresponds to an unstructured instance's
	// Object field
	Items []map[string]interface{} `yaml:"items"`
}

// yamlToJSON converts a byte slice containing yaml into
// a byte slice containing json
func yamlToJSON(in []byte) ([]byte, error) {
	return yaml.ToJSON(in)
}

// JSONToUnstructured converts a raw json document into an
// Unstructured object
func JSONToUnstructured(in []byte) (unstructured.Unstructured, error) {
	obj := unstructured.Unstructured{}
	err := obj.UnmarshalJSON(in)
	if err != nil {
		return unstructured.Unstructured{}, errors.Wrapf(err, "Failed to unmarshal JSON")
	}
	return obj, nil
}

// YAMLToUnstructured converts either a single yaml document or
// list of yaml documents into an Unstructured object
func YAMLToUnstructured(in []byte) (u unstructured.Unstructured, err error) {
	var data []byte
	yamlList := splitYAML(in)
	if len(yamlList) != 1 {
		// this is a list of raw yaml documents
		// convert this to a known struct that
		// understands list of Kubernetes yamls
		data, err = toList(yamlList)
		if err != nil {
			return u,
				errors.Wrapf(err, "Failed to split yaml docs: Len %d", len(yamlList))
		}
	} else {
		// this is just a single yaml document
		data = yamlList[0]
	}
	json, err := yamlToJSON(data)
	if err != nil {
		return u, errors.Wrapf(err, "Failed to convert YAML to JSON")
	}
	return JSONToUnstructured(json)
}

// YAMLToUnstructuredSlice converts a raw yaml document into a
// slice of pointers to Unstructured objects
func YAMLToUnstructuredSlice(in []byte) ([]unstructured.Unstructured, error) {
	u, err := YAMLToUnstructured(in)
	if err != nil {
		return nil, err
	}
	if u.IsList() {
		result := []unstructured.Unstructured{}
		err = u.EachListItem(func(obj runtime.Object) error {
			o, ok := obj.(*unstructured.Unstructured)
			if !ok {
				kind := obj.GetObjectKind().GroupVersionKind().Kind
				return errors.Errorf("Resource %s is not an unstructured type", kind)
			}
			result = append(result, *o)
			return nil
		})
		if err != nil {
			return nil,
				errors.Wrapf(
					err,
					"Failed to convert yaml docs into slice of unstructured instances",
				)
		}
		return result, nil
	}
	return []unstructured.Unstructured{u}, nil
}

// splitYAML will take raw yaml from a file and split yaml
// documents on the yaml separator `---`, returning a list
// of documents in the original input
func splitYAML(in []byte) (out [][]byte) {
	split := bytes.Split(in, []byte(yamlSeparator))
	for _, data := range split {
		if len(data) > 0 {
			out = append(out, data)
		}
	}
	return
}

// toList converts a slice of yaml documents into a
// Kubernetes List kind
func toList(in [][]byte) ([]byte, error) {
	items := []map[string]interface{}{}
	for _, item := range in {
		data := make(map[string]interface{})
		err := goyaml.Unmarshal(item, data)
		if err != nil {
			return nil, errors.Wrapf(err, "Failed to unmarshal")
		}
		items = append(items, data)
	}
	return goyaml.Marshal(&UnstructList{
		APIVersion: "v1",
		Kind:       "List",
		Items:      items,
	})
}
