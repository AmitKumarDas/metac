FROM golang:1.12.7 AS builder

WORKDIR /

# copy go modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# ensure vendoring is up-to-date by running make vendor in your local
# setup
#
# we cache the vendored dependencies before building and copying source
# so that we don't need to re-download when source changes don't invalidate
# our downloaded layer
#RUN GO111MODULE=on go mod tidy
RUN GO111MODULE=on go mod download
RUN GO111MODULE=on go mod vendor

# copy source files
COPY *.go ./

RUN go build -o /go/bin/set-status-on-cr

FROM debian:stretch-slim

RUN apt-get update && \
  apt-get install --no-install-recommends -y ca-certificates && \
  rm -rf /var/lib/apt/lists/*

COPY config.yaml /etc/config/metac/
COPY --from=builder /go/bin/set-status-on-cr /usr/bin/set-status-on-cr
