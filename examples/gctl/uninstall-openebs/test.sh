#!/bin/bash

cleanup() {
  set +e
  
  echo ""

  echo "--------------------------"
  echo "++ Clean up started"
  echo "--------------------------"

  kubectl delete -f operator.yaml || true
  kubectl delete cm --all -n uninstall-openebs || true
  kubectl delete -f operator_rbac.yaml || true
  kubectl delete -f operator_ns.yaml || true

  echo "--------------------------"
  echo "++ Clean up completed"
  echo "--------------------------"
}
trap cleanup EXIT

# Uncomment this if you want to run this script in debug mode
#set -ex

# my cr name
my_ns="uninstall-openebs"
openebs_ns="openebs"

echo -e "\n++ Installing operator"
kubectl apply -f operator_ns.yaml
kubectl apply -f operator_rbac.yaml
kubectl apply -f operator.yaml
echo -e "\n++ Installed operator successfully"


echo -e "\n++ Applying openebs - I"
kubectl apply -f openebs_ns.yaml
kubectl apply -f openebs_sa_and_crds.yaml
kubectl apply -f openebs_crs.yaml
echo -e "\n++ Applied openebs successfully"

echo -e "\n++Waiting for openebs namespace's finalizer to be updated"
while [[ "$(kubectl get ns $openebs_ns -o 'jsonpath={.metadata.finalizers}')" == *protect.gctl.metac.openebs.io* ]]; do sleep 1; done
echo -e "\n++Openebs' finalizer updated successfully"

echo -e "\n++ Deleting openebs namespace that is watched by metac"
kubectl delete -f openebs_ns.yaml
echo -e "\n++ Deleted openebs namespace successfully"

echo -e "\n++ Verify openebs resources are not present"
kubectl get ns
kubectl get crd
kubectl get runtasks -n openebs
kubectl get castemplates
kubectl get serviceaccounts
kubectl get clusterroles
kubectl get clusterrolebindings
echo -e "\n++ Verified openebs resources are not present successfully"

echo -e "\n++ Test uninstall-openebs completed successfully"