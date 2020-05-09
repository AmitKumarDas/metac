#!/bin/bash

set -e
set -u

echo ""
echo "+++ Will install binaries required to run integration test(s)"
echo ""

# This script downloads kubectl, kube-apiserver & etcd binaries
# that are used as part of the integration test environment,
# and places them in the following path:
#
#   test/integration/framework/assets/bin
#
# Integration test framework expects these binaries to be 
# found in the PATH.

# This is the kube-apiserver version to test against.
KUBE_VERSION="${KUBE_VERSION:-v1.16.4}"
KUBERNETES_RELEASE_URL="${KUBERNETES_RELEASE_URL:-https://dl.k8s.io}"

# This should be the etcd version downloaded by 
# kubernetes/hack/lib/etcd.sh as of the above Kubernetes version.
ETCD_VERSION="${ETCD_VERSION:-v3.4.3}"

# This is the folder where binaries will be downloaded
cd test/integration/framework/assets/bin

# Download kubectl if not found
if [[ -f ./kubectl ]] && ./kubectl version --client; then
    echo ""
    echo "+++ Above kubectl was installed previously"
    echo ""
else
    wget -nv "${KUBERNETES_RELEASE_URL}/${KUBE_VERSION}/bin/linux/amd64/kubectl"
    chmod +x kubectl
fi


# Download kube-apiserver if not found
if [[ -f ./kube-apiserver ]] && ./kube-apiserver --version; then
    echo ""
    echo "+++ Above kube-apiserver was installed previously"
    echo ""
else
    wget -nv "${KUBERNETES_RELEASE_URL}/${KUBE_VERSION}/bin/linux/amd64/kube-apiserver"
    chmod +x kube-apiserver
fi

# Download etcd if not found
if [[ -f ./etcd ]] && ./etcd --version; then
    echo ""
    echo "+++ Above etcd was installed previously"
    echo ""
else
    basename="etcd-${ETCD_VERSION}-linux-amd64"
    filename="${basename}.tar.gz"
    url="https://github.com/coreos/etcd/releases/download/${ETCD_VERSION}/${filename}"
    wget -nv "${url}"
    tar -zxf "${filename}"
    mv "${basename}/etcd" etcd
    rm -rf "${basename}" "${filename}"
fi
