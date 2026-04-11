# Transaction Monitor TUI

This is a standalone Rust terminal UI for the Blink Financial gRPC event stream.

It connects to:

- `blink.transactions.v1.TransactionEventsService/StreamTransactions`

and displays:

- recent streamed batches
- per-batch summary data
- transactions inside the selected batch

## Requirements

- Rust and Cargo
- the Blink stack running with the direct gRPC endpoint on `localhost:9091`
- `protoc` available locally, because the crate generates Rust bindings from the shared protobuf contract at build time

## Run

From the repository root:

```bash
cargo run --manifest-path tui/transaction-monitor/Cargo.toml
```

## Filters

The TUI reads optional stream filters from environment variables:

- `BLINK_GRPC_ENDPOINT`
- `BLINK_STREAM_SOURCE`
- `BLINK_STREAM_TENANT_ID`
- `BLINK_STREAM_ACCOUNT_ID`
- `BLINK_STREAM_BATCH_ID`

Example:

```bash
BLINK_STREAM_SOURCE=checkout \
BLINK_STREAM_TENANT_ID=tenant-123 \
cargo run --manifest-path tui/transaction-monitor/Cargo.toml
```

Default endpoint:

```text
http://127.0.0.1:9091
```

## Keys

- `q` quits
- `j` / `Down` moves to the next batch
- `k` / `Up` moves to the previous batch
