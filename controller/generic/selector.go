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

package generic

import (
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/common/selector"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
)

// makeSelectorKeyFromAVK returns a formatted string suitable
// to be used as a key of form 'kind.apiVersion'
//
// The returned key is based on a combination of api version & kind
func makeSelectorKeyFromAVK(apiVersion, kind string) string {
	return fmt.Sprintf("%s.%s", kind, apiVersion)
}

// NameSelectorRegistrar acts as the registrar of NameSelectors
// anchored by apiversion and kind
type NameSelectorRegistrar map[string]v1alpha1.NameSelector

// Set registers the given NameSelector based on the given
// api version and kind
func (m NameSelectorRegistrar) Set(apiVersion, kind string, selector v1alpha1.NameSelector) {
	m[makeSelectorKeyFromAVK(apiVersion, kind)] = selector
}

// Get returns the NameSelector from the registrar based on the
// given api version and kind
func (m NameSelectorRegistrar) Get(apiVersion, kind string) v1alpha1.NameSelector {
	return m[makeSelectorKeyFromAVK(apiVersion, kind)]
}

// LabelSelectorRegistrar acts as the registrar of LabelSelectors
// anchored by api version and kind
type LabelSelectorRegistrar map[string]labels.Selector

// Set registers the given LabelSelector based on the given version
// and kind
func (m LabelSelectorRegistrar) Set(apiVersion, kind string, selector labels.Selector) {
	m[makeSelectorKeyFromAVK(apiVersion, kind)] = selector
}

// Get returns the LabelSelector from the registrar based on the
// given version and kind
func (m LabelSelectorRegistrar) Get(apiVersion, kind string) labels.Selector {
	return m[makeSelectorKeyFromAVK(apiVersion, kind)]
}

// AnnotationSelectorRegistrar acts as the registrar of
// AnnotationSelectors anchored by api version and kind
type AnnotationSelectorRegistrar map[string]labels.Selector

// Set registers the given AnnotationSelector based on the
// given api version and kind
func (m AnnotationSelectorRegistrar) Set(apiVersion, kind string, selector labels.Selector) {
	m[makeSelectorKeyFromAVK(apiVersion, kind)] = selector
}

// Get returns the AnnotationSelector from the registrar
// based on the given version and resource
func (m AnnotationSelectorRegistrar) Get(apiVersion, kind string) labels.Selector {
	return m[makeSelectorKeyFromAVK(apiVersion, kind)]
}

// AdvancedSelectorRegistrar acts as the registrar of
// selector.Evaluation anchored by api version and kind
type AdvancedSelectorRegistrar map[string]selector.Evaluation

// Set registers the given selector.Evaluation based on the
// given api version and kind
func (m AdvancedSelectorRegistrar) Set(apiVersion, kind string, selector selector.Evaluation) {
	m[makeSelectorKeyFromAVK(apiVersion, kind)] = selector
}

// Get returns the selector.Evaluation from the registrar
// based on the given version and resource
func (m AdvancedSelectorRegistrar) Get(apiVersion, kind string) selector.Evaluation {
	return m[makeSelectorKeyFromAVK(apiVersion, kind)]
}

// Selection holds various select strategies
type Selection struct {
	nameSelectorReg       NameSelectorRegistrar
	labelSelectorReg      LabelSelectorRegistrar
	annotationSelectorReg AnnotationSelectorRegistrar
	advancedSelectorReg   AdvancedSelectorRegistrar
}

// isDuplicateResource returns true if the given resource
// is present in the selectors already
func (s *Selection) isDuplicateResource(object *dynamicdiscovery.APIResource) bool {
	// Any of the selectors can be used. We are using
	// nameSelectors
	for avk := range s.nameSelectorReg {
		if avk == makeSelectorKeyFromAVK(object.APIVersion, object.Kind) {
			return true
		}
	}
	return false
}

func (s *Selection) setLabelSelectorOrEverything(
	gctlResource v1alpha1.GenericControllerResource,
	apiResource *dynamicdiscovery.APIResource,
) error {
	var err error
	// set label selector to pass always
	lblSelector := labels.Everything()
	// Convert resource's label selector to labels.Selector
	if gctlResource.LabelSelector != nil {
		lblSelector, err =
			metav1.LabelSelectorAsSelector(gctlResource.LabelSelector)
		if err != nil {
			return errors.Wrapf(
				err,
				"Label selector failed: %q: Version %q",
				gctlResource.Resource,
				gctlResource.APIVersion,
			)
		}
	}
	s.labelSelectorReg.Set(
		apiResource.APIVersion,
		apiResource.Kind,
		lblSelector,
	)
	return nil
}

func (s *Selection) setAnnotationSelectorOrEverything(
	gctlResource v1alpha1.GenericControllerResource,
	apiResource *dynamicdiscovery.APIResource,
) error {
	var err error
	// set annotation selector to pass always
	annSelector := labels.Everything()
	// convert annotation selector to
	// 1/ api labels selector, & then to
	// 2/ labels.Selector
	if gctlResource.AnnotationSelector != nil {
		lbls := &metav1.LabelSelector{
			MatchLabels:      gctlResource.AnnotationSelector.MatchAnnotations,
			MatchExpressions: gctlResource.AnnotationSelector.MatchExpressions,
		}
		annSelector, err = metav1.LabelSelectorAsSelector(lbls)
		if err != nil {
			return errors.Wrapf(
				err,
				"Annotation selector failed: %q: Version %q",
				gctlResource.Resource,
				gctlResource.APIVersion,
			)
		}
	}
	s.annotationSelectorReg.Set(
		apiResource.APIVersion,
		apiResource.Kind,
		annSelector,
	)
	return nil
}

func (s *Selection) setNameSelectorOrEverything(
	gctlResource v1alpha1.GenericControllerResource,
	apiResource *dynamicdiscovery.APIResource,
) error {
	nameSelector := gctlResource.NameSelector
	if nameSelector == nil {
		// empty nameselector passes always i.e. evaluates to
		// true for any given name
		nameSelector = []string{}
	}
	s.nameSelectorReg.Set(
		apiResource.APIVersion,
		apiResource.Kind,
		nameSelector,
	)
	return nil
}

func (s *Selection) setAdvancedSelectorOrEverything(
	gctlResource v1alpha1.GenericControllerResource,
	apiResource *dynamicdiscovery.APIResource,
) error {
	var terms []*v1alpha1.SelectorTerm
	if gctlResource.AdvancedSelector != nil {
		terms = gctlResource.AdvancedSelector.SelectorTerms
	}
	// NOTE:
	//	resource selector will pass always if there are no terms
	advancedSelector := selector.Evaluation{
		Terms: terms,
	}
	s.advancedSelectorReg.Set(
		apiResource.APIVersion,
		apiResource.Kind,
		advancedSelector,
	)
	return nil
}

// register registers the Selection instance with various
// selectors based on the provided GenericControllerResource
func (s *Selection) register(
	discoveryMgr *dynamicdiscovery.APIResourceManager,
	gctlResource v1alpha1.GenericControllerResource,
) error {
	var err error
	if discoveryMgr == nil {
		return errors.Errorf(
			"Selector init failed: Nil api resource discovery manager",
		)
	}
	if gctlResource.APIVersion == "" {
		return errors.Errorf(
			"Selector init failed: Missing gctl resource api version",
		)
	}
	if gctlResource.Resource == "" {
		return errors.Errorf(
			"Selector init failed: Missing gctl resource name",
		)
	}
	// fetch the resource from the discovered set
	apiResource := discoveryMgr.GetByResource(
		gctlResource.APIVersion,
		gctlResource.Resource,
	)
	if apiResource == nil {
		return errors.Errorf(
			"Selector init failed: Can't find %q: Version %q",
			gctlResource.Resource,
			gctlResource.APIVersion,
		)
	}
	if s.isDuplicateResource(apiResource) {
		return errors.Errorf(
			"Selector init failed: Duplicate resource %q: Version %q",
			gctlResource.Resource,
			gctlResource.APIVersion,
		)
	}
	// Set the Selectors
	var setters = []func(
		v1alpha1.GenericControllerResource,
		*dynamicdiscovery.APIResource,
	) error{
		s.setLabelSelectorOrEverything,
		s.setAnnotationSelectorOrEverything,
		s.setNameSelectorOrEverything,
		s.setAdvancedSelectorOrEverything,
	}
	for _, setter := range setters {
		err = setter(gctlResource, apiResource)
		if err != nil {
			return errors.Wrapf(err, "Selector init failed")
		}
	}
	return nil
}

// init all the selector strategies
func (s *Selection) init() {
	s.nameSelectorReg = NameSelectorRegistrar(make(map[string]v1alpha1.NameSelector))
	s.labelSelectorReg = LabelSelectorRegistrar(make(map[string]labels.Selector))
	s.annotationSelectorReg = AnnotationSelectorRegistrar(make(map[string]labels.Selector))
	s.advancedSelectorReg = AdvancedSelectorRegistrar(make(map[string]selector.Evaluation))
}

// NewSelectorForWatch returns a new instance of Selection
// based on watch
func NewSelectorForWatch(
	discoveryMgr *dynamicdiscovery.APIResourceManager,
	watch v1alpha1.GenericControllerResource,
) (*Selection, error) {
	s := &Selection{}
	s.init()
	err := s.register(discoveryMgr, watch)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// NewSelectorForAttachments returns a new instance of Selection
// based on the attachments
func NewSelectorForAttachments(
	discoveryMgr *dynamicdiscovery.APIResourceManager,
	attachments []v1alpha1.GenericControllerAttachment,
) (*Selection, error) {
	s := &Selection{}
	s.init()
	for _, attachment := range attachments {
		// register each attachment
		err := s.register(
			discoveryMgr,
			attachment.GenericControllerResource,
		)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// MatchLAN returns true if the provided instance matches following
// selector settings:
// - LabelSelector (L)
// - AnnotationSelector (A)
// - NameSelector (N)
func (s *Selection) MatchLAN(obj *unstructured.Unstructured) (bool, error) {
	if obj == nil {
		return false, errors.Errorf("Match failed: Nil target")
	}
	// Fetch appropriate selectors based on the given object's
	// api version & kind. Run matches from the extracted selectors
	nameSelector := s.nameSelectorReg.Get(obj.GetAPIVersion(), obj.GetKind())
	labelSelector := s.labelSelectorReg.Get(obj.GetAPIVersion(), obj.GetKind())
	annotationSelector := s.annotationSelectorReg.Get(obj.GetAPIVersion(), obj.GetKind())
	// All selector matches are **AND-ed**
	return labelSelector.Matches(labels.Set(obj.GetLabels())) &&
		annotationSelector.Matches(labels.Set(obj.GetAnnotations())) &&
		nameSelector.ContainsOrTrue(obj.GetName()), nil
}

// Match returns true if the provided instance matches following
// selector settings:
//
// - NameSelector
// - LabelSelector
// - AnnotationSelector
// - AdvancedSelector (minus reference match)
func (s *Selection) Match(obj *unstructured.Unstructured) (bool, error) {
	// lanMatch refers to label, annotation & name selector
	// based match result
	lanMatch, err := s.MatchLAN(obj)
	if err != nil {
		return false, err
	}
	// fetch advanced selector based on attachment
	// api version & kind
	sel := s.advancedSelectorReg.Get(
		obj.GetAPIVersion(),
		obj.GetKind(),
	)
	// make a copy
	var newsel = selector.Evaluation{
		Terms:  sel.Terms,
		Target: obj,
	}
	advanceMatch, err := newsel.RunMatch()
	if err != nil {
		return false, err
	}
	// All selector matches are **AND-ed**
	return lanMatch && advanceMatch, nil
}

// MatchAttachmentAgainstWatch returns true if the provided
// attachment match this selector settings for the provided
// watch
//
// Following selectors are executed:
// - NameSelector
// - LabelSelector
// - AnnotationSelector
// - AdvancedSelector (i.e. includes matching attachment against watch)
func (s *Selection) MatchAttachmentAgainstWatch(
	attachment *unstructured.Unstructured,
	watch *unstructured.Unstructured,
) (bool, error) {
	// lanMatch refers to label, annotation & name selector
	// based match result
	lanMatch, err := s.MatchLAN(attachment)
	if err != nil {
		return false, err
	}
	// fetch advanced selector based on attachment
	// api version & kind
	sel := s.advancedSelectorReg.Get(
		attachment.GetAPIVersion(),
		attachment.GetKind(),
	)
	// make a copy
	var newSel = selector.Evaluation{
		Terms:     sel.Terms,
		Target:    attachment,
		Reference: watch,
	}
	advanceMatch, err := newSel.RunMatch()
	if err != nil {
		return false, err
	}
	// All selector matches are **AND-ed**
	return lanMatch && advanceMatch, nil
}
