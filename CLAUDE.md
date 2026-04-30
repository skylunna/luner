# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o aigw ./cmd/luner

# Run
./aigw -config config/config.yaml

# Test (all)
go test -race -v -cover ./...

# Test (single package)
go test -run TestProxyHandler ./internal/proxy/

# Lint
golangci-lint run ./...

# Docker (gateway only)
docker compose -f docker-compose.yml up -d

# Docker (gateway + full monitoring stack)
docker compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d
```

## Architecture

`luner` is a stateless LLM API gateway. It sits between clients and upstream LLM providers (OpenAI, Alibaba Qwen, etc.) and adds caching, rate limiting, and observability — all with zero code changes on the client side (drop-in `base_url` replacement).

**Request flow:**

```
Client POST /v1/chat/completions
  → proxy.Handler.ServeHTTP (internal/proxy/handler.go)
      → rate limit check (limiter.Manager, per-provider token bucket)
      → cache lookup (cache.LRU, non-streaming only; key = model:sha256(messages):temperature)
      → resolve provider from model name (config.Providers[].Models)
      → inject stream_options.include_usage for streaming requests
      → forward to upstream with OTel trace context propagation
      → on response: record Prometheus metrics + update cache (non-streaming)
                      or parse SSE chunks for token usage (streaming)
```

**Package responsibilities:**

| Package | Role |
|---|---|
| `cmd/luner` | Entry point; wires all components, HTTP server lifecycle |
| `internal/config` | YAML config with `${ENV_VAR}` expansion; hot-reload via `Loader` (`atomic.Pointer[Config]` + fsnotify) |
| `internal/proxy` | Core HTTP handler, SSE stream passthrough, retry on upstream 5xx |
| `internal/cache` | Zero-dependency in-memory LRU with TTL |
| `internal/limiter` | Per-provider token-bucket rate limiter |
| `internal/metrics` | Prometheus counters/histograms: `luner_requests_total`, `luner_request_duration_seconds`, `luner_tokens_used_total` |
| `internal/trace` | OpenTelemetry init (skipped in dev if `OTEL_EXPORTER_OTLP_ENDPOINT` is unset) |
| `internal/api` | OpenAI-compatible error response format with TraceID injection |

**Ports:**
- `:8080` — proxy (main gateway)
- `:9090` — Prometheus metrics endpoint

## Configuration

Config file uses YAML + `${ENV_VAR}` interpolation for secrets. The file is watched at runtime; edits apply atomically without restarting the process. **Exception:** `server.listen`, `read_timeout`, and `write_timeout` require a process restart to take effect.

Providers are matched to requests by model name — a request for `"gpt-4o"` routes to whichever provider lists that model in its `models` array.

## Key Design Decisions

- **Cache key** includes `temperature` — only `temperature=0` requests are effectively cached across users since any variation produces a unique key. Streaming responses are never cached.
- **`Loader`** (`config/watcher.go`) holds an `atomic.Pointer[Config]` so the hot-reload path is lock-free for readers.
- **Tests** use `config.NewLoaderFromCfg(cfg)` to inject mock configs without touching the filesystem; upstream calls are handled with `httptest.NewServer`.
- `retry.go` exports `ExecuteWithRetry` but the handler uses its own private `executeWithRetry` method — they are functionally equivalent but not shared.
