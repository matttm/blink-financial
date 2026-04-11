# Blink Financial

Blink Financial is a local-first Go service for experimenting with high-throughput transaction ingestion. The repository is intentionally small: an HTTP service accepts transaction batches, forwards them into Redis, and runs behind HAProxy so you can scale app replicas with Docker Compose and pressure-test the shape of the system.

This is not a full ledger yet. It is a simulation scaffold for testing local infrastructure, batch ingestion, queue-backed writes, and load-generation workflows before moving toward an append-only WAL design.

## Project Status

This repository is under early development.

Expect:

- breaking changes
- incomplete features
- rough edges in the local stack
- documentation and interfaces to evolve as the design solidifies

## What This Repository Contains

- A Go HTTP service under `cmd/ledger-sim`
- Typed startup configuration under `internal/config`
- A local Docker Compose stack with:
  - `haproxy` as the entry point
  - scalable `app` containers
  - `redis` as the sink
  - `prometheus` for metrics collection
  - `grafana` for visualization
- supporting docs for architecture, throughput checklist, smoke testing, and soak testing

## Architecture

At a high level:

```text
Client -> HAProxy -> [Go app replicas] -> Redis -> RAM disk-backed data path
```

More detail is in [architecture.md](/Users/Matt.Maloney/projects/play/blink-financial/architecture.md).

## Repository Layout

```text
.
├── README.md
├── compose.yaml
├── Dockerfile
├── .env.example
├── checklist.md
├── architecture.md
├── k6/
│   ├── light-load.js
│   └── soak.js
├── smoke-test-case.md
├── soak-test-case.md
├── cmd/
│   └── ledger-sim/
│       └── main.go
├── internal/
│   └── config/
│       └── config.go
├── docker/
│   ├── grafana/
│   │   ├── dashboards/
│   │   │   └── blink-overview.json
│   │   └── provisioning/
│   │       └── dashboards/
│   │           └── dashboards.yml
│   │       └── datasources/
│   │           └── prometheus.yml
│   ├── haproxy/
│   │   └── haproxy.cfg
│   ├── prometheus/
│   │   └── prometheus.yml
│   └── redis/
│       └── redis.conf
└── scripts/
    ├── cleanup_ramdisk.sh
    ├── install_k6.sh
    └── setup_ramdisk.sh
```

## Current Request Flow

1. A client sends an HTTP request to HAProxy on port `8080`.
2. HAProxy forwards the request to one of the running app replicas.
3. The Go service accepts the batch at `POST /api/v1/transactions`.
4. The app pushes the request payload into Redis using a pooled `redis/go-redis` client.
5. Redis persists its append-only data under the host path configured by `BLINK_RAMDISK_PATH`.

There is also a direct gRPC event stream in the current stack:

1. A client connects directly to the `grpc` service on port `9091`.
2. The Go service streams accepted transaction events through `blink.transactions.v1.TransactionEventsService/StreamTransactions`.
3. The event stream is intended for operator tools like a Rust TUI, not for primary ingest.

The Compose file keeps `app` and `grpc` as separate services even though they run the same binary. That split is intentional:

- `app` is the scalable HTTP ingest tier behind HAProxy
- `grpc` gives you one stable direct host port for the TUI event stream
- publishing `9091` directly on the scaled `app` service would not work cleanly once multiple replicas are running, because only one container can bind a fixed host port

So the duplication is operational, not architectural: there is still one application binary, but Compose uses two service entries to satisfy two different networking needs.

## API

The current service is mounted under the `/api/v1` prefix.

Endpoints:

- `GET /api/v1/healthz`
- `GET /api/v1/readyz`
- `POST /api/v1/transactions`
- `GET /metrics`
- gRPC: `blink.transactions.v1.TransactionEventsService/StreamTransactions` on `localhost:9091`

Behavior:

- `healthz` returns `200 OK` if the process is up.
- `readyz` returns `200 OK` if Redis is reachable.
- `transactions` accepts a JSON transaction batch, validates it, and returns `202 Accepted` after pushing the normalized event into Redis.
- `metrics` exposes Prometheus-formatted application and Go runtime metrics.
- `StreamTransactions` streams accepted transaction events to subscribers and supports optional filters like `source`, `tenant_id`, `account_id`, and `batch_id`.

Example:

```bash
curl -i http://localhost:8080/api/v1/healthz

curl -i http://localhost:8080/api/v1/readyz

curl -i \
  -X POST http://localhost:8080/api/v1/transactions \
  -H 'Content-Type: application/json' \
  -d '{
    "batch_id": "batch-001",
    "source": "checkout",
    "transactions": [
      {
        "idempotency_key": "idem-001",
        "tenant_id": "tenant-123",
        "account_id": "acct-456",
        "type": "debit",
        "amount": {
          "currency": "USD",
          "value": "12.50"
        },
        "reference": "invoice-789",
        "occurred_at": "2026-04-07T14:30:00Z",
        "metadata": {
          "source_channel": "web"
        }
      }
    ]
  }'
```

If you want to test the gRPC endpoint directly, `grpcurl` works well because server reflection is enabled:

```bash
grpcurl -plaintext localhost:9091 list

grpcurl -plaintext \
  -d '{"source":"checkout","tenantId":"tenant-123"}' \
  localhost:9091 \
  blink.transactions.v1.TransactionEventsService/StreamTransactions
```

## Configuration

The service reads its configuration from environment variables at startup.

App variables:

- `PORT`
- `GRPC_PORT`
- `REDIS_ADDR`
- `REDIS_LIST_KEY`
- `HOSTNAME`

Compose variables:

- `BLINK_HTTP_PORT`
- `BLINK_GRPC_PORT`
- `BLINK_RAMDISK_PATH`
- `BLINK_REDIS_LIST_KEY`
- `PROMETHEUS_PORT`
- `GRAFANA_PORT`
- `GRAFANA_ADMIN_USER`
- `GRAFANA_ADMIN_PASSWORD`

Defaults are documented in [.env.example](/Users/Matt.Maloney/projects/play/blink-financial/.env.example).

## Prerequisites

For local development you should have:

- Go `1.24+`
- Docker with Compose support
- a RAM disk or other fast local path for Redis persistence
- optionally `k6` if you want to run soak tests
- optionally `grpcurl` if you want to test the gRPC endpoint directly

For the new gRPC and protobuf workflow, these tools are also useful:

- `protoc`
- `protoc-gen-go`
- `protoc-gen-go-grpc`

The generated gRPC stubs are checked into the repository already. You only need the protobuf toolchain locally if you plan to modify [transactions.proto](/Users/Matt.Maloney/projects/play/blink-financial/proto/blink/transactions/v1/transactions.proto) and regenerate code.

The regeneration command and broader contributor workflow for expanding transports and generated code live in [capability-expansion-guide.md](/Users/Matt.Maloney/projects/play/blink-financial/capability-expansion-guide.md).

If you want quick helpers for local setup, use:

- [setup_ramdisk.sh](/Users/Matt.Maloney/projects/play/blink-financial/scripts/setup_ramdisk.sh) to create and mount the RAM disk
- [cleanup_ramdisk.sh](/Users/Matt.Maloney/projects/play/blink-financial/scripts/cleanup_ramdisk.sh) to unmount it later
- [install_k6.sh](/Users/Matt.Maloney/projects/play/blink-financial/scripts/install_k6.sh) to install `k6`

A RAM disk is different from a normal directory. A normal directory is just a folder on your SSD or disk. A RAM disk is a separate filesystem backed by memory and mounted at a directory path. The path looks like a normal folder, but reads and writes go to RAM instead of disk. That is why it is useful here: Redis can write to a very fast memory-backed path during local throughput tests.

## Quick Start

1. Copy the example env file.

```bash
cp .env.example .env
```

2. Create the RAM disk and make sure the Redis target path exists.

If you use the helper script, it creates and mounts a RAM-backed filesystem first. On macOS it does that with `hdiutil` and `diskutil`, then waits for the OS to register the new device before formatting and mounting it. That is different from just running `mkdir`, which would only create a normal folder on disk.

Example:

```bash
./scripts/setup_ramdisk.sh 1024 /Volumes/blink-ramdisk
```

If you want to know the cleanup path in advance, the matching teardown command is:

```bash
./scripts/cleanup_ramdisk.sh /Volumes/blink-ramdisk --remove-dir
```

Then create the Redis data directory inside that mounted filesystem:

```bash
mkdir -p /Volumes/blink-ramdisk/redis-data
```

This two-step process matters. If `/Volumes/blink-ramdisk` is only a normal folder, Docker will still bind mount it, but Redis will write to your regular disk instead of RAM.

If you are using Colima, there is one more important detail: the RAM disk path must also be shared into the Colima VM. Otherwise Redis may write to a VM-local path that looks the same, while your host RAM disk stays empty. See [colima-ramdisk-note.md](/Users/Matt.Maloney/projects/play/blink-financial/colima-ramdisk-note.md) before continuing.

If Redis later fails to start with a `chown: .: Permission denied` message, update the host bind-mounted directory ownership for the Redis container user before retrying:

```bash
sudo chown -R 999:1000 /Volumes/blink-ramdisk/redis-data
chmod -R u+rwX /Volumes/blink-ramdisk/redis-data
```

This repository also runs Redis as user `999:1000` in Compose so the container does not try to `chown` the shared mount at startup.

3. Start the stack.

```bash
docker compose up --build --scale app=3 -d
```

If you change container configuration, provisioning files, mounted config files, or other startup-time assets and the change does not appear, recreate the affected containers explicitly:

```bash
docker compose up --build -d --force-recreate
```

That is especially common after changes to Grafana dashboards, Prometheus config, Compose settings, or container user and volume behavior.

4. Check the health endpoints.

```bash
curl http://localhost:8080/api/v1/healthz
curl http://localhost:8080/api/v1/readyz
```

If you want to verify the direct gRPC listener too:

```bash
grpcurl -plaintext localhost:9091 list
```

5. Send a sample batch.

```bash
curl -X POST http://localhost:8080/api/v1/transactions \
  -H 'Content-Type: application/json' \
  -d '{
    "batch_id": "batch-001",
    "source": "checkout",
    "transactions": [
      {
        "idempotency_key": "idem-001",
        "tenant_id": "tenant-123",
        "account_id": "acct-456",
        "type": "debit",
        "amount": {
          "currency": "USD",
          "value": "12.50"
        },
        "reference": "invoice-789",
        "occurred_at": "2026-04-07T14:30:00Z",
        "metadata": {
          "source_channel": "web"
        }
      }
    ]
  }'
```

6. Inspect queue depth in Redis.

```bash
docker compose exec redis redis-cli LLEN blink:transactions
```

7. Open the observability tools.

```text
Prometheus: http://localhost:9090
Grafana:    http://localhost:3000
```

## Running The Service Without Docker

You can also run the service directly if Redis is already available:

```bash
PORT=8080 \
REDIS_ADDR=localhost:6379 \
REDIS_LIST_KEY=blink:transactions \
HOSTNAME=local-dev \
go run ./cmd/ledger-sim
```

Then call:

```bash
curl http://localhost:8080/api/v1/healthz
```

## Scaling The App Replicas

Compose uses:

- a scalable `app` service for HTTP ingest behind HAProxy
- a separate `grpc` service for one stable direct gRPC stream on `localhost:9091`

To increase HTTP ingest concurrency, scale the `app` service at runtime:

```bash
docker compose up --build --scale app=10 -d
```

HAProxy is configured to distribute requests across the `app` containers discovered on the Compose network. The `grpc` service is not part of that load-balanced path; it stays single-instance so the TUI has one predictable streaming endpoint.

## Observability

Prometheus scrapes the HTTP app replicas and the direct `grpc` service at `/metrics`, and Grafana is pre-provisioned with both:

- a Prometheus datasource
- a default dashboard named `Blink Financial Overview`

The default dashboard includes panels for:

- batch throughput
- transaction throughput
- transaction latency
- batch bytes throughput
- Go routines and memory
- Redis pool connections and Redis pool activity

Useful URLs:

- application metrics through the ingress path: `http://localhost:8080/metrics`
- Prometheus UI: `http://localhost:9090`
- Grafana UI: `http://localhost:3000`

Default Grafana credentials come from `.env` and default to:

- username: `admin`
- password: `admin`

The current app exports:

- Go runtime metrics from the Prometheus Go client
- `blink_ledger_transaction_batches_total`
- `blink_ledger_transactions_total`
- `blink_ledger_batch_bytes_total`
- `blink_ledger_request_duration_seconds`
- `blink_redis_pool_hits_total`
- `blink_redis_pool_misses_total`
- `blink_redis_pool_timeouts_total`
- `blink_redis_pool_wait_count_total`
- `blink_redis_pool_wait_duration_seconds_total`
- `blink_redis_pool_unusable_total`
- `blink_redis_pool_total_conns`
- `blink_redis_pool_idle_conns`
- `blink_redis_pool_stale_conns`

Provisioned Grafana files live under:

- [dashboards.yml](/Users/Matt.Maloney/projects/play/blink-financial/docker/grafana/provisioning/dashboards/dashboards.yml)
- [blink-overview.json](/Users/Matt.Maloney/projects/play/blink-financial/docker/grafana/dashboards/blink-overview.json)

## Soak Testing With k6

The repository already includes a longer guide in [soak-test-case.md](/Users/Matt.Maloney/projects/play/blink-financial/soak-test-case.md).

For log interpretation during or after a run, see [log-review-guide.md](/Users/Matt.Maloney/projects/play/blink-financial/log-review-guide.md).

The short version is:

1. Bring the stack up.
2. Verify `/api/v1/healthz` and `/api/v1/readyz`.
3. Run `k6` against `POST /api/v1/transactions`.
4. Monitor Redis queue depth, request latency, container CPU, and memory.

Example target:

```bash
k6 run -e BASE_URL=http://localhost:8080 k6/soak.js
```

## Smoke Testing With k6

For a quick, low-risk validation pass, use the light-load script in [light-load.js](/Users/Matt.Maloney/projects/play/blink-financial/k6/light-load.js).

Example target:

```bash
k6 run -e BASE_URL=http://localhost:8080 k6/light-load.js
```

The longer step-by-step guide is in [smoke-test-case.md](/Users/Matt.Maloney/projects/play/blink-financial/smoke-test-case.md).

## Design Notes

Some intentional simplifications in the current code:

- the HTTP server uses the standard library `http.ServeMux`
- the service reads config once at startup into a typed config struct
- Redis writes use `redis/go-redis` with its built-in connection pool
- the app returns quickly after queueing the request into Redis

These choices keep the current prototype small and easy to inspect.

## Current Limitations

This repository is useful for topology and local throughput experiments, but it is not yet a production ledger.

Important current limitations:

- each request is logged synchronously
- transaction payloads are pushed whole into Redis rather than being normalized or written to a WAL
- the sink is Redis-backed, not an append-only binary ledger

Those limitations matter for performance numbers. Treat current benchmark results as directional, not authoritative.

## Suggested Next Steps

Natural follow-up improvements for this repo are:

1. Introduce an append-only WAL on the RAM disk
2. Separate ingestion from background persistence workers
3. Add integration tests for the API and Redis sink behavior
4. If the gRPC stream needs to scale beyond a single direct endpoint, add an Envoy layer in front of the gRPC services so the TUI can keep one stable address.

## Additional Docs

- [checklist.md](/Users/Matt.Maloney/projects/play/blink-financial/checklist.md) for the original throughput checklist
- [architecture.md](/Users/Matt.Maloney/projects/play/blink-financial/architecture.md) for the ASCII architecture diagram
- [smoke-test-case.md](/Users/Matt.Maloney/projects/play/blink-financial/smoke-test-case.md) for quick `k6` validation runs
- [soak-test-case.md](/Users/Matt.Maloney/projects/play/blink-financial/soak-test-case.md) for k6 soak-test instructions
- [capability-expansion-guide.md](/Users/Matt.Maloney/projects/play/blink-financial/capability-expansion-guide.md) for protobuf/codegen and expansion workflows
- [transaction-monitor README](/Users/Matt.Maloney/projects/play/blink-financial/tui/transaction-monitor/README.md) for the Rust `ratatui` gRPC stream client
