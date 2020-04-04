## DontPanic kubernetes controller

This is an example of a k8s controller that demonstrates its availability irrespective of an runtime error or due to misconfigurations. In other words, controller should not crash.

- This is a k8s controller that imports metac as library
  - This enables use of inline hooks instead of webhooks
- Controller's kubernetes resources are configured in a config file
  - Refer: config/config.yaml
- Controller business logic is implemented in Go
  - Refer: cmd/main.go
  - Kubernetes client libraries are completely abstracted from this logic
  - Logic is implemented in respective reconcile functions
    - A CR `IAmError` is sent as the response i.e. desired state
    - NOTE: `IAmError`'s definition i.e. _(CRD)_ is not set in k8s cluster
    - Hence this reconciliation should **error out**
- Expectations:
    - Binary should **not panic** inspite of above error
    - Log should provide the root cause of error
- Docker image includes the binary as well as its config file
  - Refer: Dockerfile
- Controller is deployed as a single StatefulSet
  - No need of separate metac binary since metac is imported as a library
  - Refer: dontpanic-operator.yaml

### Steps

```sh
# workstation needs to have Docker
# use kind to create a k8s cluster
#
# Refer: https://kind.sigs.k8s.io/docs/user/local-registry/
sudo ./kind-with-registry.sh

# cat $HOME/.kube/config
# connect to kind cluster
sudo kubectl cluster-info --context kind-kind

# debugging info if required
#
# Kubernetes master is running at https://127.0.0.1:32774
#
# KubeDNS is running at:
# https://127.0.0.1:32774/api/v1/namespaces/# kube-system/services/kube-dns:dns/proxy
#
# To further debug and diagnose cluster problems, use
#'kubectl cluster-info dump'.
```

```sh
# NOTE:
# - Docker daemon always runs as a root user
#   - sudo may not be required depending on individual confgurations
#   - sudo is needed if docker group is not configured
# - KIND runs entirely as containers
#   - Hence, all kubectl commands might need to used with sudo
```

```sh
# workstation needs to have Docker
make image

# tag the image to use the local registry
sudo docker tag dontpanic:latest localhost:5000/dontpanic:latest

# push to local registry configured to be used by kind
sudo docker push localhost:5000/dontpanic:latest
```

```sh
# install namespace, rbac, crds & operator
sudo kubectl apply -f dontpanic-ns.yaml
sudo kubectl apply -f dontpanic-rbac-crd.yaml
sudo kubectl apply -f dontpanic-operator.yaml

# verify if above were installed properly
sudo kubectl get ns
sudo kubectl get crd
sudo kubectl get sts -n dontpanic
sudo kubectl describe po -n dontpanic
sudo kubectl get po -n dontpanic
sudo kubectl logs -n dontpanic dontpanic-0
```

### Test

```sh
# check operator pod
sudo kubectl get pods -n dontpanic

# check operator pod logs
sudo kubectl logs -n dontpanic dontpanic-0

# create the dontpanic custom resource
sudo kubectl apply -f dontpanic.yaml

# check operator pod logs
sudo kubectl logs -n dontpanic dontpanic-0
```

### Observations
- Binary did not panic
- Following were the logs that points the root cause

```bash
I0402 15:33:10.482481       1 discovery.go:174] API resources discovery completed
I0402 15:33:10.651607       1 metacontroller.go:270] Condition failed: Will retry after 1s: Local GenericController: Failed to init dontpanic-controller: Local GenericController: Selector init failed: Can't find "iamerrors": Version "notsure.com/v1"
```

### Cleanup

```sh
sudo kubectl delete -f dontpanic.yaml
sudo kubectl delete -f dontpanic-operator.yaml
sudo kubectl delete -f dontpanic-rbac-crd.yaml
sudo kubectl delete -f dontpanic-ns.yaml

sudo kind delete cluster
```