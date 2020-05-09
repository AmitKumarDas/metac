PWD := ${CURDIR}
PATH := $(PWD)/hack/bin:$(PATH)
ASSETS_PATH := $(PWD)/test/integration/framework/assets/bin

PACKAGE_NAME := openebs.io/metac
API_GROUPS := metacontroller/v1alpha1

GIT_TAGS = $(shell git fetch --all --tags)
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

all: manifests bins

$(ALL_SRC): ;

$(GIT_TAGS): ;

bins: generated_files $(IMG_NAME)

$(IMG_NAME): $(ALL_SRC)
	@rm -f $(IMG_NAME)
	@echo "+ Generating $(IMG_NAME) binary"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  go build -mod=vendor -tags bins $(GO_FLAGS) -o $@ ./main.go

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: generated_files
	@echo "+ Generating $(IMG_NAME) crds"
	@$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role paths="./apis/..." output:crd:artifacts:config=manifests/crds
	@cat manifests/crds/*.yaml > manifests/metacontroller.yaml
	@cp manifests/crds/*.yaml helm/metac/crds/
	@sed -i'' -e 's@annotations:@annotations:\n    "helm.sh/hook": crd-install@g' helm/metac/crds/*
	@rm -rf manifests/crds

# Code generators
# https://github.com/kubernetes/community/blob/master/contributors/devel/api_changes.md#generate-code
.PHONY: generated_files
generated_files: vendor
	@./hack/update-codegen.sh

# go mod download modules to local cache
# make vendored copy of dependencies
# install other go binaries for code generation
.PHONY: vendor
vendor: go.mod go.sum
	@go mod download
	@go mod tidy
	@go mod vendor

.PHONY: image
image: $(GIT_TAGS)
	docker build -t $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION) .
	docker build -t $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION)_debug -f Dockerfile.debug .

.PHONY: push
push: image
	docker push $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION)
	docker push $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION)_debug

.PHONY: unit-test
unit-test: generated_files
	@go test -cover -i ${PKGS}
	@go test -cover ${PKGS}

# integration-dependencies ensures generation of manifests as 
# well as metac binary.
#
# NOTE:
# 	One can use metac binary in integration tests with webhooks
# only. The tests that need to use inline hooks need to use the
# source code itself.
.PHONY: integration-dependencies
integration-dependencies: all
	@rm -f $(ASSETS_PATH)/$(IMG_NAME)
	@mv $(IMG_NAME) $(ASSETS_PATH)/$(IMG_NAME)
	@./hack/get-kube-binaries.sh

# Integration test makes use of kube-apiserver, etcd & kubectl
# binaries. One may optionally make use of metac binary. Use of
# metac docker image is not required. This can be run on one's 
# laptop or via docker build. This ensures integration tests
# can be run on any CI environments like Travis, etc without the
# need to have a full blown kubernetes setup.
.PHONY: integration-test
integration-test: integration-dependencies
	@go test ./test/integration/... \
		-v -timeout=10m -args --logtostderr --alsologtostderr -v=1

# integration-test-crd-mode runs tests with metac loading 
# metacontrollers as kubernetes custom resources
.PHONY: integration-test-crd-mode
integration-test-crd-mode: integration-dependencies
	@go test ./test/integration/crd-mode/... \
		-v -timeout=10m -args --logtostderr --alsologtostderr -v=1

# integration-test-local-gctl runs generic controller tests when 
# metac makes use of metacontrollers as config files
.PHONY: integration-test-config-mode
integration-test-config-mode: integration-dependencies
	@go test ./test/integration/config-mode/... \
		-v -timeout=10m -args --logtostderr --alsologtostderr -v=1