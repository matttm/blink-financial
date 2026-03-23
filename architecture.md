# Blink Financial Local Architecture

## ASCII Diagram

```text
                          +----------------------+
                          |   Client / k6 / curl |
                          +----------+-----------+
                                     |
                                     | HTTP :8080
                                     v
                    +--------------------------------------+
                    |        HAProxy / Entry Point         |
                    |   container: haproxy                |
                    |   routes requests across replicas   |
                    +-----------+-------------+------------+
                                |             |
                                |             |
                ----------------+-------------+----------------
                |               |             |               |
                v               v             v               v
      +----------------+ +----------------+ +----------------+ +----------------+
      | Go App Replica | | Go App Replica | | Go App Replica | | Go App Replica |
      | app-1          | | app-2          | | app-3          | | app-N          |
      | /transactions  | | /transactions  | | /transactions  | | /transactions  |
      | /healthz       | | /healthz       | | /healthz       | | /healthz       |
      +--------+-------+ +--------+-------+ +--------+-------+ +--------+-------+
               \                  |                  |                  /
                \                 |                  |                 /
                 \                |                  |                /
                  \               |                  |               /
                   +--------------+------------------+--------------+
                                                  |
                                                  | TCP :6379
                                                  v
                                   +-------------------------------+
                                   |           Redis Sink          |
                                   |   append target / queue sink  |
                                   |   mounted to RAM disk path    |
                                   |   /data <- BLINK_RAMDISK_PATH |
                                   +---------------+---------------+
                                                   |
                                                   v
                                  +----------------------------------+
                                  | Host RAM Disk / tmpfs / Volume   |
                                  | e.g. /Volumes/blink-ramdisk      |
                                  | stores Redis append-only data    |
                                  +----------------------------------+
```

## Summary

```text
Client -> HAProxy -> [Go app replicas x3..10] -> Redis -> RAM disk
```
