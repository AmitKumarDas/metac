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

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	"openebs.io/metac/controller/common"
	dynamicdiscovery "openebs.io/metac/dynamic/discovery"
)

type attachmentUpdateStrategyManager struct {
	// strategies holds update strategies of attachments
	// corresponding to GenericController
	strategies map[string]*v1alpha1.GenericControllerAttachmentUpdateStrategy

	// defaultMethod if any attachment does not have
	// an update strategy set
	defaultMethod v1alpha1.ChildUpdateMethod
}

// String implements Stringer interface
func (mgr attachmentUpdateStrategyManager) String() string {
	return "attachmentUpdateStrategyManager"
}

// GetByGKOrDefault returns the attachment update strategy based on
// the given api group & kind
func (mgr attachmentUpdateStrategyManager) GetByGKOrDefault(
	apiGroup, kind string,
) v1alpha1.ChildUpdateMethod {

	strategy := mgr.getByGK(apiGroup, kind)
	if strategy == nil || strategy.Method == "" {
		return mgr.defaultMethod
	}
	return strategy.Method
}

// get returns the controller's attachment's upgrade strategy
// based on the given api group & kind
func (mgr attachmentUpdateStrategyManager) getByGK(
	apiGroup, kind string,
) *v1alpha1.GenericControllerAttachmentUpdateStrategy {

	return mgr.strategies[makeUpdateStrategyKeyFromGK(apiGroup, kind)]
}

// makeUpdateStrategyKeyFromGK builds a key to be used in
// update strategy registry. This key is built out of the
// provided api group & kind.
func makeUpdateStrategyKeyFromGK(apiGroup, kind string) string {
	return fmt.Sprintf("%s.%s", kind, apiGroup)
}

// newAttachmentUpdateStrategyManager returns a new instance of
// attachmentUpdateStrategyManager.
func newAttachmentUpdateStrategyManager(
	resourceMgr *dynamicdiscovery.APIResourceManager,
	attachments []v1alpha1.GenericControllerAttachment,
) (*attachmentUpdateStrategyManager, error) {

	mgr := &attachmentUpdateStrategyManager{
		strategies: make(
			map[string]*v1alpha1.GenericControllerAttachmentUpdateStrategy,
		),
		defaultMethod: v1alpha1.ChildUpdateOnDelete,
	}

	for _, attachment := range attachments {
		// no need to store default strategy since no need to lookup later.
		// This can also remove the need to maintain the map of strategies
		// if all the attachments are not set with a strategy or set with
		// default strategy.
		if attachment.UpdateStrategy != nil &&
			attachment.UpdateStrategy.Method != mgr.defaultMethod {
			// this is done to map resource name to kind name
			resource := resourceMgr.GetByResource(attachment.APIVersion, attachment.Resource)
			if resource == nil {
				return nil, errors.Errorf(
					"%s: Can't find resource %s/%s",
					mgr,
					attachment.APIVersion,
					attachment.Resource,
				)
			}
			// Ignore API version.
			apiGroup, _ := common.ParseAPIVersionToGroupVersion(attachment.APIVersion)
			key := makeUpdateStrategyKeyFromGK(apiGroup, resource.Kind)
			mgr.strategies[key] = attachment.UpdateStrategy
		}
	}
	return mgr, nil
}
