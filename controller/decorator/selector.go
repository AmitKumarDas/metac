/*
Copyright 2018 Google Inc.

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

package decorator

import (
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/common"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
)

// selector options supported by decorator controller
//
// TODO (@amitkumardas) Provide useful unit tests that help
// to:
//	1/ eliminate wrong logic
//	2/ enable contributors to understand the logic
//	3/ add/update new logic without injecting bugs
//
// TODO (@amitkumardas) Check if this can be a common package
// perhaps in dynamic package that in turn can be used as a
// library by other projects.
type decoratorSelector struct {
	labelSelectors      map[string]labels.Selector
	annotationSelectors map[string]labels.Selector
}

// newDecoratorSelector returns a new instance of decorator
// selector based on the provided resources and decorator
// controller
//
// TODO (@amitkumardas) Should this be renamed to newParentSelector
func newDecoratorSelector(
	resources *dynamicdiscovery.ResourceMap,
	dc *v1alpha1.DecoratorController,
) (*decoratorSelector, error) {
	// init
	ds := &decoratorSelector{
		labelSelectors:      make(map[string]labels.Selector),
		annotationSelectors: make(map[string]labels.Selector),
	}
	var err error

	// resources are the parents in decorator controller
	for _, parent := range dc.Spec.Resources {
		// fetch the resource from the discovered set
		resource := resources.Get(parent.APIVersion, parent.Resource)
		if resource == nil {
			return nil, errors.Errorf(
				"can't find parent resource %q in apiVersion %q",
				parent.Resource,
				parent.APIVersion,
			)
		}
		key := selectorMapKey(resource.Group, resource.Kind)

		// Convert the label selector to the internal form.
		if parent.LabelSelector != nil {
			ds.labelSelectors[key], err = metav1.LabelSelectorAsSelector(parent.LabelSelector)
			if err != nil {
				return nil, errors.Wrapf(
					err,
					"can't convert label selector for parent resource %q in apiVersion %q",
					parent.Resource,
					parent.APIVersion,
				)
			}
		} else {
			// Add an explicit selector so we can tell the difference between
			// missing (not a type we care about) and empty (select everything).
			ds.labelSelectors[key] = labels.Everything()
		}

		// Convert the annotation selector to a label selector, then to internal form.
		if parent.AnnotationSelector != nil {
			labelSelector := &metav1.LabelSelector{
				MatchLabels:      parent.AnnotationSelector.MatchAnnotations,
				MatchExpressions: parent.AnnotationSelector.MatchExpressions,
			}
			ds.annotationSelectors[key], err = metav1.LabelSelectorAsSelector(labelSelector)
			if err != nil {
				return nil, errors.Wrapf(
					err,
					"can't convert annotation selector for parent resource %q in apiVersion %q",
					parent.Resource,
					parent.APIVersion,
				)
			}
		} else {
			// Add an explicit selector so we can tell the difference between
			// missing (not a type we care about) and empty (select everything).
			ds.annotationSelectors[key] = labels.Everything()
		}
	}

	return ds, nil
}

// Matches flags if the provided unstruct instance match the
// selectors that were set previously
func (ds *decoratorSelector) Matches(obj *unstructured.Unstructured) bool {
	// Look up the label and annotation selectors for this object.
	// Use only Group and Kind. Ignore Version.
	apiGroup, _ := common.ParseAPIVersion(obj.GetAPIVersion())
	key := selectorMapKey(apiGroup, obj.GetKind())

	labelSelector := ds.labelSelectors[key]
	annotationSelector := ds.annotationSelectors[key]
	if labelSelector == nil || annotationSelector == nil {
		// This object is not a kind we care about, so it doesn't match
		// It is very strict in the sense that the given unstruct instance
		// must provide a selector (earlier) to be considered as a
		// successful match
		return false
	}

	// It must match both selectors.
	//
	// note: annotations are translated into labels here
	return labelSelector.Matches(labels.Set(obj.GetLabels())) &&
		annotationSelector.Matches(labels.Set(obj.GetAnnotations()))
}

func selectorMapKey(apiGroup, kind string) string {
	return fmt.Sprintf("%s.%s", kind, apiGroup)
}
