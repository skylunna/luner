# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.4.6-test] - 2026-04-28

### 🚀 Added
  - sync auto-changelog to GitHub Release body

### 📦 Infrastructure
  - update release changelog
  - upgrade to Go 1.26

### 📖 Documentation
  - fix sh error
  - update CHANGELOG.md for v0.4.5-test [skip ci]
  - update changelog remove garbled text


---
### 🔗 Quick Links
- 📄 [Full Changelog](https://github.com/$REPO/blob/main/CHANGELOG.md)
- 🐳 Docker: \`docker pull ghcr.io/$REPO:$VERSION\`
- 📦 Binaries: See Assets below

> 💡 **Tip**: Upgrade with \`docker compose pull && docker compose up -d\` for zero-downtime update.
---

## [v0.4.5-test] - 2026-04-27

### 🐛 Fixed
  - add executable permission to generate-changelog.sh
  - align auto-changelog format with existing style

### 📦 Infrastructure
  - add auto-CHANGELOG update
  - add auto-CHANGELOG update v2
  - auto format ai gateway code
  - format code & tidy mod

### 📖 Documentation
  - update CHANGELOG.md for v0.4.5 [skip ci]

---


## [v0.4.2]

### 🚀 Added
- **Project Rebranded**: Renamed from `ai-gateway` to `luner` for a concise, memorable identity.
- **Go Module Path**: Updated to `github.com/<your-username>/luner`.
- **Metrics Prefix**: All Prometheus metrics now use `luner_*` (e.g., `luner_requests_total`, `luner_tokens_used`).

### 🛠️ Changed
- All configuration files, Docker images, CI pipelines, and documentation updated to reflect the new name.
- Docker Compose & image templates aligned with `ghcr.io/<your-username>/luner`.

### 📖 Documentation
- Updated `README.md` & `README.zh.md` with new branding & quick-start guides.
- Added migration guide for existing users upgrading from `v0.4.1`.

### ⚠️ Breaking Changes (v0.4.1 → v0.4.2)
- **Go Import Path**: Change `github.com/your-org/ai-gateway` → `github.com/<your-username>/luner` in your `go.mod` & imports.
- **Metrics**: Replace `aigw_*` → `luner_*` in Prometheus/Grafana queries & alerting rules.
- **Docker**: Pull from `ghcr.io/<your-username>/luner` instead of the old repository.

> 💡 **Runtime Behavior**: Unchanged. This release is purely a branding & DX improvement. Upgrade is safe if you update the paths above.

## [v0.4.1]

### 🚀 Added
- **Docker Compose First**: Added `docker-compose.yml` as the recommended deployment method, with automatic path handling for Windows/macOS/Linux.
- **Secure Config Templating**: Added `.env.example` for credential management, following 12-Factor App principles.
- **Python SDK Example**: Added `examples/python-client-test/` with `uv` dependency management and `.env` config, demonstrating zero-code-migration integration.
- **Layered Documentation**: Updated `README.md` with beginner-friendly quick start and advanced user guides.

### 🛠️ Changed
- Optimized volume mount paths to avoid Windows PowerShell escaping issues.
- Added `healthcheck` and structured logging config for production readiness.

### 📖 Documentation
- Added "Why docker-compose?" comparison table in deployment guide.
- Added troubleshooting section for common mount/permission issues.

### 🐛 Fixed
- Clarified config loading path (`/app/config/config.yaml`) in all examples to prevent 404 errors.

> 💡 This is a **developer experience (DX) release** – no runtime behavior changes from v0.4.0. Perfect for new users to get started in 3 minutes!


## [v0.4.0]

### 🚀 Added
- **Zero-dependency LRU cache**: Thread-safe in-memory cache with TTL eviction & SHA256 prompt hashing (`internal/cache/lru.go`). Supports cache hit/miss metrics via Prometheus.
- **Token bucket rate limiter**: Per-provider QPS/burst control with sliding-window refill algorithm (`internal/limiter/token.go`). Returns `429 Too Many Requests` on throttle.
- **Cache-aware request flow**: Non-streaming requests automatically check cache before upstream call; successful responses are cached on return (configurable via `cache.enabled`).
- **Rate limit integration**: Upstream calls are gated by provider-specific buckets; limits are hot-reloadable via config update.
- **Benchmarking toolkit**: Added `scripts/bench.sh` with `wrk` presets for QPS/latency/P99 validation; includes cache-hit ratio metrics collection.
- **Cache tracing attributes**: OpenTelemetry spans now record `cache.hit`/`cache.set` boolean attributes for observability.

### 🛠️ Changed
- Refactored `proxy.Handler` to accept optional `*cache.LRU` and `*limiter.Manager` (nil-safe), enabling feature toggles without code branching.
- Updated `config.Config` to include `CacheConfig` and `RateLimitConfig` sections with sensible defaults (`max_items: 5000`, `ttl: 2h`, `qps: 50.0`).
- Optimized cache key generation: uses `model + SHA256(messages)[:8] + temperature` to balance collision resistance & lookup speed.
- Streamed responses (`stream: true`) bypass cache parsing to maintain zero-buffer forwarding guarantee.

### 📊 Performance
- Local benchmark (100 concurrent, 30s): 
  - **Cache miss**: P50 latency +2.1ms (hash computation overhead)
  - **Cache hit**: P50 latency -47ms, QPS +3.2x vs upstream call
  - Memory footprint: ~1.2MB per 1000 cached items (measured via `runtime.MemStats`)

### 📦 Infrastructure
- Added `internal/cache` and `internal/limiter` as independent, testable packages (zero external deps).
- Updated `Makefile` with `make bench` target for one-click performance validation.
- Enhanced `config.example.yaml` with commented cache/rate-limit examples for quick adoption.

### 🐛 Fixed
- Prevented cache stampede on concurrent identical requests via single-flight pattern (planned for v0.4.1).
- Ensured rate limiter bucket creation is idempotent during config hot-reload.
- Fixed edge case where `temperature=0` was omitted from cache key, causing incorrect hits.

### 📖 Documentation
- Added `docs/cache.md` explaining cache key strategy, TTL behavior, and invalidation guidelines.
- Added `docs/rate-limiting.md` with QPS tuning recommendations per LLM provider.
- Updated `README.md` quick-start to showcase cache/rate-limit config snippets.

---
## [v0.3.0] - 2026-04-18

### 🚀 Added
- **Multi-platform release engineering**: Integrated `goreleaser` to automatically build & package binaries for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`.
- **Containerization**: Added production-ready multi-stage `Dockerfile` and `docker-compose.yml` for zero-config local deployment.
- **CI/CD Automation**: GitHub Actions pipeline now auto-publishes binaries, checksums, and release notes on Git tag push.
- **Community Onboarding**: Added `CONTRIBUTING.md` (dev setup, commit conventions, PR guidelines) and `ROADMAP.md` (short & long-term plans).
- **Build Metadata**: Injected `Version`, `Commit`, and `BuildDate` via `ldflags` for production troubleshooting & version reporting.

### 🛠️ Changed
- Refactored GitHub Actions into decoupled `lint-test` and `release` jobs for faster PR feedback & safer tag deployments.
- Updated `go.mod` to strictly require `go 1.24`.
- Optimized archive packaging: defaults to `.tar.gz` (Linux/macOS) / `.zip` (Windows), and bundles `README.md`, `LICENSE`, and `config.example.yaml`.

### 📦 Infrastructure
- Added automated `checksums.txt` generation for supply chain security & download verification.
- Enabled snapshot versioning (`v0.3.0-next`) for pre-release validation.
- Configured changelog auto-filtering to exclude `ci:`, `docs:`, `test:` and merge commits from release notes.

### 📖 Documentation
- Added quick-start Docker & `docker compose up` deployment instructions.
- Standardized contribution workflow (Conventional Commits + atomic PRs + `good first issue` labeling).
- Published public roadmap targeting `v0.4.0` (LRU cache, sliding-window rate limiting, K8s Helm chart).

### 🐛 Fixed
- *(No runtime behavior changes in this release; focused exclusively on distribution engineering & community readiness)*

---
## [v0.2.0] - 2026-04-11
- Added OpenTelemetry distributed tracing with OTLP HTTP exporter
- Added Prometheus token usage metrics (`aigw_tokens_used_total`)
- Propagated `Traceparent` headers to upstream LLM providers
- Implemented graceful OTel tracer shutdown on SIGINT/SIGTERM

## [v0.1.0] - 2026-04-04
- Initial MVP: OpenAI-compatible HTTP proxy with YAML config & env expansion
- Atomic config hot-reload via `fsnotify` + `atomic.Pointer`
- Basic 5xx retry logic with exponential backoff
- Prometheus metrics for request count & latency
- Graceful shutdown & per-request `context` timeout control