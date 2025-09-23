# whale

A tiny Go CLI that shows Docker container stats in a pretty table or JSON — either as a one-shot snapshot or in a live streaming view.

## Prerequisites
- Go 1.22+
- Docker daemon running locally or remotely
  - If remote, set `DOCKER_HOST`, and any TLS variables as usual

## Install / Build
```bash
# From the repo root
make tidy
make build
./bin/whale
```

## Usage
```bash
whale                 # list running containers with stats in a table
whale --all           # include stopped containers (stats zeroed; STATUS shows state)
whale --format=json   # emit JSON (useful for scripts)
whale --sort=mem      # sort by memory descending
whale --no-trunc      # show full IDs and names

# Live/streaming mode (table only)
whale --watch                   # continuously refresh; press Ctrl+C to exit
whale --watch --interval=1s     # set refresh interval (default 2s)

# Networks view
whale net                       # group containers by network (one-shot)
whale net --watch               # live network view (table only)
```

### JSON example
```bash
./bin/whale --format=json | jq .
```

### Sample table output
```
NAME            ID           STATUS          CPU %  MEM USAGE / LIMIT  MEM %  NET I/O        BLOCK I/O      PIDS
web             a1b2c3d4e5f6 Up 5 minutes    12.3   123.4MiB / 2.00GiB  6.0   12.0MiB / 8.0MiB  5.0MiB / 1.0MiB  12
worker          b2c3d4e5f6g7 Up 3 minutes     4.8    80.0MiB / 2.00GiB  3.9   8.0MiB / 6.0MiB   1.0MiB / 2.0MiB  7
redis           c3d4e5f6g7h8 Up 2 minutes     —      10.0MiB / 1.00GiB  1.0   —                —                3
```

- A single dash `—` indicates missing or zeroed metrics.
- If a container exits between list and stats read, it will show `STATUS=ERROR` and blanks for numeric fields.

### Live mode notes
- Live mode clears and redraws the screen each interval for a smooth, top-of-screen update.
- JSON format is not supported in `--watch` mode (for both default and `net` views).
- Use Ctrl+C to exit cleanly.

## Exit codes
- `0` on success
- Non-zero on fatal errors

## Notes
- CPU % calculation matches Docker CLI approach: `(cpuDelta / systemDelta) * onlineCPUs * 100` with safeguards when fields are missing (e.g., cgroup v2). Memory is shown as `usage / limit` with MEM % = `usage/limit*100`.

## License
MIT
