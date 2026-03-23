# Blink Financial Soak Test Guide

This document explains how to soak test the local Blink Financial stack with `k6`.

The current stack is:

- `haproxy` as the public entry point on port `8080`
- `app` replicas serving `POST /transactions`
- `redis` as the sink, with its data mounted to your RAM disk path

## What The Service Expects

The Go app currently accepts:

- `POST /transactions`
- A non-empty request body
- JSON is fine for the load test payload

On success, the service returns `202 Accepted` and pushes the batch into Redis.

## 1. Prepare The Environment

Create an env file from the example:

```bash
cp .env.example .env
```

Make sure your RAM disk path exists and has a Redis data directory:

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
curl http://localhost:8080/healthz
```

Check that the app can talk to Redis:

```bash
curl http://localhost:8080/readyz
```

You want:

- `/healthz` to return `200`
- `/readyz` to return `200`

## 4. Install k6

On macOS:

```bash
brew install k6
```

Check the install:

```bash
k6 version
```

## 5. Create A Soak Script

Create a file named `soak.js` with the following contents:

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

const baseUrl = __ENV.BASE_URL || 'http://localhost:8080';

function makeBatch(size) {
  const txs = [];
  for (let i = 0; i < size; i++) {
    txs.push({
      id: `${__VU}-${__ITER}-${i}`,
      account_id: `acct-${i % 100}`,
      amount_cents: (i % 5000) + 1,
      currency: 'USD',
      ts: new Date().toISOString(),
    });
  }
  return JSON.stringify(txs);
}

export const options = {
  scenarios: {
    soak: {
      executor: 'constant-arrival-rate',
      rate: 200,
      timeUnit: '1s',
      duration: '30m',
      preAllocatedVUs: 100,
      maxVUs: 500,
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

const payload = makeBatch(500);

export default function () {
  const res = http.post(`${baseUrl}/transactions`, payload, {
    headers: { 'Content-Type': 'application/json' },
    timeout: '2s',
  });

  check(res, {
    accepted: (r) => r.status === 202,
  });

  sleep(0.1);
}
```

## 6. Run The Soak Test

Run the script against the local stack:

```bash
k6 run -e BASE_URL=http://localhost:8080 soak.js
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
- whether `/readyz` starts failing under sustained pressure

## 10. Current Bottlenecks In This Repo

The current sample app is useful for topology testing, but it is not yet optimized for serious throughput testing.

Known constraints:

- each request opens a fresh Redis TCP connection
- each request logs synchronously
- the app pushes whole request payloads into Redis without batching downstream writes
- there are no Prometheus metrics yet

That means your first soak tests should be treated as baseline topology tests, not final performance numbers.

## 11. Recommended Next Improvements

Before pushing toward very high TPS, the next useful changes are:

1. Add a checked-in `k6` script under a `k6/` directory
2. Add Prometheus metrics for request rate, latency, and Redis write failures
3. Replace per-request Redis connection setup with a pooled or persistent approach
4. Reduce or disable per-request logging during load tests
5. Move from this Redis sink toward the append-only WAL design in your checklist
