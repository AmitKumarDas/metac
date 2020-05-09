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
	"k8s.io/klog"

	dynamicclientset "openebs.io/metac/dynamic/clientset"
)

const (
	// APIGroup is the group used for CRDs created as part of the test.
	APIGroup = "test.metac.openebs.io"
	// APIVersion is the group-version used for CRDs created as part of the test.
	APIVersion = APIGroup + "/v1"
)

// UnstructOption provides a functional option to set
// unstructured instance fields
type UnstructOption func(u *unstructured.Unstructured)

// SetFinalizers sets given finalizers against the CR
func SetFinalizers(finalizers []string) UnstructOption {
	return func(u *unstructured.Unstructured) {
		u.SetFinalizers(finalizers)
	}
}

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

	//f.t.Logf("Creating %s CRD", kind)
	klog.V(2).Infof("Creating CRD %s", kind)
	crd, err := f.crdClient.CustomResourceDefinitions().Create(crd)
	if err != nil {
		f.t.Fatal(err)
	}
	klog.V(2).Infof("Created CRD %s", kind)

	// add to teardown functions
	f.addToTeardown(func() error {
		_, err := f.crdClient.
			CustomResourceDefinitions().
			Get(
				crd.GetName(),
				metav1.GetOptions{},
			)
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		return f.crdClient.CustomResourceDefinitions().Delete(crd.Name, nil)
	})

	klog.V(2).Infof("Discovering %s API", kind)
	err = f.Wait(func() (bool, error) {
		return apiDiscovery.GetAPIForAPIVersionAndResource(
			APIVersion,
			plural,
		) != nil, nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
	klog.V(2).Infof("Discovered %s API", kind)

	klog.V(2).Infof("Listing CRs for %s", kind)
	crClient, err := f.dynamicClientset.
		GetClientForAPIVersionAndResource(
			APIVersion,
			plural,
		)
	if err != nil {
		f.t.Fatal(err)
	}
	err = f.Wait(func() (bool, error) {
		_, err := crClient.List(metav1.ListOptions{})
		return err == nil, err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	klog.V(2).Infof("Listed CRs for %s", kind)

	return crd, crClient
}

// SetupNamespaceCRDAndItsCR will install a namespace scoped
// CRD, & then create corresponding resource of this CRD
func (f *Fixture) SetupNamespaceCRDAndItsCR(
	kind string,
	namespace string,
	name string,
	opts ...UnstructOption,
) (*v1beta1.CustomResourceDefinition,
	*dynamicclientset.ResourceClient,
	*unstructured.Unstructured,
) {
	// set up custom resource definition
	crd, resClient := f.SetupCRD(kind, v1beta1.NamespaceScoped)

	// set up corresponding custom resource
	obj := BuildUnstructObjFromCRD(crd, name)
	for _, o := range opts {
		o(obj)
	}

	obj, err := resClient.
		Namespace(namespace).
		Create(
			obj,
			metav1.CreateOptions{},
		)
	if err != nil {
		f.t.Fatal(err)
	}

	// add to teardown functions
	f.addToTeardown(func() error {
		_, err := resClient.
			Namespace(namespace).
			Get(
				obj.GetName(),
				metav1.GetOptions{},
			)
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		return resClient.
			Namespace(namespace).
			Delete(
				obj.GetName(),
				&metav1.DeleteOptions{},
			)
	})

	return crd, resClient, obj
}

// SetupClusterCRDAndItsCR will install a cluster scoped CRD,
// then create corresponding resource of this CRD
func (f *Fixture) SetupClusterCRDAndItsCR(
	kind string,
	name string,
	opts ...UnstructOption,
) (*v1beta1.CustomResourceDefinition,
	*dynamicclientset.ResourceClient,
	*unstructured.Unstructured,
) {

	crd, resClient := f.SetupCRD(kind, v1beta1.ClusterScoped)
	obj := BuildUnstructObjFromCRD(crd, name)
	for _, o := range opts {
		o(obj)
	}

	obj, err := resClient.Create(obj, metav1.CreateOptions{})
	if err != nil {
		f.t.Fatal(err)
	}

	// add to teardown functions
	f.addToTeardown(func() error {
		_, err := resClient.Get(obj.GetName(), metav1.GetOptions{})
		if err != nil && apierrors.IsNotFound(err) {
			return nil
		}
		return resClient.Delete(obj.GetName(), &metav1.DeleteOptions{})
	})

	return crd, resClient, obj
}
