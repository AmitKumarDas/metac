/*
Copyright 2019 Google Inc.
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
	"fmt"
	"strings"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	dynamicclientset "openebs.io/metac/dynamic/clientset"
)

const (
	// APIGroup is the group used for CRDs created as part of the test.
	APIGroup = "test.metac.openebs.io"
	// APIVersion is the group-version used for CRDs created as part of the test.
	APIVersion = APIGroup + "/v1"
)

// SetupCRD generates a quick-and-dirty CRD for use in tests,
// and installs it in the test environment's API server.
//
// It accepts the singular name of the resource i.e. kind and
// this resource's scope i.e. whether this resource is namespace
// scoped or cluster scoped.
//
// NOTE:
//	This method takes care of teardown as well
func (f *Fixture) SetupCRD(
	kind string,
	scope v1beta1.ResourceScope,
) (*v1beta1.CustomResourceDefinition, *dynamicclientset.ResourceClient) {

	// singular name must be lower cased
	singular := strings.ToLower(kind)

	// this may not work always. For example singular StorageClass
	// becomes storageclasses. Hence, this method is annotated as
	// quick & dirty logic
	plural := singular + "s"

	crd := &v1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s.%s", plural, APIGroup),
		},
		Spec: v1beta1.CustomResourceDefinitionSpec{
			Group: APIGroup,
			Scope: scope,
			Names: v1beta1.CustomResourceDefinitionNames{
				Singular: singular,
				Plural:   plural,
				Kind:     kind,
			},
			Versions: []v1beta1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
				},
			},
		},
	}

	f.t.Logf("Creating %s CRD", kind)

	crd, err := f.crdClient.CustomResourceDefinitions().Create(crd)
	if err != nil {
		f.t.Fatal(err)
	}

	// add to teardown functions
	f.addToTeardown(func() error {
		_, err := f.crdClient.CustomResourceDefinitions().Get(crd.GetName(), metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		return f.crdClient.CustomResourceDefinitions().Delete(crd.Name, nil)
	})

	f.t.Logf("Discovering %s CRD server API", kind)
	err = f.Wait(func() (bool, error) {
		return resourceManager.GetByResource(APIVersion, plural) != nil, nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Logf("Discovered %s CRD server API", kind)

	crdClient, err := f.dynamicClientset.GetClientByResource(APIVersion, plural)
	if err != nil {
		f.t.Fatal(err)
	}

	f.t.Logf("Listing CRDs")
	err = f.Wait(func() (bool, error) {
		_, err := crdClient.List(metav1.ListOptions{})
		return err == nil, err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Logf("Listed CRDs")

	f.t.Logf("Created %s CRD", kind)
	return crd, crdClient
}

// SetupNSCRDAndDeployOneCR will install a namespace scoped
// CRD, & then create corresponding resource of this CRD
func (f *Fixture) SetupNSCRDAndDeployOneCR(
	kind string,
	namespace string,
	name string,
) (*v1beta1.CustomResourceDefinition,
	*dynamicclientset.ResourceClient,
	*unstructured.Unstructured,
) {
	crd, resClient := f.SetupCRD(kind, v1beta1.NamespaceScoped)

	res := BuildUnstructObjFromCRD(crd, name)

	res, err := resClient.Namespace(namespace).Create(res, metav1.CreateOptions{})
	if err != nil {
		f.t.Fatal(err)
	}

	// add to teardown functions
	f.addToTeardown(func() error {
		_, err := resClient.Namespace(namespace).Get(res.GetName(), metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		return resClient.Namespace(namespace).Delete(res.GetName(), &metav1.DeleteOptions{})
	})

	return crd, resClient, res
}

// SetupCRDAndDeployOneCR will install a cluster scoped CRD,
// then create corresponding resource of this CRD
func (f *Fixture) SetupCRDAndDeployOneCR(
	kind string,
	name string,
) (*v1beta1.CustomResourceDefinition,
	*dynamicclientset.ResourceClient,
	*unstructured.Unstructured,
) {

	crd, resClient := f.SetupCRD(kind, v1beta1.ClusterScoped)

	res := BuildUnstructObjFromCRD(crd, name)

	res, err := resClient.Create(res, metav1.CreateOptions{})
	if err != nil {
		f.t.Fatal(err)
	}

	// add to teardown functions
	f.addToTeardown(func() error {
		_, err := resClient.Get(res.GetName(), metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		return resClient.Delete(res.GetName(), &metav1.DeleteOptions{})
	})

	return crd, resClient, res
}
