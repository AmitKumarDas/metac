## Sample Controller

This is Metacontroller implementation of SampleController found at https://github.com/kubernetes/sample-controller/. 

Sample controller provides a simple implementation of Kubernetes extension (i.e. custom controller). It defines a custom resource of kind `Foo`. When an instance of Foo gets applied against Kubernetes cluster, it is expected to create a Deployment instance. There should be a deployment instance corresponding to each instance of Foo.

This version of SampleController shows how a controller needs to be bother about constructing its desired state only. In other words, controller logic returns the desired 'Deployment' object from the observed Foo instance without bothering about inner details of Kubernetes. Metacontroller on its parts handles create, update & delete operations of the Deployment instance based on Foo instance's current state.

This example also provides a fair idea to write idempotent logic required to implement Kubernetes controllers.

### Prerequisites

* Install Metac from the repo's `manifests` folder
```sh
kubectl apply -f ../../../manifests/
```

```sh
# verify presence of metac CRDs
kubectl get crd

# verify presence of metac controller
kubectl get sts -n metac
```

### Deploy the sample controller

```sh
# embed javascript controller logic into a configmap
kubectl create configmap sample-controller -n metac --from-file=sync.js
# apply NodeJS image that mounts above javascript controller code
kubectl apply -f operator.yaml

# verify presence of Foo CRD
kubectl get crd | grep foos

# verify presence of Foo controller deployment & service
kubectl get deploy -n metac
kubectl get svc -n metac
```

### Create a Foo resource

```sh
kubectl apply -f foo.yaml
```

### Verify creation of resources
```sh
kubectl get foo
kubectl get deploy
kubectl get po
```

### Delete the Foo resource
```sh
kubectl delete -f foo.yaml
```

### Verify deletion of deployment object
```sh
kubectl get foo
kubectl get deploy
kubectl get po
```