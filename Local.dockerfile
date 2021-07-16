FROM alpine:3

WORKDIR /app
COPY babylon .

CMD ["./babylon"]