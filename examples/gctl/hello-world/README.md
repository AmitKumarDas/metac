## Hello World K8s controller

- This is a K8s controller that imports metac as library
  - In other words, this uses inline hooks than webhooks
- Controller's K8s dependencies is configured in a config file
  - Refer: config/config.yaml
- Controller business logic is implemented in Go
  - Refer: cmd/main.go
  - K8s is completely abstracted from this logic
  - Logic is implemented in respective reconcile functions
    - A Pod gets created via sync inline hook
    - This Pod gets deleted via finalize inline hook
- Docker image includes the binary as well as its config file
  - Refer: Dockerfile
- Controller is deployed as a single StatefulSet
  - No need of separate metac binary since metac is used as a library
  - Refer: helloworld-operator.yaml

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
# workstation needs to have Docker
make image

# tag the image to use the local registry
sudo docker tag hello-world:latest localhost:5000/hello-world:latest

# push to local registry configured to be used by kind
sudo docker push localhost:5000/hello-world:latest
```

```sh
# install namespace, rbac, crds & operator
sudo kubectl apply -f helloworld-ns.yaml
sudo kubectl apply -f helloworld-rbac-crd.yaml
sudo kubectl apply -f helloworld-operator.yaml

# verify if above were installed properly
sudo kubectl get ns
sudo kubectl get crd
sudo kubectl get sts -n hello-world
sudo kubectl describe po -n hello-world
sudo kubectl get po -n hello-world
sudo kubectl logs -n hello-world hello-world-0
```

### Test

```sh
# create the helloworld custom resource
sudo kubectl apply -f helloworld.yaml

# verify creation of Pod
sudo kubectl get pods -n hello-world

# delete helloworld custom resource
sudo kubectl delete -f helloworld.yaml

# verify deletion of Pod
sudo kubectl get pods -n hello-world

# verify deletion of helloworld
sudo kubectl get helloworlds -n hello-world
```

### Cleanup

```sh
sudo kubectl delete -f helloworld-operator.yaml
sudo kubectl delete -f helloworld-rbac-crd.yaml
sudo kubectl delete -f helloworld-ns.yaml
sudo kind delete cluster
```