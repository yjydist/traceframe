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
| `TRACEFRAME_MODEL_PROVIDER` | `none` | Set to `openai` to enable specialist runs |
| `OPENAI_API_KEY` | unset | OpenAI API key; required only for the OpenAI provider |
| `TRACEFRAME_OPENAI_MODEL` | `gpt-5.6` | OpenAI model used by the adapter |
| `TRACEFRAME_OPENAI_BASE_URL` | `https://api.openai.com/v1` | OpenAI Responses API base URL |

The adaptive workflow routes discovery, requirements, architecture, quality/risk, and delivery specialists by stage. Significant decisions appear in the Decisions workspace and require approval against the current project and subject revisions before the workflow can advance.
