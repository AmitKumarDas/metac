## DynamicConfig controller

This is an example of a k8s controller that demonstrates implementation of **DynamicConfig** controller. This config works by merging values set by user or external tool against the defaults set by its controller. Values specified by user _(or external tool)_ will be prioritized over the controller set defaults. This controller also showcases forward & backward compatibility w.r.t its versions. 

- This is a k8s controller that imports metac as library
  - This enables use of inline hooks instead of webhooks
- Controller's kubernetes resources are configured in config/controller.yaml
- Controller business logic is implemented in Go
  - Refer: cmd/main.go
  - Kubernetes client libraries are completely abstracted from this logic
  - Logic is implemented in respective reconcile functions
- Expectations:
    - User provided values are always considered in `DynamicConfig` resource
    - Controller will set the unset fields with default values
    - `spec.version` is set to higher version to upgrade the config values
    - `spec.version` is set to lower version to downgrade the config values
    -  DynamicConfig fields get added or removed based on its version
- Docker image includes the binary as well as its config file
  - Refer: Dockerfile
- Controller is deployed as a single StatefulSet
  - No need of separate metac binary since metac is imported as a library
  - Refer: dynamicconfig-operator.yaml

### How it works

Controller makes use of 3 way merge logic to implement needs of DynamicConfig controller. 3 way merge uses `observed`, `last applied` & `desired state` to arrive at final state. Observed state is the state found in k8s cluster whereas desired state is the default values considered in controller logic.

if status.version == spec.version then
    F = D + O + O  (user values overrides the defaults)
if status.version != spec.version then
    F = O + O + D  (adds/removes fields w.r.t version & defaults overrides user values)
    F' = F + F + O (user values overrides the defaults)

### Steps

```sh
# workstation needs to have Docker
# use kind to create a k8s cluster
#
# Refer: https://kind.sigs.k8s.io/docs/user/local-registry/
sudo ../../kind-with-registry.sh

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
sudo docker tag dynamicconfig:latest localhost:5000/dynamicconfig:latest

# push to local registry configured to be used by kind
sudo docker push localhost:5000/dynamicconfig:latest
```

```sh
# install namespace, rbac, crds & operator
sudo kubectl apply -f dynamicconfig-ns.yaml
sudo kubectl apply -f dynamicconfig-rbac-crd.yaml
sudo kubectl apply -f dynamicconfig-operator.yaml

# verify if above were installed properly
sudo kubectl get ns
sudo kubectl get crd
sudo kubectl get sts -n dynamicconfig
sudo kubectl describe po -n dynamicconfig
sudo kubectl get po -n dynamicconfig
sudo kubectl logs -n dynamicconfig dynamicconfig-0
```

### Test

```sh
# check operator pod
sudo kubectl get pods -n dynamicconfig

# check operator pod logs
sudo kubectl logs -n dynamicconfig dynamicconfig-0

# create dynamicconfig custom resource
sudo kubectl apply -f dynamicconfig.yaml

# use version v1
# verify v1 fields
# update values
# verify if these values persist

# upgrade config to version v2
# verify v2 fields
# verify v1 specific fields are removed
# update values
# verify if these values persist

# downgrade config to version v1
# verify v1 fields
# verify v2 specific fields are removed
# update config values
# verify if these values persist

# upgrade config to version v3
# verify v3 fields
# verify v1 specific fields are removed
# update config values
# verify if these values persist

# upgrade config to version v2
# verify v2 fields
# verify v3 specific fields are removed
# update values
# verify if these values persist
```

### Cleanup

```sh
sudo kubectl delete -f dontpanic.yaml
sudo kubectl delete -f dontpanic-operator.yaml
sudo kubectl delete -f dontpanic-rbac-crd.yaml
sudo kubectl delete -f dontpanic-ns.yaml

sudo kind delete cluster
```