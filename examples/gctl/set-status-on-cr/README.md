## Set status on Custom Resource

This is an example of a single binary Kubernetes controller. The controller logic is implemented in Go and packaged in a container. This controller will wait for the `CoolNerd` custom resource in $NAMESPACE to be created. Once created, it will update the CR Status. In this case, go code and config are the main files to be understood.

### Prerequisites

* Kubernetes 1.8+ is recommended for its improved CRD support, especially garbage collection.
* Install following yamls

```sh
kubectl apply -f ns_and_crd.yaml
kubectl apply -f rbac.yaml
kubectl apply -f operator.yaml

# verify if above was installed properly
kubectl get ns
kubectl get crd
kubectl get deployment -n set-status-on-cr
```

### Create the Custom Resource

```sh
kubectl apply -f coolnerd.yaml
```

Watch for the CR to get created with status

```sh
kubectl get coolnerds -n set-status-on-cr -oyaml
```

### Cleanup

```sh
kubectl delete -f coolnerd.yaml
kubectl delete -f operator.yaml
kubectl delete -f rbac.yaml
kubectl delete -f ns_and_crd.yaml
```