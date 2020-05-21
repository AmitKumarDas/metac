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

package framework

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	metac "openebs.io/metac/apis/metacontroller/v1alpha1"
	dynamicapply "openebs.io/metac/dynamic/apply"
)

// Apply represents the desired state that needs to
// be applied against the cluster
type Apply struct {
	// Desired state that needs to be created or
	// updated or deleted. Resource gets created if
	// this state is not observed in the cluster.
	// However, if this state is found in the cluster,
	// then the corresponding resource gets updated
	// via a 3-way merge.
	State *unstructured.Unstructured `json:"state"`

	// Desired count that needs to be created
	//
	// NOTE:
	//	If value is 0 then this state needs to be
	// deleted
	Replicas *int `json:"replicas,omitempty"`

	// Resources that needs to be **updated** with above
	// desired state
	//
	// NOTE:
	//	Presence of Targets implies an update operation
	Targets metac.ResourceSelector `json:"targets,omitempty"`
}

// String implements the Stringer interface
func (a Apply) String() string {
	raw, err := json.MarshalIndent(
		a,
		" ",
		".",
	)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// ApplyResultPhase is a typed definition to determine the
// result of executing an apply
type ApplyResultPhase string

const (
	// ApplyResultPassed indicates that the apply operation
	// was successful
	ApplyResultPassed ApplyResultPhase = "Passed"

	// ApplyResultWarning indicates that the apply operation
	// resulted into warning
	ApplyResultWarning ApplyResultPhase = "Warning"

	// ApplyResultFailed indicates that the apply operation
	// was not successful
	ApplyResultFailed ApplyResultPhase = "Failed"
)

// ToTestStepResultPhase transforms ApplyResultPhase to TestResultPhase
func (phase ApplyResultPhase) ToTestStepResultPhase() TestStepResultPhase {
	switch phase {
	case ApplyResultPassed:
		return TestStepResultPassed
	case ApplyResultFailed:
		return TestStepResultFailed
	case ApplyResultWarning:
		return TestStepResultWarning
	default:
		return ""
	}
}

// ApplyResult holds the result of the apply operation
type ApplyResult struct {
	Phase   ApplyResultPhase `json:"phase"`
	Message string           `json:"message,omitempty"`
	Verbose string           `json:"verbose,omitempty"`
	Warning string           `json:"warning,omitempty"`
}

// Applyable helps applying desired state(s) against the cluster
type Applyable struct {
	*Fixture
	Retry *Retryable
	Apply Apply

	result *ApplyResult
	err    error
}

// ApplyableConfig helps in creating new instance of Applyable
type ApplyableConfig struct {
	Fixture *Fixture
	Retry   *Retryable
	Apply   Apply
}

// NewApplier returns a new instance of Applyable
func NewApplier(config ApplyableConfig) *Applyable {
	return &Applyable{
		Apply:   config.Apply,
		Fixture: config.Fixture,
		Retry:   config.Retry,
		result:  &ApplyResult{},
	}
}

func (a *Applyable) postCreateCRD(
	crd *v1beta1.CustomResourceDefinition,
) error {
	message := fmt.Sprintf(
		"PostCreate CRD: GVK %s %s",
		crd.Spec.Group+"/"+crd.Spec.Version,
		crd.Spec.Names.Singular,
	)
	// discover custom resource API
	err := a.Retry.Waitf(
		func() (bool, error) {
			got := APIDiscovery.GetAPIForAPIVersionAndResource(
				crd.Spec.Group+"/"+crd.Spec.Version,
				crd.Spec.Names.Plural,
			)
			if got == nil {
				return false, errors.Errorf(
					"Failed to discover %s %s",
					crd.Spec.Group+"/"+crd.Spec.Version,
					crd.Spec.Names.Plural,
				)
			}
			// fetch dynamic client for the custom resource
			// corresponding to this CRD
			customResourceClient, err := a.dynamicClientset.
				GetClientForAPIVersionAndResource(
					crd.Spec.Group+"/"+crd.Spec.Version,
					crd.Spec.Names.Plural,
				)
			if err != nil {
				return false, err
			}
			_, err = customResourceClient.List(metav1.ListOptions{})
			if err != nil {
				return false, err
			}
			return true, nil
		},
		message,
	)
	return err
}

func (a *Applyable) createCRD() (*ApplyResult, error) {
	var crd *v1beta1.CustomResourceDefinition
	err := unstructToTyped(a.Apply.State, &crd)
	if err != nil {
		return nil, err
	}
	// use crd client to create crd
	crd, err = a.crdClient.
		CustomResourceDefinitions().
		Create(crd)
	if err != nil {
		return nil, err
	}
	// add to teardown functions
	a.addToTeardown(func() error {
		_, err := a.crdClient.
			CustomResourceDefinitions().
			Get(
				crd.GetName(),
				metav1.GetOptions{},
			)
		if err != nil && apierrors.IsNotFound(err) {
			// nothing to do
			return nil
		}
		return a.crdClient.
			CustomResourceDefinitions().
			Delete(
				crd.Name,
				nil,
			)
	})
	// run an additional step to wait till this CRD
	// is discovered at apiserver
	err = a.postCreateCRD(crd)
	if err != nil {
		return nil, err
	}
	return &ApplyResult{
		Phase: ApplyResultPassed,
		Message: fmt.Sprintf(
			"Create CRD: GVK %s %s",
			crd.Spec.Group+"/"+crd.Spec.Version,
			crd.Spec.Names.Singular,
		),
	}, nil
}

func (a *Applyable) updateCRD() (*ApplyResult, error) {
	var crd *v1beta1.CustomResourceDefinition
	// transform to typed CRD to make use of crd client
	err := unstructToTyped(a.Apply.State, &crd)
	if err != nil {
		return nil, err
	}
	// get the CRD observed at the cluster
	target, err := a.crdClient.
		CustomResourceDefinitions().
		Get(
			a.Apply.State.GetName(),
			metav1.GetOptions{},
		)
	if err != nil {
		return nil, err
	}
	// tansform back to unstruct type to run 3-way merge
	targetAsUnstruct, err := typedToUnstruct(target)
	if err != nil {
		return nil, err
	}
	merged := &unstructured.Unstructured{}
	// 3-way merge
	merged.Object, err = dynamicapply.Merge(
		targetAsUnstruct.UnstructuredContent(), // observed
		a.Apply.State.UnstructuredContent(),    // last applied
		a.Apply.State.UnstructuredContent(),    // desired
	)
	if err != nil {
		return nil, err
	}
	// transform again to typed CRD to execute update
	err = unstructToTyped(merged, crd)
	if err != nil {
		return nil, err
	}
	// update the final merged state of CRD
	//
	// NOTE:
	//	At this point we are performing a server side
	// apply against the CRD
	_, err = a.crdClient.
		CustomResourceDefinitions().
		Update(
			crd,
		)
	if err != nil {
		return nil, err
	}
	return &ApplyResult{
		Phase: ApplyResultPassed,
		Message: fmt.Sprintf(
			"Update CRD: GVK %s %s",
			crd.Spec.Group+"/"+crd.Spec.Version,
			crd.Spec.Names.Singular,
		),
	}, nil
}

func (a *Applyable) applyCRD() (*ApplyResult, error) {
	var crd *v1beta1.CustomResourceDefinition
	err := unstructToTyped(a.Apply.State, &crd)
	if err != nil {
		return nil, err
	}
	message := fmt.Sprintf(
		"Apply CRD: GVK %s %s",
		crd.Spec.Group+"/"+crd.Spec.Version,
		crd.Spec.Names.Singular,
	)
	// use crd client to get crd
	err = a.Retry.Waitf(
		func() (bool, error) {
			_, err = a.crdClient.
				CustomResourceDefinitions().
				Get(
					crd.GetName(),
					metav1.GetOptions{},
				)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// condition exits since this is valid
					return true, err
				}
				return false, err
			}
			return true, nil
		},
		message,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// this is a **create** operation
			return a.createCRD()
		}
		return nil, err
	}
	// this is an **update** operation
	return a.updateCRD()
}

func (a *Applyable) createResource() (*ApplyResult, error) {
	var message = fmt.Sprintf(
		"Create resource %s %s: GVK %s",
		a.Apply.State.GetNamespace(),
		a.Apply.State.GetName(),
		a.Apply.State.GroupVersionKind(),
	)
	client, err := a.dynamicClientset.
		GetClientForAPIVersionAndKind(
			a.Apply.State.GetAPIVersion(),
			a.Apply.State.GetKind(),
		)
	if err != nil {
		return nil, err
	}
	_, err = client.
		Namespace(a.Apply.State.GetNamespace()).
		Create(
			a.Apply.State,
			metav1.CreateOptions{},
		)
	if err != nil {
		return nil, err
	}
	a.addToTeardown(func() error {
		_, err := client.
			Namespace(a.Apply.State.GetNamespace()).
			Get(
				a.Apply.State.GetName(),
				metav1.GetOptions{},
			)
		if err != nil && apierrors.IsNotFound(err) {
			// nothing to do since resource is already deleted
			return nil
		}
		return client.
			Namespace(a.Apply.State.GetNamespace()).
			Delete(
				a.Apply.State.GetName(),
				&metav1.DeleteOptions{},
			)
	})
	return &ApplyResult{
		Phase:   ApplyResultPassed,
		Message: message,
	}, nil
}

func (a *Applyable) updateResource() (*ApplyResult, error) {
	var message = fmt.Sprintf(
		"Update resource %s %s: GVK %s",
		a.Apply.State.GetNamespace(),
		a.Apply.State.GetName(),
		a.Apply.State.GroupVersionKind(),
	)
	err := a.Retry.Waitf(
		func() (bool, error) {
			// get appropriate dynamic client
			client, err := a.dynamicClientset.
				GetClientForAPIVersionAndKind(
					a.Apply.State.GetAPIVersion(),
					a.Apply.State.GetKind(),
				)
			if err != nil {
				return false, err
			}
			// get the resource from cluster to update
			target, err := client.
				Namespace(a.Apply.State.GetNamespace()).
				Get(
					a.Apply.State.GetName(),
					metav1.GetOptions{},
				)
			if err != nil {
				return false, err
			}
			merged := &unstructured.Unstructured{}
			// 3-way merge
			merged.Object, err = dynamicapply.Merge(
				target.UnstructuredContent(),        // observed
				a.Apply.State.UnstructuredContent(), // last applied
				a.Apply.State.UnstructuredContent(), // desired
			)
			if err != nil {
				return false, err
			}
			// update the final merged state
			//
			// NOTE:
			//	At this point we are performing a server
			// side apply against the resource
			_, err = client.
				Namespace(a.Apply.State.GetNamespace()).
				Update(
					merged,
					metav1.UpdateOptions{},
				)
			if err != nil {
				return false, err
			}
			return true, nil
		},
		message,
	)
	if err != nil {
		return nil, err
	}
	return &ApplyResult{
		Phase:   ApplyResultPassed,
		Message: message,
	}, nil
}

func (a *Applyable) applyResource() (*ApplyResult, error) {
	message := fmt.Sprintf(
		"Apply resource %s %s: GVK %s",
		a.Apply.State.GetNamespace(),
		a.Apply.State.GetName(),
		a.Apply.State.GroupVersionKind(),
	)
	err := a.Retry.Waitf(
		func() (bool, error) {
			var err error
			client, err := a.dynamicClientset.
				GetClientForAPIVersionAndKind(
					a.Apply.State.GetAPIVersion(),
					a.Apply.State.GetKind(),
				)
			if err != nil {
				return false, err
			}
			_, err = client.
				Namespace(a.Apply.State.GetNamespace()).
				Get(
					a.Apply.State.GetName(),
					metav1.GetOptions{},
				)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// condition exits since this is valid
					return true, err
				}
				return false, err
			}
			return true, nil
		},
		message,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// this is a **create** operation
			return a.createResource()
		}
		return nil, err
	}
	// this is an **update** operation
	return a.updateResource()
}

// Run executes applying the desired state against the
// cluster
func (a *Applyable) Run() (*ApplyResult, error) {
	if a.Apply.State.GetKind() == "CustomResourceDefinition" {
		// swtich to applying CRD
		return a.applyCRD()
	}
	return a.applyResource()
}
