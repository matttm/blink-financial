# Log Review Guide

This repository gives you several useful log streams during local load testing:

- `app`
- `haproxy`
- `redis`
- `prometheus`
- `grafana`

For throughput tuning, the first three matter the most.

A good default command is:

```bash
docker compose logs -f app haproxy redis
```

If you need to isolate one service:

```bash
docker compose logs -f app
docker compose logs -f haproxy
docker compose logs -f redis
```

## App Logs

The Go service in [main.go](/Users/Matt.Maloney/projects/play/blink-financial/cmd/ledger-sim/main.go) currently logs one line per request with:

- HTTP method
- request path
- total request duration

Example patterns to watch for:

- many requests taking close to `1.5s`
- bursts of `/api/v1/transactions` requests that slow down together
- readiness failures from `/api/v1/readyz`

What that usually means:

- request durations clustering near `1.5s` suggest the Redis enqueue path is stalling near the app timeout
- slow requests with few explicit errors suggest backpressure rather than outright process failure
- readiness failures usually mean Redis connectivity or Redis responsiveness is degraded

What to improve if this shows up:

- add response status code and outcome to request logs
- log Redis enqueue errors explicitly
- reduce per-request log volume during heavy tests if log I/O starts to distort results

## HAProxy Logs

HAProxy is configured in [haproxy.cfg](/Users/Matt.Maloney/projects/play/blink-financial/docker/haproxy/haproxy.cfg) with `option httplog`, so it is your best first source for ingress-side failures.

Look for:

- `5xx` responses
- backend health-check failures
- server timeouts
- repeated issues on one specific backend replica

### Interpreting 5xx Errors

Not every `5xx` means the same thing. The exact code helps narrow down where the failure is happening.

#### `500 Internal Server Error`

This usually means the upstream app handled the request but failed while processing it.

In this repository, a `500` would more likely come from the app itself if you later add explicit internal error handling paths. Right now the app more commonly returns `502` on Redis enqueue failure.

Inference:

- application bug
- unhandled internal condition
- failed downstream operation that the app translated into `500`

Improvement direction:

- inspect app logs first
- add explicit structured error logging around the failing code path
- add Prometheus counters by error outcome

#### `502 Bad Gateway`

This usually means HAProxy reached an upstream, but the upstream response was invalid or the app itself returned `502`.

In this repository, the app returns `502 Bad Gateway` when Redis enqueue fails in [main.go](/Users/Matt.Maloney/projects/play/blink-financial/cmd/ledger-sim/main.go).

Inference:

- Redis is unavailable or timing out from the app's perspective
- the app is still reachable, but its dependency path is failing
- this is often a dependency bottleneck, not an ingress bottleneck

Improvement direction:

- inspect `app` and `redis` logs together
- check Redis pool metrics in Grafana
- verify whether Redis latency, pool waits, or timeouts are spiking

#### `503 Service Unavailable`

This usually means HAProxy had no healthy backend available, or the service was considered unavailable at the time of routing.

Inference:

- app replicas are failing health checks
- the backend pool is flapping
- app containers may be starting, restarting, or becoming unhealthy under load

Improvement direction:

- inspect HAProxy health-check behavior
- inspect `docker compose ps`
- review app startup behavior and readiness
- verify whether replicas are crashing or failing `/api/v1/healthz`

#### `504 Gateway Timeout`

This usually means the reverse proxy waited too long for the upstream response.

Inference:

- the app accepted the connection but did not finish in time
- backend saturation is likely
- heavy queueing, slow Redis writes, or lock contention may be delaying completion

Improvement direction:

- compare HAProxy timeouts with app-level dependency timeouts
- inspect app latency and Redis pool wait metrics
- determine whether requests are blocked on Redis, CPU, or logging overhead

### HAProxy Patterns That Matter

These patterns tend to be more informative than the raw count of failures:

- `502` rising while app health checks stay green:
  the app is alive, but dependency operations are failing

- `503` rising with unhealthy backends:
  ingress cannot find healthy app instances

- `504` rising with long request durations:
  requests are reaching the app but are not finishing promptly

- failures concentrated on one backend:
  one replica may be overloaded, unhealthy, or misconfigured

## Redis Logs

Redis is the sink in this stack, configured in [redis.conf](/Users/Matt.Maloney/projects/play/blink-financial/docker/redis/redis.conf) with:

- `appendonly yes`
- `appendfsync everysec`

Look for:

- persistence or append-only file warnings
- fsync delays
- memory pressure warnings
- blocked clients
- connection churn

What that usually means:

- persistence warnings suggest the write path is slowing the queue sink
- connection churn suggests poor client reuse or pool sizing
- blocked clients suggest Redis is becoming the bottleneck under load

What to improve if this shows up:

- compare Redis logs with Redis pool metrics
- tune `go-redis` pool settings if wait counts or timeouts grow
- test with and without persistence to isolate storage-path cost

## Prometheus Logs

Prometheus logs are mostly useful for validating that your metrics are trustworthy.

Look for:

- scrape failures
- scrape timeouts
- config reload errors

What that usually means:

- Grafana may show partial or empty data
- metrics-based conclusions from that run may not be reliable

## Grafana Logs

Grafana logs are mostly useful for provisioning issues rather than system performance.

Look for:

- datasource errors
- dashboard provisioning failures
- plugin or startup errors

What that usually means:

- the observability surface is broken
- empty panels may be a Grafana query or datasource issue, not an app issue

## Recommended Review Flow

Use this order when a test starts failing:

1. Check `haproxy` logs to see whether failures are ingress-side, upstream-side, or timeout-related.
2. Check `app` logs to see whether request durations are spiking or dependency paths are failing.
3. Check `redis` logs to confirm whether Redis is actually the bottleneck.
4. Correlate the logs with Grafana and Prometheus:
   - app latency up plus Redis pool waits up usually means client-side contention
   - app failures up with quiet Redis logs usually points back to the app layer
   - Redis persistence warnings plus latency spikes suggest the sink path is slowing ingest

## Current Logging Gap

The current app logs are still fairly thin. The next useful improvement would be to add:

- response status code
- request outcome
- Redis enqueue error details

That would make container logs much more useful during load-test review.
