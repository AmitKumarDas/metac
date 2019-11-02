REGISTRY ?= quay.io/amitkumardas
IMG_NAME ?= set-status-on-cr
PACKAGE_VERSION ?= latest

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