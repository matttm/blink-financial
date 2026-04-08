# Capability Expansion Guide

This document collects contributor-facing workflows for expanding the project beyond the default local run path.

## Generated gRPC Code

The files under [internal/gen/blink/transactions/v1/transactions.pb.go](/Users/Matt.Maloney/projects/play/blink-financial/internal/gen/blink/transactions/v1/transactions.pb.go) and [transactions_grpc.pb.go](/Users/Matt.Maloney/projects/play/blink-financial/internal/gen/blink/transactions/v1/transactions_grpc.pb.go) are generated from [transactions.proto](/Users/Matt.Maloney/projects/play/blink-financial/proto/blink/transactions/v1/transactions.proto).

Use this command from the repository root to regenerate them after editing the proto file:

```bash
PATH=$(pwd)/bin:$PATH protoc \
  --go_out=. \
  --go_opt=module=github.com/matttm/blink-financial \
  --go-grpc_out=. \
  --go-grpc_opt=module=github.com/matttm/blink-financial \
  proto/blink/transactions/v1/transactions.proto
```

Required local tools:

- `protoc`
- `protoc-gen-go`
- `protoc-gen-go-grpc`

The repository ignores the local [bin](/Users/Matt.Maloney/projects/play/blink-financial/bin) directory, so it is safe to install those generators there for local use.

## Current Transport Split

The project currently uses two different ingress patterns:

- HTTP `POST /api/v1/transactions` for validated ingest
- gRPC `blink.transactions.v1.TransactionEventsService/StreamTransactions` for server-side event streaming to tools like a TUI

That split is intentional:

- ingest remains simple and easy to benchmark through HAProxy
- the gRPC stream stays off the primary write path
- a Rust or Go terminal client can subscribe to accepted transaction events without affecting HTTP ingestion design

## When To Regenerate Code

You should regenerate the protobuf output when you change:

- service names
- RPC names
- message fields
- message types
- `go_package`

After regeneration, run:

```bash
gofmt -w internal/gen/blink/transactions/v1/transactions.pb.go internal/gen/blink/transactions/v1/transactions_grpc.pb.go
GOCACHE=$(pwd)/.gocache go build ./cmd/ledger-sim
```

## Expansion Ideas

Natural next areas to expand are:

- richer stream filters for the TUI
- replay or history APIs alongside the live stream
- WAL-backed event emission instead of Redis pub/sub
- a Rust TUI subscriber using the gRPC stream
- additional protobuf services for operator tooling rather than primary ingest
