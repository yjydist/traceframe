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
| `TRACEFRAME_REPOSITORY_MAX_FILE_BYTES` | `1048576` | Maximum readable repository file size |
| `TRACEFRAME_REPOSITORY_MAX_RESULT_BYTES` | `262144` | Maximum repository excerpt result size |
| `TRACEFRAME_REPOSITORY_MAX_RESULTS` | `100` | Maximum entries returned by one repository tool call |
| `TRACEFRAME_REPOSITORY_MAX_WALK_FILES` | `10000` | Maximum files inspected during a bounded repository walk |

The adaptive workflow routes discovery, requirements, architecture, quality/risk, and delivery specialists by stage. Significant decisions appear in the Decisions workspace and require approval against the current project and subject revisions before the workflow can advance.

At `REVIEW`, an isolated critic records typed findings without modifying the Project Model. The Reviews workspace enforces deterministic readiness checks and explicit risk, finding, and exact-revision baseline approvals before the project can enter `READY`.

The Artifacts workspace routes only applicable views and versions deterministic HTML, Markdown, JSON, and Mermaid projections with source entity IDs, revision provenance, checksums, and stale-state warnings. Markdown export produces an implementation packet from the latest immutable baseline when one exists.

Feature and refactor projects can grant and revoke local repository roots from Settings. Repository inspection is read-only, bounded by the configured limits, excludes ignored secrets and binary files, records redacted excerpts with hashes and line locators, and treats all source text as untrusted evidence.
