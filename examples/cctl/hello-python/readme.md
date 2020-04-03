## Example python Controller
This example is taken from [metacontroller](https://metacontroller.app/guide/create/) doc. There were some issue in this example if you are using `CompositeController` and status subresource is not enabled for parent resource then your controller will go into an infinite loop. For `CompositeController` it updates `status.observedGeneration` with `metadata.generation` for parent resource.

This will be done in 2 ways -

1. If status subresource is enabled for parent then it will patch the status with cr's status end point and  `metadata.generation` of parent will not change.
2. If status subresource is not enabled for parent then it will update the full object. For that there will be a change in `metadata.generation` of parent. Which will invoke a reconciliation and that reconciliation will invoke `metadata.generation` update in parent. So it will be a contineous loop.


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
  subresources:
    status: {}
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