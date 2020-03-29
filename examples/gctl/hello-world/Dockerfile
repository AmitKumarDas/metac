# --------------------------
# Build d-operators binary
# --------------------------
FROM golang:1.13.5 as builder

WORKDIR /example.io/hello-world/

# copy go modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

COPY Makefile Makefile

COPY vendor/ vendor/

# ensure vendoring is up-to-date by running make vendor 
# in your local setup
#
# we cache the vendored dependencies before building and
# copying source so that we don't need to re-download when
# source changes don't invalidate our downloaded layer
RUN go mod download
RUN go mod tidy
RUN go mod vendor

# copy source file(s)
COPY cmd/ cmd/

# build the binary
RUN make

# ---------------------------
# Use distroless as minimal base image to package the final binary
# Refer https://github.com/GoogleContainerTools/distroless
# ---------------------------
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /example.io/hello-world/hello-world .
COPY config/config.yaml /etc/config/metac/

USER nonroot:nonroot

CMD ["/hello-world"]
