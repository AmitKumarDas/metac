#!/bin/bash

cleanup() {
  set +e
  echo ""

  echo "--------------------------"
  echo "++ Clean up started"
  echo "--------------------------"

  kubectl delete -f my-bluegreen.yaml
  kubectl delete rs,svc -l app=nginx,component=frontend
  kubectl delete -f operator.yaml
  kubectl delete configmap bluegreen-controller -n metac

  echo "--------------------------"
  echo "++ Clean up completed"
  echo "--------------------------"
}

# Comment below if you want to check manually
# the state of the cluster and intended resources
trap cleanup EXIT

# Uncomment below if debug / verbose execution is needed
#set -ex

bgd="bluegreendeployments.ctl.enisoc.com"

echo -e "\n Install BlueGreen controller..."
kubectl create configmap bluegreen-controller -n metac --from-file=sync.js
kubectl apply -f operator.yaml

echo -e "\n Wait until BlueGreen CRD is available..."
until kubectl get $bgd; do sleep 1; done

echo -e "\n Create a BlueGreen object..."
kubectl apply -f my-bluegreen.yaml

# TODO(juntee): change observedGeneration steps to compare against generation number when k8s 1.10 and earlier retire.

echo -e "\n Wait for nginx-blue RS to be active..."
until [[ "$(kubectl get rs nginx-blue -o 'jsonpath={.status.readyReplicas}')" -eq 3 ]]; do sleep 1; done
until [[ "$(kubectl get rs nginx-green -o 'jsonpath={.status.replicas}')" -eq 0 ]]; do sleep 1; done
until [[ "$(kubectl get $bgd nginx -o 'jsonpath={.status.activeColor}')" -eq "blue" ]]; do sleep 1; done
until [[ "$(kubectl get $bgd nginx -o 'jsonpath={.status.active.availableReplicas}')" -eq 3 ]]; do sleep 1; done
until [[ "$(kubectl get $bgd nginx -o 'jsonpath={.status.observedGeneration}')" -eq "$(kubectl get $bgd nginx -o 'jsonpath={.metadata.generation}')" ]]; do sleep 1; done


echo -e "\n Trigger a rollout..."
kubectl patch $bgd nginx --type=merge -p '{"spec":{"template":{"metadata":{"labels":{"new":"label"}}}}}'

echo -e "\n Wait for nginx-green RS to be active..."
until [[ "$(kubectl get rs nginx-green -o 'jsonpath={.status.readyReplicas}')" -eq 3 ]]; do sleep 1; done
until [[ "$(kubectl get rs nginx-blue -o 'jsonpath={.status.replicas}')" -eq 0 ]]; do sleep 1; done
until [[ "$(kubectl get $bgd nginx -o 'jsonpath={.status.activeColor}')" -eq "green" ]]; do sleep 1; done
until [[ "$(kubectl get $bgd nginx -o 'jsonpath={.status.active.availableReplicas}')" -eq 3 ]]; do sleep 1; done
until [[ "$(kubectl get $bgd nginx -o 'jsonpath={.status.observedGeneration}')" -eq "$(kubectl get $bgd nginx -o 'jsonpath={.metadata.generation}')" ]]; do sleep 1; done

echo -e "\n Trigger another rollout..."
kubectl patch $bgd nginx --type=merge -p '{"spec":{"template":{"metadata":{"labels":{"new2":"label2"}}}}}'

echo -e "\n Wait for nginx-blue RS to be active..."
until [[ "$(kubectl get rs nginx-blue -o 'jsonpath={.status.readyReplicas}')" -eq 3 ]]; do sleep 1; done
until [[ "$(kubectl get rs nginx-green -o 'jsonpath={.status.replicas}')" -eq 0 ]]; do sleep 1; done
until [[ "$(kubectl get $bgd nginx -o 'jsonpath={.status.activeColor}')" -eq "blue" ]]; do sleep 1; done
until [[ "$(kubectl get $bgd nginx -o 'jsonpath={.status.active.availableReplicas}')" -eq 3 ]]; do sleep 1; done
until [[ "$(kubectl get $bgd nginx -o 'jsonpath={.status.observedGeneration}')" -eq "$(kubectl get $bgd nginx -o 'jsonpath={.metadata.generation}')" ]]; do sleep 1; done