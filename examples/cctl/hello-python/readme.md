## Example python Controller
This example is taken from [metacontroller](https://metacontroller.app/guide/create/) doc. There is an issue in this example that leads to continuous reconciliations. When _'CompositeController'_ is used with the parent resource's status **undefined** as a sub-resource, then the reconciliation gets into an infinite loop. This happens since _'CompositeController'_ updates _'status.observedGeneration'_ with _'metadata.generation'_ as part of its reconciliation.

Following sums up the reconciliation logic when status is defined as a sub resource & vice-versa:

1. If parent's status is defined as a sub resource then reconciliation logic patches this status using status api end point. This does not change resource's `metadata.generation` field.
2. If parent's status is not defined as a sub resource then reconciliation logic updates the full parent object. This in turn leads to a update in parent's `metadata.generation` field. This leads to re-triggering of sync hook. In other words this forces the reconciliation to get into a never ending loop.

**Solution**: Add below to watch's CRD
```yaml
  subresources:
    status: {}
```


What changes you need to do? 

You need to add below in your crd
```yaml
  subresources:
    status: {}
```
### Use
```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: helloworlds.example.com
spec:
  group: example.com
  version: v1
  names:
    kind: HelloWorld
    plural: helloworlds
    singular: helloworld
  scope: Namespaced
  subresources:
    status: {}
```
### Do not use
```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: helloworlds.example.com
spec:
  group: example.com
  version: v1
  names:
    kind: HelloWorld
    plural: helloworlds
    singular: helloworld
  scope: Namespaced
```

### To try this example follow the below steps 

Create namespace:
```bash
kubectl create namespace hello
```
Create helloworld crd:
```bash
kubectl apply -f crd.yaml
```
Create hello-controller:
```bash
kubectl apply -f controller.yaml
```
Create a configmap of python sync webhook:
```bash
kubectl -n hello create configmap hello-controller --from-file=sync.py
```
deploy the python webhook:
```bash
kubectl -n hello apply -f webhook.yaml
```
Create an new helloworld like below:
```yaml
apiVersion: example.com/v1
kind: HelloWorld
metadata:
  name: your-name
spec:
  who: Your Name
```
Check the pod is created or not [Pod and helloworld cr will be in same namespace and will have same name]
```bash
kubectl get pods -A
```
Check the log of the pod and try to create other helloworld cr or update any helloworld cr. Then check the pod and log.

### Cleanup
```bash
kubectl delete helloworld -A --all
kubectl ns hello
kubectl delete -f controller.yaml
kubectl delete -f crd.yaml
```