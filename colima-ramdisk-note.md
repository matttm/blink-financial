# Colima RAM Disk Note

If you are using Colima instead of Docker Desktop, a host RAM disk path may not automatically be visible to the Docker daemon inside the Colima VM.

This matters for this repository because Redis uses a bind mount:

```text
${BLINK_RAMDISK_PATH}/redis-data:/data
```

If Colima cannot see the host path, Redis may still write files inside the VM at the same apparent path, but your macOS host folder will remain empty. That means your persistence path is not actually using the host RAM disk you expected.

## Symptom

You may see all of the following at once:

- Redis has data in memory
- `/data` inside the container contains AOF or RDB files
- `/Volumes/blink-ramdisk/redis-data` on the macOS host looks empty

## Why This Happens

Bind mounts are resolved by the Docker daemon.

With Colima:

- Docker runs inside the Colima VM
- host paths must be mounted into that VM
- paths outside the default shared directories may not be available automatically

So Redis can end up writing to a VM-local path like `/Volumes/blink-ramdisk/redis-data` instead of your real host RAM disk.

## How To Check

Check your host path:

```bash
find /Volumes/blink-ramdisk/redis-data -maxdepth 3 -print
```

Check the same path inside Colima:

```bash
colima ssh -- find /Volumes/blink-ramdisk/redis-data -maxdepth 3 -print
```

If files appear inside Colima but not on the macOS host, the RAM disk path is not shared into the VM correctly.

## Fix

Stop Colima:

```bash
colima stop
```

Edit the Colima config:

```bash
colima start --edit
```

Make sure the `mounts:` section includes your RAM disk path as writable. Example:

```yaml
mounts:
  - location: /Users/Matt.Maloney
    writable: true
  - location: /Volumes/blink-ramdisk
    writable: true
```

Then restart your Compose stack:

```bash
sudo chown -R 999:1000 /Volumes/blink-ramdisk/redis-data
chmod -R u+rwX /Volumes/blink-ramdisk/redis-data
docker compose down
docker compose up --build -d
```

The `chown` step matters because the Redis container runs as user `999:1000` (`redis:redis`). If the bind-mounted host directory is owned only by your macOS user, Redis startup can fail with a permission error while trying to prepare `/data`.

After that, the host path and the container path should reflect the same Redis persistence files.

## If Redis Still Fails With `chown: .: Permission denied`

Some Colima shared mounts do not allow the Redis image's startup `chown` logic to succeed, even when the directory is otherwise writable from the host side.

In this repository, the Compose file avoids that path by running Redis directly as the Redis user:

```yaml
redis:
  user: "999:1000"
```

That prevents the image entrypoint from attempting the startup ownership change on `/data`.

## Re-Check

Run these again:

```bash
docker compose exec redis sh -lc 'find /data -maxdepth 3 -print'
find /Volumes/blink-ramdisk/redis-data -maxdepth 3 -print
```

If the setup is correct, both views should show the same files.
