## Install-Uninstall-CRD

This is an example of GenericController that adds a CRD for a given Namespace.
This also removes the CRD when the given Namespace is deleted.

### Prerequisites

* Kubernetes 1.8+ is recommended for its improved CRD support, especially garbage collection.
* Install Metac using yamls from manifests folder

```sh
# cd to metac's root folder

kubectl apply -f ./manifests/metacontroller-namespace.yaml
kubectl apply -f ./manifests/metacontroller-rbac.yaml
kubectl apply -f ./manifests/metacontroller.yaml

# verify if metac was installed properly
kubectl get crd
kubectl get sts -n metac
```

### Deploy the Controllers

```sh
# cd to examples/gctl/install-uninstall-crd/

# any change in any of the hooks should be accompanied
# by deleting 1/ configmap & 2/ operator & 3/ re-applying both
kubectl create configmap install-uninstall-crd -n metac --from-file=hooks
kubectl apply -f operator.yaml

# verify the deploy, svc & configmap of this example
kubectl get gctl
kubectl get cm -n metac
kubectl get deploy -n metac
kubectl get svc -n metac
```

### Create the Namespace

```sh
kubectl apply -f my-namespace.yaml
```

Watch for the CRD to get created:

```sh
kubectl get crds --watch

kubectl get crd storages.dao.amitd.io
```

Check that the CRD get cleaned up when this Namespace is deleted:

```sh
kubectl delete -f my-namespace.yaml
kubectl get crds --watch
```