## BlueGreenDeployment

This is an example CompositeController that implements a custom rollout strategy based on a technique called Blue-Green Deployment.

The controller ramps up a completely separate ReplicaSet in the background for any change to the Pod template. It then waits for the new ReplicaSet to be fully Ready and Available (all Pods satisfy minReadySeconds), and then switches a Service to point to the new ReplicaSet. Finally, it scales down the old ReplicaSet.

### Prerequisites

* Install Metac from the manifests folder

```sh
# verify presence of metac CRDs
kubectl get crd

# verify presence of metac controller
kubectl get sts -n metac
```

### Deploy the BlueGreen controller

```sh
kubectl create configmap bluegreen-controller -n metac --from-file=sync.js

kubectl apply -f operator.yaml

# verify presence of BlueGreen CRD
kubectl get crd

# verify presence of BlueGreen controller deployment & service
kubectl get deploy -n metac
kubectl get svc -n metac
```

### Create a BlueGreenDeployment

```sh
kubectl apply -f my-bluegreen.yaml
```
