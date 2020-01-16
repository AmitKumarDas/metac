#!/bin/bash

cleanup() {
  set +e
  echo ""

  echo "--------------------------"
  echo "++ Clean up started"
  echo "--------------------------"

  kubectl delete -f foo.yaml || true
  kubectl delete -f operator.yaml || true
  kubectl delete configmap sample-controller -n metac || true

  echo "--------------------------"
  echo "++ Clean up completed"
  echo "--------------------------"
}

# Comment below if you want to check manually
# the state of the cluster and intended resources
trap cleanup EXIT

# Uncomment below if debug / verbose execution is needed
#set -ex

my_crd="foos.samplecontroller.k8s.io"
my_deploy="my-deploy"

echo -e "\n Install SampleController..."
kubectl create configmap sample-controller -n metac --from-file=sync.js
kubectl apply -f operator.yaml

echo -e "\n Wait until SampleController $my_crd is available..."
until kubectl get $my_crd; do sleep 1; done

echo -e "\n Create a Foo object..."
kubectl apply -f foo.yaml

echo -e "\n Wait for Foo deployment to be available..."
until kubectl get deploy $my_deploy; do sleep 1; done

echo -e "\n Wait for Foo deployment's pods to be available..."
until [[ "$(kubectl get deploy $my_deploy -o 'jsonpath={.status.availableReplicas}')" -eq 3 ]]; do sleep 1; done

echo -e "\n Delete the Foo object..."
kubectl delete -f foo.yaml

echo -e "\n++ Waiting for Foo Deployment deletion..."
until [[ "$(kubectl get deploy $my_deploy 2>&1)" == *NotFound* ]]; do sleep 1; done