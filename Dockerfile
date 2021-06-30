FROM golang:1.16-alpine AS builder

WORKDIR /app

COPY . .

RUN go build -o babylon

FROM alpine:3.14

WORKDIR /app

COPY --from=builder /app/babylon /app/babylon

CMD ["/app/babylon"]