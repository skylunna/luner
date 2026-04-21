![Product Logo](./assets/logo/luner-logo-fat.png)
# ai-gateway

<p align="center">
  <a href="README.md">English</a> | <strong>中文</strong>
</p>

[![Release](https://img.shields.io/github/v/release/skylunna/ai-gateway?label=Release&color=blue)](https://github.com/your-org/ai-gateway/releases)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker)](https://docs.docker.com/compose/)
[![License](https://img.shields.io/github/license/skylunna/ai-gateway?color=green)](https://github.com/skylunna/ai-gateway/blob/main/LICENSE)


基于 Go 1.24 构建的轻量级、生产就绪的 LLM API 网关。通过与 OpenAI 兼容的接口，无缝实现对 AI 工作负载的代理、缓存、限流和可观测性监控。专为云原生环境和开发者优先体验而设计。

---

## ✨ 特性
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](https://github.com/your-org/ai-gateway/releases)

- **兼容 OpenAI**: 零侵入替换 `base_url`。现有的 Python/Node.js SDK 无需修改任何代码。
- **高性能**: 零依赖的 LRU 缓存 + 令牌桶限流。纯 Go 实现，内存占用极低且稳定。
- **配置热重载**: 基于 `fsnotify` 和 `atomic.Pointer` 实现原子级配置更新。路由变更零停机。
- **全面可观测性**: OpenTelemetry 链路追踪 + Prometheus 监控指标（延迟、Token 消耗、缓存命中率、限流情况）。
- **云原生就绪**: 支持多架构二进制文件、多阶段 Dockerfile 构建，提供开箱即用的 `docker-compose`。
- **安全设计**: 支持环境变量注入和 `.env` 模板化，完全符合 12-Factor App 规范。

---

## 🚀 快速开始

### 方案 1：Docker Compose（推荐）
```bash
git clone [https://github.com/skylunna/ai-gateway.git](https://github.com/skylunna/ai-gateway.git) && cd ai-gateway
cp config/config.example.yaml config/config.yaml
cp .env.example .env  # 在 .env 文件中配置你的 API Key
docker compose up -d

# 验证服务是否启动
curl http://localhost:8080/health
```

### 方案 2：Docker 运行
```bash
docker run -d --name ai-gateway -p 8080:8080 \
  -v "$(pwd)/config/config.yaml:/app/config.yaml:ro" \
  --env-file .env \
  ghcr.io/your-org/ai-gateway:v0.4.1
```

### 方案 3：源码编译
```bash
go build -o aigw ./cmd/aigw
./aigw -config config/config.yaml
```

## 配置说明
`ai-gateway` 实现了路由逻辑与敏感信息（密钥）的分离。你可以随时修改 `config/config.yaml`，变更会自动生效。
```yaml
# config/config.yaml
providers:
  - name: openai-prod
    base_url: "[https://api.openai.com/v1](https://api.openai.com/v1)"
    api_key: "${OPENAI_API_KEY}"  # 从 .env 文件中注入
    models: ["gpt-4o", "gpt-4o-mini"]
    timeout: "30s"

cache:
  enabled: true
  max_items: 5000
  default_ttl: "2h"

rate_limit:
  enabled: true
  providers:
    - name: openai-prod
      qps: 50.0
      burst: 10
```
>  **热重载**: 编辑 config.yaml 并保存，网关将自动且原子地切换路由表，绝不会断开当前的活动连接。

## 客户端集成
完美兼容任何支持 OpenAI 接口规范的客户端。只需更新 `base_url` 即可。
### Python (uv + openai)
```bash
cp .env.example .env  # 配置 AI_GATEWAY_BASE_URL 和 API_KEY
cd examples/python-sdk
uv run python test_integration.py
```

### 代码示例
```python
from openai import OpenAI

client = OpenAI(
    api_key="sk-xxx",  # 占位符，网关会使用配置中的真实 Key 覆盖它
    base_url="http://localhost:8080/v1"
)
response = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Hello"}]
)
```

## 可观测性
- **指标 (Metrics):** `GET /metrics` (Prometheus 格式)
    - `aigw_requests_total{status="200-cache"}` → 缓存命中率
    - `aigw_request_duration_seconds` → 延迟分布
    - `aigw_tokens_used_total{type="prompt|completion|total"}` → Token 使用统计
- **链路追踪 (Tracing):U** 设置 `OTEL_EXPORTER_OTLP_ENDPOINT` 即可自动将 Spans 导出至 Jaeger/Tempo
- **健康检查 (Health):** `GET /health` (兼容 K8s 探针)

--- 

## 📈 性能基准测试
测试环境：**Ubuntu 22.04 / 8核 vCPU / 16GB 内存** (生产目标环境)
测试工具：`hey -c 50 -n 1000` | [🔗 复现脚本](scripts/bench.sh)

| Scenario | QPS | P50 Latency | P99 Latency | Cache Hit | Upstream Calls | Memory (RSS) |
|----------|-----|-------------|-------------|-----------|----------------|--------------|
|  Cache Hit (`prompt+model+temp=0`) | **32,082** | **1.3ms** | **6.9ms** | **100%** | **0** | ~42MB |
|  Cold Start (first request) | ~95 | ~380ms | ~1.1s | 0% | 100% | ~45MB |
|  Direct to Upstream (baseline) | ~88 | ~365ms | ~1.0s | N/A | 100% | N/A |
|  Rate Limited (`qps=10, burst=2`) | ~10 | ~45ms | ~180ms | variable | throttled | ~43MB |

>  **缓存命中:** 相同的 prompt+model+temperature=0 请求将直接从内存 LRU 缓存中返回，网络开销为零。
>  **冷启动:** 首次请求包含上游延迟 + 代理路由开销（约 5-10ms 额外延迟）。
>  **跨平台:** 我们提供了适用于 Linux/macOS/Windows 的二进制文件。基准测试结果会因操作系统调度器和 Docker 运行时的不同而有所差异；请使用 scripts/bench.sh 在您自己的环境中进行测试。

---

## 参与贡献
我们非常欢迎提交 PR、Issue 和反馈意见。请查阅 [CONTRIBUTING.md](CONTRIBUTING.md) 了解环境搭建指引、提交规范以及带有 `good first issue` 标签的新手任务。