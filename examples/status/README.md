## Noop

This is an example DecoratorController returning a status

### Prerequisites

* Install Metac from the manifests folder

```sh
# verify creation of CRDs
kubectl get crd
```

### Steps to deploy Noop controller

```sh
# This configmap embeds the sync hook file (a javascript impl).
# This hook file is injected into noop controller.
#
# NOTE:
#   Noop controller is a nodejs server exposing a http endpoint URL
# ending with '/sync'
kubectl create configmap noop-controller -n metacontroller --from-file=sync.js

# Deploy the Noop controller
kubectl apply -f operator.yaml

# Verify creation of CRD
kubectl get crd noops.metac.openebs.io -oyaml

# Verify creation of Noop operator
kubectl get deploy -n metac
kubectl get svc -n metac
kubectl get configmap -n metac
```

### Verify by creating a Noop resource

```sh
kubectl apply -f my-noop.yaml

# verify if status is set with 'success'
kubectl get noop -oyaml
```
