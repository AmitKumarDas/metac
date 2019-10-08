#!/bin/bash

cleanup() {
  set +e
  
  echo ""

  echo "--------------------------"
  echo "++ Clean up started"
  echo "--------------------------"

  kubectl patch namespace amitd --type=merge -p '{"metadata":{"finalizers":[]}}' || true
  kubectl delete -f my-namespace.yaml || true
  kubectl delete -f operator.yaml || true
  kubectl delete configmap install-uninstall-crd -n metac || true
  kubectl delete crd storages.dao.amitd.io || true
  
  echo "--------------------------"
  echo "++ Clean up completed"
  echo "--------------------------"
}
trap cleanup EXIT

# Uncomment this if you want to run this script in debug mode
#set -ex

# my crd name
my_crd="storages.dao.amitd.io"

echo -e "\n++ Installing operator"
kubectl create configmap install-uninstall-crd -n metac --from-file=hooks
kubectl apply -f operator.yaml
echo -e "\n++ Installed operator successfully"


echo -e "\n++ Applying namespace that will get watched by metac - I"
kubectl apply -f my-namespace.yaml
echo -e "\n++ Applied namespace successfully"

echo -e "\n++ Waiting for CRD $my_crd creation..."
until kubectl get crd $my_crd; do sleep 1; done
echo -e "\n++ CRD $my_crd created successfully"

echo -e "\n++ Deleting namespace that is being watched by metac - II"
kubectl delete -f my-namespace.yaml
echo -e "\n++ Deleted namespace successfully"

echo -e "\n++ Waiting for CRD $my_crd deletion..."
until [[ "$(kubectl get crd $my_crd 2>&1)" == *NotFound* ]]; do sleep 1; done
echo -e "\n++ Deleted CRD $my_crd successfully"

echo -e "\n++ Applying namespace that will get watched by metac - III"
kubectl apply -f my-namespace.yaml
echo -e "\n++ Applied namespace successfully"

echo -e "\n++ Waiting for CRD $my_crd creation..."
until kubectl get crd $my_crd; do sleep 1; done
echo -e "\n++ CRD $my_crd created successfully"

echo -e "\n++ Deleting namespace that is being watched by metac - IV"
kubectl delete -f my-namespace.yaml
echo -e "\n++ Deleted namespace successfully"

echo -e "\n++ Waiting for CRD $my_crd deletion..."
until [[ "$(kubectl get crd $my_crd 2>&1)" == *NotFound* ]]; do sleep 1; done
echo -e "\n++ CRD $my_crd deleted successfully"

echo -e "\n++ Test install-uninstall-crd completed successfully..."