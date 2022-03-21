FROM docker.io/curlimages/curl:latest as linkerd
ARG LINKERD_AWAIT_VERSION=v0.2.3
RUN curl -sSLo /tmp/linkerd-await https://github.com/linkerd/linkerd-await/releases/download/release%2F${LINKERD_AWAIT_VERSION}/linkerd-await-${LINKERD_AWAIT_VERSION}-amd64 && \
    chmod 755 /tmp/linkerd-await

FROM golang:1.18.0-alpine AS builder

WORKDIR /app

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
# Copy the go source
COPY main.go main.go

COPY pkg/ pkg/

RUN go build -o babylon

FROM alpine:3.14

WORKDIR /app
COPY --from=linkerd /tmp/linkerd-await /linkerd-await
COPY --from=builder /app/babylon /app/babylon

ENTRYPOINT ["/linkerd-await", "--"]
CMD ["/app/babylon"]
