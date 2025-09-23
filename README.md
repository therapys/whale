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

### Quick install (Unix) via curl
Prebuilt binaries (recommended) if releases are available:
```bash
curl -fsSL https://raw.githubusercontent.com/yourusername/whale/main/scripts/install.sh | sh
```

Install a specific version (e.g., v0.1.0):
```bash
VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/yourusername/whale/main/scripts/install.sh | sh
```

If no release asset is found, the script falls back to `go install` (requires Go), then places `whale` into `/usr/local/bin` (use `sudo` if required). You can override destination with `DEST=/some/path`.

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
