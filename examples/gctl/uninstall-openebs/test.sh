#!/bin/bash

cleanup() {
  set +e
  
  echo ""

  echo "--------------------------"
  echo "++ Clean up started"
  echo "--------------------------"

  kubectl delete -f coolnerd.yaml || true
  kubectl delete -f operator.yaml || true
  kubectl delete -f rbac.yaml || true
  kubectl delete -f ns_and_crd.yaml || true
  
  echo "--------------------------"
  echo "++ Clean up completed"
  echo "--------------------------"
}
trap cleanup EXIT

# Uncomment this if you want to run this script in debug mode
#set -ex

# my cr name
my_res="cool-status-on-me"
my_ns="set-status-on-cr"

echo -e "\n++ Installing operator"
kubectl apply -f ns_and_crd.yaml
kubectl apply -f rbac.yaml
kubectl apply -f operator.yaml
echo -e "\n++ Installed operator successfully"


echo -e "\n++ Applying custom resource that gets watched by metac - I"
kubectl apply -f coolnerd.yaml
echo -e "\n++ Applied custom resource successfully"

echo -e "\n++Waiting for custom resource's status to be updated"
until [[ "$(kubectl -n $my_ns get coolnerd $my_res -o 'jsonpath={.status.phase}')" == "Active" ]]; do sleep 1; done
echo -e "\n++Custom resource's status updated successfully"

echo -e "\n++ Deleting custom resource that is watched by metac"
kubectl delete -f coolnerd.yaml
echo -e "\n++ Deleted custom resource successfully"

echo -e "\n++ Applying custom resource that gets watched by metac - II"
kubectl apply -f coolnerd.yaml
echo -e "\n++ Applied custom resource successfully"

echo -e "\n++Waiting for custom resource's status to be updated"
until [[ "$(kubectl -n $my_ns get coolnerd $my_res -o 'jsonpath={.status.phase}')" == "Active" ]]; do sleep 1; done
echo -e "\n++Custom resource's status updated successfully"

echo -e "\n++ Test set-status-on-cr completed successfully"