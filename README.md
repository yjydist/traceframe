# Traceframe

Traceframe is a local-first adaptive software design workspace. The implementation follows [DESIGN.md](./DESIGN.md).

## Prerequisites

- Go 1.26 or newer
- Node.js 22 or newer
- npm

## Run locally

```sh
make dev
```

The command installs frontend dependencies, builds the web application, applies SQLite migrations, and starts the server at <http://127.0.0.1:8080>.

## Checks

```sh
make check
```

Environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `TRACEFRAME_ADDR` | `127.0.0.1:8080` | HTTP listen address; non-loopback addresses are rejected |
| `TRACEFRAME_DATABASE_PATH` | `data/traceframe.db` | SQLite database path |
| `TRACEFRAME_WEB_DIR` | `web/dist` | Built frontend directory |
| `TRACEFRAME_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
