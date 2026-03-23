FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd

RUN go build -o /out/blink-ledger ./cmd/ledger-sim

FROM alpine:3.21

RUN adduser -D -u 10001 appuser

WORKDIR /app

COPY --from=build /out/blink-ledger /usr/local/bin/blink-ledger

USER appuser

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/blink-ledger"]
