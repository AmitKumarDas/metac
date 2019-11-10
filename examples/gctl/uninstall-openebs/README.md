## Clean un-install OpenEBS controller

This is an example of a single binary Kubernetes controller. The controller logic is implemented in Go and packaged in a container. This controller gets triggered on deletion of namespace called `openebs` or deletion of a CRD called `CASTemplate` if this controller was deployed after the deletion of `openebs` namespace. In other words, this controller is also capable of uninstalling openebs which is stuck in deleting namespace state.

### Features of this Kubernetes controller
- This controller takes care of cases when custom resources are set with finalizers and there is no corresponding pod(s) that are originally responsible to clear these finalizers.
- main.go and config/ are the important files to be understood w.r.t business logic.
- Kubernetes code related to watcher, informer, lister, client-go are all abstracted away by metac (which is used/imported as a library).
- The security blast radius is small & is limited to OpenEBS namespace & related custom resources only.

### Prerequisites

* Kubernetes 1.8+ is recommended for its improved CRD support, especially garbage collection.
* Install following yamls

```sh
kubectl apply -f operator_ns.yaml
kubectl apply -f operator_rbac.yaml
kubectl create configmap uninstall-openebs -n uninstall-openebs --from-file=config
kubectl apply -f operator.yaml

# verify if above were installed properly
kubectl get ns
kubectl get crd
kubectl get cm -n uninstall-openebs
kubectl get deployment -n uninstall-openebs
```

### Create OpenEBS namespace & related resources

```sh
kubectl apply -f openebs_ns_and_crds.yaml
kubectl apply -f openebs_crs.yaml
```

Wait till OpenEBS namespace is set with metac finalizers
```sh
kubectl get ns openebs -oyaml
```

Delete OpenEBS namespace that in turn triggers this controller
```sh
kubectl delete ns openebs
```

Verify if all OpenEBS CRD & CRs are deleted
```sh
kubectl get ns
kubectl get crd
kubectl get runtasks -n openebs
kubectl get castemplates
kubectl get serviceaccounts
kubectl get clusterroles
kubectl get clusterrolebindings
```

### Cleanup

```sh
kubectl delete -f operator.yaml
kubectl delete cm --all -n uninstall-openebs
kubectl delete -f operator_rbac.yaml
kubectl delete -f operator_ns.yaml
```