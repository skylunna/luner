# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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