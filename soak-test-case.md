# Blink Financial Soak Test Guide

This document explains how to soak test the local Blink Financial stack with `k6`.

The current stack is:

- `haproxy` as the public entry point on port `8080`
- `app` replicas serving `POST /api/v1/transactions`
- `redis` as the sink, with its data mounted to your RAM disk path

## What The Service Expects

The Go app currently accepts:

- `POST /api/v1/transactions`
- A JSON batch object with `batch_id`, `source`, and `transactions`
- Each transaction must include `idempotency_key`, `tenant_id`, `account_id`, `type`, `amount`, and `occurred_at`

On success, the service returns `202 Accepted` and pushes the validated batch into Redis.

## 1. Prepare The Environment

Create an env file from the example:

```bash
cp .env.example .env
```

Make sure your RAM disk path exists and has a Redis data directory:

If you use the RAM disk helper script, it mounts a memory-backed filesystem at that path first. That is different from creating a plain directory on your SSD. The directory name may look the same, but the storage underneath it is different.

Example:

```bash
./scripts/setup_ramdisk.sh 1024 /Volumes/blink-ramdisk
```

Then create the Redis subdirectory:

```bash
mkdir -p /Volumes/blink-ramdisk/redis-data
```

This is worth doing explicitly because Docker can create missing bind-mount directories for you. If the RAM disk is not actually mounted, Docker may silently create a normal directory on disk and Redis will write there instead.

If you are not using `/Volumes/blink-ramdisk`, update `.env`:

```dotenv
BLINK_HTTP_PORT=8080
BLINK_RAMDISK_PATH=/your/ramdisk/path
BLINK_REDIS_LIST_KEY=blink:transactions
```

## 2. Start The Stack

Bring the local stack up with multiple app replicas:

```bash
docker compose up --build --scale app=3 -d
```

If you want more pressure on the load balancer and sink, increase replicas:

```bash
docker compose up --build --scale app=10 -d
```

## 3. Verify The Stack Before Load

Check that HAProxy is reachable:

```bash
curl http://localhost:8080/api/v1/healthz
```

Check that the app can talk to Redis:

```bash
curl http://localhost:8080/api/v1/readyz
```

You want:

- `/api/v1/healthz` to return `200`
- `/api/v1/readyz` to return `200`

## 4. Install k6

On macOS:

```bash
brew install k6
```

Check the install:

```bash
k6 version
```

## 5. Use The Checked-In Soak Script

This repository now includes a reusable script at `k6/soak.js`.

Useful environment variables:

- `BASE_URL`, default `http://localhost:8080`
- `BATCH_SIZE`, default `500`
- `HOT_ACCOUNT_COUNT`, default `100`
- `RATE`, default `200`
- `DURATION`, default `30m`
- `PREALLOCATED_VUS`, default `100`
- `MAX_VUS`, default `500`

## 6. Run The Soak Test

Run the script against the local stack:

```bash
k6 run -e BASE_URL=http://localhost:8080 k6/soak.js
```

Example with a larger batch and higher arrival rate:

```bash
k6 run \
  -e BASE_URL=http://localhost:8080 \
  -e BATCH_SIZE=1000 \
  -e RATE=1000 \
  -e DURATION=45m \
  k6/soak.js
```

## 7. Watch The System While It Runs

Watch container resource usage:

```bash
docker stats
```

Tail logs:

```bash
docker compose logs -f haproxy app redis
```

Check how many batches Redis has received:

```bash
docker compose exec redis redis-cli LLEN blink:transactions
```

## 8. How To Ramp Load

Do not jump straight to an aggressive rate. Increase load in steps and find the point where latency or failures spike.

Recommended progression:

1. Start at `rate: 200`
2. Move to `rate: 1000`
3. Move to `rate: 5000`
4. Increase until p95 latency or failure rate becomes unacceptable

You can also increase:

- batch size from `500` to `1000`
- app replicas from `3` to `10`
- duration from `30m` to multiple hours for a real soak test

## 9. What To Watch For

During the soak test, pay attention to:

- `http_req_failed`
- `http_req_duration`
- container CPU saturation
- container memory growth
- Redis queue depth growth
- whether `/api/v1/readyz` starts failing under sustained pressure

## 10. Current Bottlenecks In This Repo

The current sample app is useful for topology testing, but it is not yet optimized for serious throughput testing.

Known constraints:

- each request is still logged synchronously
- the app pushes whole request payloads into Redis without batching downstream writes

That means your first soak tests should be treated as baseline topology tests, not final performance numbers.

## 11. Recommended Next Improvements

Before pushing toward very high TPS, the next useful changes are:

1. Reduce or disable per-request logging during load tests
2. Move from this Redis sink toward the append-only WAL design in your checklist
