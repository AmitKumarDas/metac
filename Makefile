PWD := ${CURDIR}

PACKAGE_NAME := openebs.io/metac
API_GROUPS := metacontroller/v1alpha1

PACKAGE_VERSION ?= $(shell git describe --always --tags)
OS = $(shell uname)

ALL_SRC = $(shell find . -name "*.go" | grep -v -e "vendor")

BUILD_LDFLAGS = -X $(PACKAGE_NAME)/build.Hash=$(PACKAGE_VERSION)
GO_FLAGS = -gcflags '-N -l' -ldflags "$(BUILD_LDFLAGS)"

REGISTRY ?= quay.io/amitkumardas
IMG_NAME ?= metac

all: bins

bins: generated_files $(IMG_NAME)

$(IMG_NAME): $(ALL_SRC)
	@echo "+ Generating $(IMG_NAME) binary"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on \
		go build -tags bins $(GO_FLAGS) -o $@ ./main.go

$(ALL_SRC): ;

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
	@GO111MODULE=on go mod download
	@GO111MODULE=on go mod vendor

.PHONY: image
image:
	docker build -t $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION) .

.PHONY: push
push: image
	docker push $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION)

.PHONY: unit-test
unit-test: generated_files
	@pkgs="$$(go list ./... | grep -v '/test/integration/\|/examples/')" ; \
		go test -i $${pkgs} && \
		go test $${pkgs}

.PHONY: integration-dependencies
integration-dependencies:
	@./hack/get-kube-binaries.sh

.PHONY: integration-test
integration-test: generated_files integration-dependencies
	@PATH="$(PWD)/hack/bin:$(PATH)" go test ./test/integration/... -v -timeout 5m -args -v=6