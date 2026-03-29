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
│   │   └── provisioning/
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
4. The app pushes the request payload into Redis using a raw RESP write.
5. Redis persists its append-only data under the host path configured by `BLINK_RAMDISK_PATH`.

## API

The current service is mounted under the `/api/v1` prefix.

Endpoints:

- `GET /api/v1/healthz`
- `GET /api/v1/readyz`
- `POST /api/v1/transactions`
- `GET /metrics`

Behavior:

- `healthz` returns `200 OK` if the process is up.
- `readyz` returns `200 OK` if Redis is reachable.
- `transactions` accepts a non-empty request body and returns `202 Accepted` after pushing the payload into Redis.
- `metrics` exposes Prometheus-formatted application and Go runtime metrics.

Example:

```bash
curl -i http://localhost:8080/api/v1/healthz

curl -i http://localhost:8080/api/v1/readyz

curl -i \
  -X POST http://localhost:8080/api/v1/transactions \
  -H 'Content-Type: application/json' \
  -d '[{"id":"txn-1","account_id":"acct-1","amount_cents":1250,"currency":"USD"}]'
```

## Configuration

The service reads its configuration from environment variables at startup.

App variables:

- `PORT`
- `REDIS_ADDR`
- `REDIS_LIST_KEY`
- `HOSTNAME`

Compose variables:

- `BLINK_HTTP_PORT`
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

3. Start the stack.

```bash
docker compose up --build --scale app=3 -d
```

4. Check the health endpoints.

```bash
curl http://localhost:8080/api/v1/healthz
curl http://localhost:8080/api/v1/readyz
```

5. Send a sample batch.

```bash
curl -X POST http://localhost:8080/api/v1/transactions \
  -H 'Content-Type: application/json' \
  -d '[{"id":"txn-1","account_id":"acct-1","amount_cents":1250,"currency":"USD"}]'
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

Compose defines a single `app` service. To increase concurrency, scale that service at runtime:

```bash
docker compose up --build --scale app=10 -d
```

HAProxy is configured to distribute requests across the `app` containers discovered on the Compose network.

## Observability

Prometheus scrapes the app's `/metrics` endpoint and Grafana is pre-provisioned with a Prometheus datasource.

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

## Soak Testing With k6

The repository already includes a longer guide in [soak-test-case.md](/Users/Matt.Maloney/projects/play/blink-financial/soak-test-case.md).

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
- Redis writes are sent using raw TCP plus RESP rather than a client library
- the app returns quickly after queueing the request into Redis

These choices keep the current prototype small and easy to inspect.

## Current Limitations

This repository is useful for topology and local throughput experiments, but it is not yet a production ledger.

Important current limitations:

- each request opens a fresh Redis connection
- each request is logged synchronously
- transaction payloads are pushed whole into Redis rather than being normalized or written to a WAL
- there are no Prometheus metrics yet
- the sink is Redis-backed, not an append-only binary ledger

Those limitations matter for performance numbers. Treat current benchmark results as directional, not authoritative.

## Suggested Next Steps

Natural follow-up improvements for this repo are:

1. Add Prometheus metrics and pprof endpoints
2. Replace per-request Redis dialing with connection reuse
3. Introduce an append-only WAL on the RAM disk
4. Separate ingestion from background persistence workers
5. Add integration tests for the API and Redis sink behavior

## Additional Docs

- [checklist.md](/Users/Matt.Maloney/projects/play/blink-financial/checklist.md) for the original throughput checklist
- [architecture.md](/Users/Matt.Maloney/projects/play/blink-financial/architecture.md) for the ASCII architecture diagram
- [smoke-test-case.md](/Users/Matt.Maloney/projects/play/blink-financial/smoke-test-case.md) for quick `k6` validation runs
- [soak-test-case.md](/Users/Matt.Maloney/projects/play/blink-financial/soak-test-case.md) for k6 soak-test instructions
