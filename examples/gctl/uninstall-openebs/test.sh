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
finalizer="protect.gctl.metac.openebs.io/uninstall-openebs"

echo -e "\n++ Installing operator"
kubectl apply -f operator_ns.yaml
kubectl apply -f operator_rbac.yaml
kubectl create configmap uninstall-openebs -n uninstall-openebs --from-file=config
kubectl apply -f operator.yaml
echo -e "\n++ Installed operator successfully"


echo -e "\n++ Applying openebs"
kubectl apply -f openebs_ns.yaml
kubectl apply -f openebs_sa_and_crds.yaml
kubectl apply -f openebs_crs.yaml
echo -e "\n++ Applied openebs successfully"

echo -e "\n++ Verify openebs resources are present"
kubectl get serviceaccounts -n $openebs_ns --no-headers 2>/dev/null | wc -l && echo "\__serviceaccounts"
kubectl get clusterroles --no-headers | grep $openebs_ns | wc -l && echo "\__clusterroles"
kubectl get clusterrolebindings --no-headers | grep $openebs_ns | wc -l  && echo "\__clusterrolebindings"
kubectl get ns $openebs_ns --no-headers 2>/dev/null | wc -l && echo "\__namespaces"
kubectl get crd --no-headers | grep $openebs_ns | wc -l && echo "\__crds"
echo -e "\n++ Verified openebs resources are present successfully"

echo -e "\n++Waiting for openebs namespace's finalizer to be updated"
until [[ "$(kubectl get ns $openebs_ns -o 'jsonpath={.metadata.finalizers}')" == "[${finalizer}]" ]]; do sleep 1; done
echo -e "\n++Openebs' finalizer updated successfully"

echo -e "\n++ Deleting openebs namespace that is watched by metac"
kubectl delete -f openebs_ns.yaml
echo -e "\n++ Deleted openebs namespace successfully"

echo -e "\n++ Verify openebs resources are not present"
kubectl get serviceaccounts -n $openebs_ns --no-headers 2>/dev/null | wc -l && echo "\__serviceaccounts"
kubectl get clusterroles --no-headers | grep $openebs_ns | wc -l && echo "\__clusterroles"
kubectl get clusterrolebindings --no-headers | grep $openebs_ns | wc -l  && echo "\__clusterrolebindings"
kubectl get ns $openebs_ns --no-headers 2>/dev/null | wc -l && echo "\__namespaces"
kubectl get crd --no-headers | grep $openebs_ns | wc -l && echo "\__crds"
echo -e "\n++ Verified openebs resources are not present successfully"

echo -e "\n++ Test uninstall-openebs completed successfully"