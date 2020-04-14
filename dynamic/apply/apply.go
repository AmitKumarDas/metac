/*
Copyright 2018 Google Inc.
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

// Package apply is a server-side substitute for `kubectl apply` that
// tries to guess the right thing to do without any type-specific knowledge.
// Instead of generating a PATCH request, it does the patching locally and
// returns a full object with the ResourceVersion intact.
//
// We can't use actual `kubectl apply` yet because it doesn't support
// strategic merge for CRDs. For example, if we include a PodTemplateSpec
// in a Custom Resource spec, then its containers and volumes will merge
// incorrectly.
package apply

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
)

const (
	lastAppliedAnnotation = "metac.openebs.io/last-applied-configuration"
)

// SetLastApplied sets the last applied state against a
// predefined annotation key
func SetLastApplied(obj *unstructured.Unstructured, lastApplied map[string]interface{}) error {
	return SetLastAppliedByAnnKey(obj, lastApplied, lastAppliedAnnotation)
}

// SetLastAppliedByAnnKey sets the last applied state against the
// provided annotation key
func SetLastAppliedByAnnKey(
	obj *unstructured.Unstructured,
	lastApplied map[string]interface{},
	annKey string,
) error {

	if len(lastApplied) == 0 {
		return nil
	}

	lastAppliedJSON, err := json.Marshal(lastApplied)
	if err != nil {
		return errors.Wrapf(
			err,
			"%s:%s:%s:%s: Failed to marshal last applied state against annotation %q",
			obj.GetAPIVersion(),
			obj.GetKind(),
			obj.GetNamespace(),
			obj.GetName(),
			annKey,
		)
	}

	ann := obj.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string, 1)
	}
	ann[annKey] = string(lastAppliedJSON)
	obj.SetAnnotations(ann)

	glog.V(5).Infof(
		"%s:%s:%s:%s: Annotation %q will be set with last applied state:\n%s",
		obj.GetAPIVersion(),
		obj.GetKind(),
		obj.GetNamespace(),
		obj.GetName(),
		annKey,
		string(lastAppliedJSON),
	)

	return nil
}

// SanitizeLastAppliedByAnnKey sanitizes the last applied state
// by removing last applied state related info (i.e. its own info)
// to avoid building up of nested last applied states.
//
// In other words, last applied state might have an annotation that stores
// the previous last applied state which in turn might have an annotation
// that stores the previous to previous last applied state. This nested
// annotation gets added everytime a reconcile event happens for the
// resource that needs to be applied against Kubernetes cluster.
func SanitizeLastAppliedByAnnKey(last map[string]interface{}, annKey string) {
	if len(last) == 0 {
		return
	}
	unstructured.RemoveNestedField(last, "metadata", "annotations", annKey)
}

// GetLastApplied returns the last applied state of the given
// object. Last applied state is derived based on a predefined
// annotation key to store last applied state.
func GetLastApplied(obj *unstructured.Unstructured) (map[string]interface{}, error) {
	return GetLastAppliedByAnnKey(obj, lastAppliedAnnotation)
}

// GetLastAppliedByAnnKey returns the last applied state of the given
// object based on the provided annotation
func GetLastAppliedByAnnKey(
	obj *unstructured.Unstructured, annKey string,
) (map[string]interface{}, error) {

	lastAppliedJSON := obj.GetAnnotations()[annKey]
	if lastAppliedJSON == "" {
		return nil, nil
	}

	lastApplied := make(map[string]interface{})
	err := json.Unmarshal([]byte(lastAppliedJSON), &lastApplied)
	if err != nil {
		return nil,
			errors.Wrapf(
				err,
				"%s:%s:%s:%s: Failed to unmarshal last applied config against annotation %q",
				obj.GetAPIVersion(),
				obj.GetKind(),
				obj.GetNamespace(),
				obj.GetName(),
				annKey,
			)
	}

	return lastApplied, nil
}

// Merge updates the observed object with the desired changes.
// Merge is based on a 3-way apply that takes in observed state,
// last applied state & desired state into consideration.
func Merge(observed, lastApplied, desired map[string]interface{}) (map[string]interface{}, error) {
	// Make a copy of observed & use it as the destination where merge
	// happens
	observedAsDest := runtime.DeepCopyJSON(observed)

	if _, err := merge("", observedAsDest, lastApplied, desired); err != nil {
		return nil, errors.Wrapf(err, "Can't merge desired changes")
	}
	return observedAsDest, nil
}

func merge(fieldPath string, observedAsDest, lastApplied, desired interface{}) (interface{}, error) {
	glog.V(7).Infof("Will try merge for field %q", fieldPath)

	switch observedDestVal := observedAsDest.(type) {
	case map[string]interface{}:
		// In this case, observed is a map.
		// Make sure the others are maps too.
		// Nil desired &/ nil last applied are OK.
		lastAppliedVal, ok := lastApplied.(map[string]interface{})
		if !ok && lastAppliedVal != nil {
			return nil,
				errors.Errorf(
					"%s: Expecting last applied as map[string]interface{}, got %T",
					fieldPath, lastApplied,
				)
		}
		desiredVal, ok := desired.(map[string]interface{})
		if !ok && desiredVal != nil {
			return nil,
				errors.Errorf(
					"%s: Expecting desired as map[string]interface{}, got %T",
					fieldPath, desired,
				)
		}
		return mergeMap(fieldPath, observedDestVal, lastAppliedVal, desiredVal)
	case []interface{}:
		// In this case observed is an array.
		// Make sure desired & last applied are arrays too.
		// Nil desired &/ last applied are OK.
		lastAppliedVal, ok := lastApplied.([]interface{})
		if !ok && lastAppliedVal != nil {
			return nil,
				errors.Errorf(
					"%s: Expecting last applied as []interface{}, got %T",
					fieldPath, lastApplied,
				)
		}
		desiredVal, ok := desired.([]interface{})
		if !ok && desiredVal != nil {
			return nil,
				fmt.Errorf(
					"%s: Expecting desired as []interface{}, got %T",
					fieldPath, desired,
				)
		}
		return mergeArray(fieldPath, observedDestVal, lastAppliedVal, desiredVal)
	default:
		// Observed is either a scalar or null.
		//
		// NOTE:
		// 	We have traversed to the leaf of the object. There is no further
		// traversal that needs to be done. At this point desired value is the
		// final merge value.
		//
		// NOTE:
		//	Since merge method is being called recursively, this point signals
		// end of last recursion
		return desired, nil
	}
}

func mergeMap(fieldPath string, observedAsDest, lastApplied, desired map[string]interface{}) (interface{}, error) {
	glog.V(7).Infof("Will try merge of map for field %q", fieldPath)

	// Remove fields that were present in lastApplied, but no longer
	// in desired. In other words, this decision to delete a field
	// is based on last applied state.
	//
	// NOTE:
	//	If there is no last applied then there will be **no** removals
	for key := range lastApplied {
		if _, present := desired[key]; !present {
			glog.V(4).Infof(
				"%s merge map: Will delete key %s: Last Applied 'Y': Desired 'N'",
				fieldPath, key,
			)
			delete(observedAsDest, key)
		}
	}

	// Once deletion _(which is probably the easy part)_ is done
	// Try Add or Update of fields.
	//
	// NOTE:
	//	If there is no desired state i.e. nil, then there will be
	// **no** adds or updates.
	var err error
	for key, desiredVal := range desired {
		// destination is mutated here
		observedAsDest[key], err =
			merge(
				fmt.Sprintf("%s[%s]", fieldPath, key),
				observedAsDest[key], lastApplied[key], desiredVal,
			)
		if err != nil {
			return nil, err
		}
	}

	// NOTE:
	//	If there is nil last applied state & nil desired state then
	// observed map will be returned as merge map.
	return observedAsDest, nil
}

func mergeArray(fieldPath string, observedAsDest, lastApplied, desired []interface{}) (interface{}, error) {
	glog.V(7).Infof("Will try merge of array for field %q", fieldPath)

	// If it looks like a list of map, use the special merge
	// by determing the best possible **merge key**
	if mergeKey := detectListMapKey(observedAsDest, lastApplied, desired); mergeKey != "" {
		return mergeListMap(fieldPath, mergeKey, observedAsDest, lastApplied, desired)
	}

	// It's a normal array of scalars.
	// Hence, consider the desired array.
	// TODO(enisoc): Check if there are any common cases where we want to merge.
	return desired, nil
}

func mergeListMap(fieldPath, mergeKey string, observedAsDest, lastApplied, desired []interface{}) (interface{}, error) {
	// transform the list to map, keyed by the mergeKey field.
	observedDestMap := makeMapFromList(mergeKey, observedAsDest)
	lastAppliedMap := makeMapFromList(mergeKey, lastApplied)
	desiredMap := makeMapFromList(mergeKey, desired)

	// once in map, try map based merge
	_, err := mergeMap(fieldPath, observedDestMap, lastAppliedMap, desiredMap)
	if err != nil {
		return nil, err
	}

	// Turn merged map back into a list, trying to preserve **partial order**.
	//
	// NOTE:
	//	In most of the cases, this ordering is more than sufficient.
	// This ordering helps in negating the diff found between two
	// lists each with same items but with different order (i.e. index).
	observedDestList := make([]interface{}, 0, len(observedDestMap))
	added := make(map[string]bool, len(observedDestMap))

	// First take items that were already in destination.
	// This helps in maintaining the order that was found before
	// the merge operation.
	for _, item := range observedAsDest {
		valueAsKey := stringMergeKey(item.(map[string]interface{})[mergeKey])
		if mergedItem, ok := observedDestMap[valueAsKey]; ok {
			observedDestList = append(observedDestList, mergedItem)
			// Remember which items we've already added to the final list.
			added[valueAsKey] = true
		}
	}
	// Then take items in desired that haven't been added yet.
	//
	// NOTE:
	//	This handles the case of newly added items in the desried
	// state. These items won't be present in observed or last applied
	// states.
	for _, item := range desired {
		valueAsKey := stringMergeKey(item.(map[string]interface{})[mergeKey])
		if !added[valueAsKey] {
			// append it since its not available in the final list
			observedDestList = append(observedDestList, observedDestMap[valueAsKey])
			added[valueAsKey] = true
		}
	}

	return observedDestList, nil
}

func makeMapFromList(mergeKey string, list []interface{}) map[string]interface{} {
	res := make(map[string]interface{}, len(list))
	for _, item := range list {
		// We only end up here if detectListMapKey() already verified that
		// all items are of type map
		itemMap := item.(map[string]interface{})
		res[stringMergeKey(itemMap[mergeKey])] = item
	}
	return res
}

// stringMergeKey converts the provided value _(corresponding to the
// merge key)_ that is not of type string to string.
func stringMergeKey(val interface{}) string {
	switch tval := val.(type) {
	case string:
		return tval
	default:
		return fmt.Sprintf("%v", val)
	}
}

// knownMergeKeys lists the key names we will guess as merge keys.
//
// The order determines precedence if multiple entries might work,
// with the first item having the highest precedence.
//
// NOTE:
// 	As of now we don't do merges on status because the controller is
// solely responsible for providing the entire contents of status.
// As a result, we don't try to handle things like status.conditions.
var knownMergeKeys = []string{
	"containerPort",
	"port",
	"name",
	"uid",
	"ip",
}

// detectListMapKey tries to guess whether a field is a
// k8s-style "list map".
//
// For example in the given sample 'names' is a list of maps:
// ```yaml
// names:
// - name: abc
//   desc: blah blah
// - name: def
//   desc: blah blah blah
// - name: xyz
//   desc: blabber
// ```
//
// You pass in all known examples of values for the field.
// If a likely merge key can be found, we return it.
// Otherwise, we return an empty string.
//
// NOTE:
//	Above sample yaml will return 'name' if this yaml is run
// against this method. In other words, 'name' is decided to be
// the key that is fit to be considered as merge key for the
// given list of maps.
//
// NOTE:
//	For this to work all items in **observed**, **lastApplied** &
// **desired** lists should have at-least one key in common. In
// addition, this common key should be part of knownMergeKeys.
//
// NOTE:
//	If any particular list is empty then common keys will be formed
// out of non-empty lists.
func detectListMapKey(lists ...[]interface{}) string {
	// Remember the set of keys that every object has in common.
	var commonKeys map[string]bool

	// loop over observed, last applied & desired lists
	for _, list := range lists {
		for _, item := range list {
			// All the items must be objects.
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				// no need to proceed since this is not a
				// list of maps
				return ""
			}
			if commonKeys == nil {
				// one time initialization
				// initialize commonKeys to consider all the fields
				// found in this map.
				commonKeys = make(map[string]bool, len(itemMap))
				for key := range itemMap {
					commonKeys[key] = true
				}
				continue
			}

			// For all other objects, prune the set.
			for key := range commonKeys {
				if _, ok := itemMap[key]; !ok {
					// remove the earlier added key, since its not
					// common across all the items of this list
					delete(commonKeys, key)
				}
			}
		}
	}
	// If all objects have **one** of the known conventional
	// merge keys in common, we'll guess that this is a list map.
	for _, key := range knownMergeKeys {
		if commonKeys[key] {
			// first possible match is the merge key
			//
			// NOTE:
			//	If an obj has more than one keys as known merge key,
			// preference is given to the first key found in
			// knownMergeKeys
			return key
		}
	}
	// If there were no matches for the common keys, then
	// this list will **not** be considered a list of maps even
	// though technically it will be at this point.
	//
	// Returning empty string implies this is not a list of maps
	return ""
}
