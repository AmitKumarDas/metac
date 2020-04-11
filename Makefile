PWD := ${CURDIR}
PATH := $(PWD)/hack/bin:$(PATH)

PACKAGE_NAME := openebs.io/metac
API_GROUPS := metacontroller/v1alpha1

PACKAGE_VERSION ?= $(shell git describe --always --tags)
OS = $(shell uname)

PKGS = $(shell go list ./... | grep -v '/test/integration/\|/examples/')
ALL_SRC = $(shell find . -name "*.go" | grep -v -e "vendor")

BUILD_LDFLAGS = -X $(PACKAGE_NAME)/build.Hash=$(PACKAGE_VERSION)
GO_FLAGS = -gcflags '-N -l' -ldflags "$(BUILD_LDFLAGS)"
GOPATH := $(firstword $(subst :, ,$(GOPATH)))

REGISTRY ?= quay.io/amitkumardas
IMG_NAME ?= metac

ifeq (, $(shell which deepcopy-gen))
  $(shell GOFLAGS="" go get k8s.io/code-generator/cmd/deepcopy-gen)
  DEEPCOPY_GEN=$(GOBIN)/deepcopy-gen
else
  DEEPCOPY_GEN=$(shell which deepcopy-gen)
endif

export GO111MODULE=on

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"
CONTROLLER_GEN := go run ./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go

export GO111MODULE=on

all: manifests bins

bins: generated_files $(IMG_NAME)

$(IMG_NAME): $(ALL_SRC)
	@echo "+ Generating $(IMG_NAME) binary"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  go build -mod=vendor -tags bins $(GO_FLAGS) -o $@ ./main.go

$(ALL_SRC): ;

# Code generators
# https://github.com/kubernetes/community/blob/master/contributors/devel/api_changes.md#generate-code
.PHONY: generated_files
generated_files: vendor
	@./hack/update-codegen.sh

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: generated_files
	@echo "+ Generating $(IMG_NAME) crds"
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role paths="./apis/..." output:crd:artifacts:config=manifests/crds
	@cat manifests/crds/*.yaml > manifests/metacontroller.yaml
	@cp manifests/crds/*.yaml helm/metac/crds/
	@sed -i'' -e 's@annotations:@annotations:\n    "helm.sh/hook": crd-install@g' helm/metac/crds/*
	@rm -rf manifests/crds

# go mod download modules to local cache
# make vendored copy of dependencies
# install other go binaries for code generation
.PHONY: vendor
vendor: go.mod go.sum
	@go mod download
	@go mod tidy
	@go mod vendor

.PHONY: image
image:
	docker build -t $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION) .
	docker build -t $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION)_debug -f Dockerfile.debug .

.PHONY: push
push: image
	docker push $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION)
	docker push $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION)_debug

.PHONY: unit-test
unit-test: generated_files
	@go test -mod=vendor -i ${PKGS}
	@go test -mod=vendor ${PKGS}

.PHONY: integration-dependencies
integration-dependencies: manifests
	@./hack/get-kube-binaries.sh

# Integration test makes use of kube-apiserver, etcd & kubectl
# binaries. This does not require metac binary or docker image.
# This can be run on one's laptop or Travis like CI environments.
.PHONY: integration-test
integration-test: integration-dependencies
	@go test -mod=vendor ./test/integration/... -v -short -timeout 5m \
	-args --logtostderr -v=1

.PHONY: integration-test-gctl
integration-test-gctl: integration-dependencies
	@go test -mod=vendor ./test/integration/generic/... -v -timeout 5m \
	-args --logtostderr -v=1

.PHONY: integration-test-local-gctl
integration-test-local-gctl: integration-dependencies
	@go test -mod=vendor ./test/integration/genericlocal/... -v -timeout 5m \
	-args --logtostderr -v=1
