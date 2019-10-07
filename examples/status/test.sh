#!/bin/bash

cleanup() {
  set +e
  echo ""

  echo "--------------------------"
  echo "++ Clean up started"
  echo "--------------------------"

  kubectl delete -f my-noop.yaml
  kubectl delete rs,svc -l app=noop-controller 
  kubectl delete -f operator.yaml
  kubectl delete configmap noop-controller -n metac

  echo "--------------------------"
  echo "++ Clean up completed"
  echo "--------------------------"
}
# Comment below if you want to check manually
# the state of the cluster and intended resources
trap cleanup EXIT

# Uncomment below if debug / verbose execution is needed
#set -ex

# noop crd name
np="noops.metac.openebs.io"

echo -e "\n++Will install Noop operator..."
kubectl create configmap noop-controller -n metac --from-file=sync.js
kubectl apply -f operator.yaml

echo -e "\n++Wait until CRD is available..."
until kubectl get $np; do sleep 1; done

echo -e "\n++Will apply Noop resource..."
kubectl apply -f my-noop.yaml

echo -e "\n++Wait for Noop resource's status to be updated..."
until [[ "$(kubectl get $np noop -o 'jsonpath={.status.message}')" == "success" ]]; do sleep 1; done
