# Blink Financial Smoke Test Guide

This document explains how to run a light, low-risk `k6` smoke test against the local Blink Financial stack.

Use this test when you want to confirm:

- the stack boots cleanly
- HAProxy can route traffic to the app
- the app accepts transaction batches
- Redis receives data

This is not meant to characterize sustained throughput. It is a quick confidence check before heavier load or soak testing.

## What This Smoke Test Hits

The current stack is:

- `haproxy` as the public entry point on port `8080`
- `app` replicas serving `POST /api/v1/transactions`
- `redis` as the sink, with its data mounted to your RAM disk path

The smoke test targets:

- `POST /api/v1/transactions`

On success, the service returns `202 Accepted`.

## 1. Prepare The Environment

Create an env file from the example:

```bash
cp .env.example .env
```

Make sure your RAM disk path exists and has a Redis data directory.

If you use the RAM disk helper script, it mounts a memory-backed filesystem at that path first. That is different from creating a plain directory on your SSD. The directory name may look the same, but the storage underneath it is different.

Example:

```bash
./scripts/setup_ramdisk.sh 1024 /Volumes/blink-ramdisk
```

Then create the Redis subdirectory:

```bash
mkdir -p /Volumes/blink-ramdisk/redis-data
```

If you are not using `/Volumes/blink-ramdisk`, update `.env`:

```dotenv
BLINK_HTTP_PORT=8080
BLINK_RAMDISK_PATH=/your/ramdisk/path
BLINK_REDIS_LIST_KEY=blink:transactions
```

## 2. Start The Stack

Start the local stack:

```bash
docker compose up --build --scale app=3 -d
```

For a smoke test, you usually do not need aggressive scaling. Three app replicas is enough to validate routing and basic fan-out.

## 3. Verify The Stack Before Running k6

Check that the public entrypoint is reachable:

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

If `k6` is not installed yet, use:

```bash
./scripts/install_k6.sh
```

Then verify:

```bash
k6 version
```

## 5. Use The Checked-In Smoke Script

This repository includes a reusable light-load script at `k6/light-load.js`.

Default behavior:

- small batches: `25`
- low concurrency: `1 -> 5 VUs`
- short runtime: about `105s`
- relaxed pacing with sleeps between requests

Useful environment variables:

- `BASE_URL`, default `http://localhost:8080`
- `BATCH_SIZE`, default `25`
- `HOT_ACCOUNT_COUNT`, default `20`
- `START_VUS`, default `1`
- `TARGET_VUS`, default `5`
- `RAMP_UP`, default `30s`
- `STEADY`, default `60s`
- `RAMP_DOWN`, default `15s`
- `SLEEP_SECONDS`, default `0.5`

## 6. Run The Smoke Test

Run the default light-load test:

```bash
k6 run -e BASE_URL=http://localhost:8080 k6/light-load.js
```

Example with a slightly stronger but still casual load:

```bash
k6 run \
  -e BASE_URL=http://localhost:8080 \
  -e BATCH_SIZE=50 \
  -e TARGET_VUS=10 \
  -e STEADY=90s \
  k6/light-load.js
```

## 7. What Success Looks Like

A successful smoke test should show:

- very low or zero request failures
- `202` responses from the transaction endpoint
- healthy `haproxy`, `app`, and `redis` containers
- increasing Redis queue depth while the test runs

## 8. What To Watch While It Runs

Watch resource usage:

```bash
docker stats
```

Tail logs:

```bash
docker compose logs -f haproxy app redis
```

Check Redis queue depth:

```bash
docker compose exec redis redis-cli LLEN blink:transactions
```

## 9. When To Use Smoke Test vs Soak Test

Use the smoke test when:

- you just changed routing or container config
- you want a quick validation after a code change
- you want to verify the stack before a longer test

Use the soak test when:

- you want sustained pressure over time
- you are watching for latency drift
- you are trying to expose long-run instability or resource growth

For the long-run version, see [soak-test-case.md](/Users/Matt.Maloney/projects/play/blink-financial/soak-test-case.md).
