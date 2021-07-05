FROM golang:1.16-alpine AS builder

WORKDIR /app

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
# Copy the go source
COPY main.go main.go

# TODO: Enable when code is in pkg/
# COPY pkg/ pkg/

RUN go build -o babylon

FROM alpine:3.14

WORKDIR /app

COPY --from=builder /app/babylon /app/babylon

CMD ["/app/babylon"]
