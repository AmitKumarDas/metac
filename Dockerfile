# Tester image
FROM golang:1.13.5 as tester

WORKDIR /go/src/openebs.io/metac/

# copy go modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# copy build manifests
COPY Makefile Makefile

# ensure vendoring is up-to-date by running make vendor in your local
# setup
#
# we cache the vendored dependencies before building and copying source
# so that we don't need to re-download when source changes don't invalidate
# our downloaded layer
RUN make vendor

# copy build manifests
COPY . .

RUN make unit-test
RUN make integration-test

# Build metac binary
FROM golang:1.13.5 as builder

WORKDIR /go/src/openebs.io/metac/

# copy go modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# copy build manifests
COPY Makefile Makefile

# ensure vendoring is up-to-date by running make vendor in your local
# setup
#
# we cache the vendored dependencies before building and copying source
# so that we don't need to re-download when source changes don't invalidate
# our downloaded layer
RUN make vendor

# copy source files
COPY *.go ./
COPY apis/ apis/
COPY client/ client/
COPY config/ config/
COPY hack/ hack/
COPY controller/ controller/
COPY dynamic/ dynamic/
COPY hooks/ hooks/
COPY server/ server/
COPY start/ start/
COPY third_party/ third_party/

# build metacontroller binary
RUN make bins

# Use a distroless image
# Sort the instructions to be more effective for caching 

FROM gcr.io/distroless/base-debian10:nonroot

WORKDIR /
USER nonroot:nonroot
CMD ["/usr/bin/metac"]
COPY --from=builder --chown=nonroot /go/src/openebs.io/metac/metac /usr/bin/
