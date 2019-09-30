#!/bin/bash

cleanup() {
  set +e
  
  echo ""

  echo "--------------------------"
  echo "++ Clean up started"
  echo "--------------------------"

  kubectl patch statefulset nginx --type=merge -p '{"metadata":{"finalizers":[]}}' || true
  kubectl delete -f my-statefulset.yaml || true
  kubectl delete -f service-per-pod.yaml || true
  kubectl delete svc -l app=service-per-pod || true
  kubectl delete configmap service-per-pod-hooks -n metac || true
  
  echo "--------------------------"
  echo "++ Clean up completed"
  echo "--------------------------"
}
trap cleanup EXIT

#set -ex

finalizer="protect.dctl.metac.openebs.io/service-per-pod-test"

echo -e "\n++ Installing meta controllers & webhook service"
kubectl create configmap service-per-pod-hooks -n metac --from-file=hooks
kubectl apply -f service-per-pod.yaml

echo -e "\n++ Applying STS that will get watched by metac"
kubectl apply -f my-statefulset.yaml

echo -e "\n++ Waiting for per-pod Service..."
until [[ "$(kubectl get svc nginx-2 -o 'jsonpath={.spec.selector.pod-name}')" == "nginx-2" ]]; \
  do echo "++ Will retry" && sleep 1; \
done

echo -e "\n++ Waiting for pod-name label..."
until [[ "$(kubectl get pod nginx-2 -o 'jsonpath={.metadata.labels.pod-name}')" == "nginx-2" ]]; do sleep 1; done

echo -e "\n++ Removing annotation to opt out of service-per-pod without deleting the STS"
kubectl annotate statefulset nginx service-per-pod-label-

echo -e "\n++ Waiting for per-pod Service to get cleaned up by the decorator's finalizer"
until [[ "$(kubectl get svc nginx-2 2>&1)" == *NotFound* ]]; do sleep 1; done

echo -e "\n++ Waiting for the decorator's finalizer to be removed"
while [[ "$(kubectl get statefulset nginx -o 'jsonpath={.metadata.finalizers}')" == *decoratorcontroller-service-per-pod* ]]; do sleep 1; done

echo -e "\n++ Adding the annotation back to opt in again"
kubectl annotate statefulset nginx service-per-pod-label=pod-name

echo -e "\n++ Wait for per-pod Service to come back"
until [[ "$(kubectl get svc nginx-2 -o 'jsonpath={.spec.selector.pod-name}')" == "nginx-2" ]]; do sleep 1; done

echo -e "\n++ Appending our own finalizer so we can check deletion ordering"
kubectl patch statefulset nginx --type=json -p '[{"op":"add","path":"/metadata/finalizers/-","value":"'${finalizer}'"}]'

echo -e "\n++ Deleting the StatefulSet"
kubectl delete statefulset nginx --wait=false

echo -e "\n++ Waiting for per-pod Service to get cleaned up by the decorator's finalizer"
until [[ "$(kubectl get svc nginx-2 2>&1)" == *NotFound* ]]; do sleep 1; done

echo -e "\n++ Waiting for the decorator's finalizer to be removed..."
while [[ "$(kubectl get statefulset nginx -o 'jsonpath={.metadata.finalizers}')" == *decoratorcontroller-service-per-pod* ]]; do sleep 1; done
