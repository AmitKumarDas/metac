ALL_SRC = $(shell find . -name "*.go" | grep -v -e "vendor")

IMG_NAME ?= dynamicconfig
PACKAGE_VERSION ?= latest

all: bins

bins: vendor $(IMG_NAME)

$(IMG_NAME): $(ALL_SRC)
	@echo "+ Generating $(IMG_NAME) binary"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on \
		go build -o $@ ./cmd/main.go

$(ALL_SRC): ;

# go mod download modules to local cache
# make vendored copy of dependencies
# install other go binaries for code generation
.PHONY: vendor
vendor: go.mod go.sum
	@GO111MODULE=on go mod download
	@GO111MODULE=on go mod tidy
	@GO111MODULE=on go mod vendor

.PHONY: image
image:
	docker build -t $(IMG_NAME):$(PACKAGE_VERSION) .