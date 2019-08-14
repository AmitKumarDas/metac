PWD := ${CURDIR}

PACKAGE_NAME := openebs.io/metac
API_GROUPS := metacontroller/v1alpha1

PACKAGE_VERSION ?= $(shell git describe --always --tags)
OS = $(shell uname)

ALL_SRC = $(shell find . -name "*.go" | grep -v -e "vendor")
ALL_PKGS = $(shell go list $(sort $(dir $(ALL_SRC))))
ALL_UT_PKGS = $($(ALL_PKGS) | grep -v -e "test/integration" \
	-e "hack" \
	-e "examples" \
	-e "client/generated")

BUILD_LDFLAGS = -X $(PACKAGE_NAME)/build.Hash=$(PACKAGE_VERSION)
GO_FLAGS = -gcflags '-N -l' -ldflags "$(BUILD_LDFLAGS)"

REGISTRY ?= quay.io/amitkumardas
IMG_NAME ?= metac

all: vendor bins

bins: generated_files $(IMG_NAME)

$(IMG_NAME): $(ALL_SRC)
	@echo "+ Generating $(IMG_NAME) binary"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on \
		go build -tags bins $(GO_FLAGS) -o $@ ./main.go

$(ALL_SRC): ;

unit-test:
	@GO111MODULE=on go test $(ALL_UT_PKGS) -mod=vendor

gofmt:
	@GO111MODULE=on go fmt $(ALL_PKGS)

# go mod download modules to local cache
# make vendored copy of dependencies
# install other go binaries for code generation
.PHONY: vendor
vendor: go.mod go.sum
	@GO111MODULE=on go mod download
	@GO111MODULE=on go mod vendor

integration-test:
	go test -i ./test/integration/...
	PATH="$(PWD)/hack/bin:$(PATH)" go test ./test/integration/... -v -timeout 5m -args -v=6

image:
	docker build -t $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION) .

push: image
	docker push $(REGISTRY)/$(IMG_NAME):$(PACKAGE_VERSION)

# Code generators
# https://github.com/kubernetes/community/blob/master/contributors/devel/api_changes.md#generate-code

generated_files:
	@./hack/update-codegen.sh
