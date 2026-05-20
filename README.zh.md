![Product Logo](./assets/logo/luner-logo-fat.png)

# luner

<p align="center">
  <a href="README.md">English</a> | <strong>中文</strong>
</p>

[![Release](https://img.shields.io/github/v/release/skylunna/luner?label=Release&color=blue)](https://github.com/skylunna/luner/releases)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker)](https://docs.docker.com/compose/)
[![License](https://img.shields.io/github/license/skylunna/luner?color=green)](https://github.com/skylunna/luner/blob/main/LICENSE)


**具有实时治理功能的AI网关** —— 在错误的LLM请求花费你的钱之前阻止它们。代理、缓存、速率限制，并通过OpenAI兼容的接口观察您的AI工作负载。
内置的CEL策略引擎在请求到达LLM提供商之前强制执行预算和模型分配。

---

![architecture](./assets/architecture/architecture-v0.5.0.png)

---

## ✨ 特性

### CEL 策略引擎 — 花钱前先拦截

基于 Google CEL 表达式的实时治理：

```json
// 超出预算直接拦截
{ "expression": "cost_usd > 10.0", "action": "block" }

// 高频请求自动降级到便宜模型
{ "expression": "request_count > 100 && model == 'gpt-4o'", "action": "downgrade" }

// 可疑用量触发告警
{ "expression": "tokens_used > 50000", "action": "alert" }
```

策略存储于 SQLite，热重载无需重启。可实现模型白名单、用户级消费上限或自定义路由逻辑。

### 兼容 OpenAI
零侵入替换 `base_url`，兼容任何 OpenAI 兼容 SDK。

### LRU 缓存
 — 零依赖内存缓存，支持 TTL 配置。仅缓存非流式请求；缓存键包含 `model + messages + temperature`。

### 令牌桶限流
按 Provider 配置 QPS + Burst，超限立即返回 429。

### 全面可观测性

OpenTelemetry 链路追踪（OTLP）+ Prometheus 指标，Span 级别的成本归因存储于 SQLite。

### 内置 Web 控制台

暗色主题 React SPA，与网关同端口提供服务。包含 Dashboard、Traces 浏览器、Policies 管理、Settings 查看器，无需额外部署。

### 配置热重载

`fsnotify` + `atomic.Pointer[Config]` 原子切换路由表，零停机。

### 云原生就绪

多架构二进制、多阶段 Dockerfile、开箱即用的 `docker-compose`。

---

## 🚀 快速开始

[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](https://github.com/skylunna/luner/releases)

| 方式 | 时间 | 前置条件 |
|---|---|---|
| **Demo 模式** | ~2 分钟 | 仅需 Docker，无需 API Key |
| **生产模式** | ~5 分钟 | Docker + 真实 LLM API Key |
| **源码编译** | ~5 分钟 | Go 1.26+ 和 Node 20+ |

---

### Demo 模式 — 2 分钟体验

无需 API Key。使用内置 Mock LLM，自动向 Dashboard 写入演示数据。

```bash
git clone https://github.com/skylunna/luner.git
cd luner

docker compose up -d --build
docker compose logs -f seed-data    # 等待出现 "Demo data ready"
```

打开 **http://localhost:8080**，即可看到已预加载的 Trace 时间线、成本图表和策略列表。

```bash
curl http://localhost:8080/api/health    # 返回 {"status":"ok"} 即正常

docker compose down      # 停止（保留数据库）
docker compose down -v   # 停止并删除数据库
```

---

### 生产模式 — 接入真实 LLM

支持任意 OpenAI 兼容的 Provider。默认配置使用阿里云 Qwen；在 `config.prod.yaml` 中取消注释即可切换到 OpenAI。

**第 1 步 — 前置条件**

- Docker 24+ 且已安装 Compose V2（执行 `docker compose version` 确认）
- LLM Provider 的 API Key

**第 2 步 — 克隆仓库**

```bash
git clone https://github.com/skylunna/luner.git
cd luner
```

**第 3 步 — 配置 API Key**

在**仓库根目录**创建 `.env` 文件：

```bash
echo "DASHSCOPE_API_KEY=sk-..."  > .env    # 阿里云百炼（DashScope / Qwen）
# echo "OPENAI_API_KEY=sk-..."  >> .env    # OpenAI（同时取消注释 config.prod.yaml 中的 openai provider）
```

**第 4 步 — （可选）修改 Provider 配置**

`deployments/production/config.prod.yaml` 已为 Qwen 预配置好。可在此切换 Provider、调整限流参数或修改缓存 TTL。luner 支持热重载，保存后即时生效，无需重启。

**第 5 步 — 构建镜像**

```bash
cd deployments/production

# 标准构建
docker compose -f docker-compose.prod.yml build

# 国内网络 — 使用 Go 模块镜像加速
docker compose -f docker-compose.prod.yml build --build-arg GOPROXY=https://goproxy.cn,direct
```

> 首次构建需 3–5 分钟（编译前端 + Go 二进制）。后续启动无需重新构建，秒级拉起。

**第 6 步 — 启动**

```bash
docker compose -f docker-compose.prod.yml up -d

docker compose -f docker-compose.prod.yml ps         # 确认 Status = healthy
docker compose -f docker-compose.prod.yml logs luner  # 查看启动日志
```

**第 7 步 — 验证**

```bash
# 健康检查
curl http://localhost:8080/api/health

# 发送一条测试请求
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer any-key" \
  -H "X-Luner-Agent: my-app" \
  -H "X-Luner-User: user-1" \
  -d '{"model":"qwen-turbo","messages":[{"role":"user","content":"你好"}]}'

# 浏览器打开 Web 控制台
# http://<服务器公网IP>:8080
```

> **云服务器用户：** 请在防火墙或安全组中放行 **8080**（网关 + Web 控制台）和 **9090**（Prometheus 指标）入站端口。

**日常运维**

```bash
# 查看实时日志
docker compose -f docker-compose.prod.yml logs -f luner

# 停止网关
docker compose -f docker-compose.prod.yml down

# 更新配置无需重启 — 直接保存 config.prod.yaml，luner 自动热重载
# 例外：server.listen / read_timeout / write_timeout 变更需重启进程
```

---

### 源码编译

需要 Go 1.26+ 和 Node 20+。

```bash
git clone https://github.com/skylunna/luner.git
cd luner

make build    # 构建前端（npm）再构建 Go 二进制 → bin/luner

cp config/config.example.yaml config/config.yaml
# 编辑 config.yaml：填入 Provider 的 base_url 和 api_key

./bin/luner --config config/config.yaml
```

---

### 完整监控栈（Prometheus + Grafana + Tempo）

```bash
docker compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d
# Grafana:    http://localhost:3000  (admin / admin)
# Prometheus: http://localhost:9091
```

---

### 常见问题

| 现象 | 解决方法 |
|---|---|
| `seed-data` 容器立即退出 | `docker compose logs luner` — luner 可能仍在启动中，等待 healthy 后重试 |
| 8080 端口被占用 | `lsof -ti:8080 \| xargs kill`，或修改 compose 文件中的端口映射 |
| Go 模块下载超时 | 构建命令加上 `--build-arg GOPROXY=https://goproxy.cn,direct` |
| Dashboard 无新数据 | 相同内容的请求命中了 LRU 缓存，换一条不同的 prompt 发送，或临时将 `cache.enabled` 设为 `false` |
| 容器反复重启 | `docker logs <容器名>` — 检查是否有 *config not found* 或 *readonly database* 错误 |
| 重新部署后 Dashboard 无数据 | 执行 `docker compose run --rm seed-data` 重新写入演示数据 |

---

## 配置说明

`luner` 实现了路由逻辑与敏感信息的分离。随时修改 `config/config.yaml`，变更原子生效，无需重启。

```yaml
# config/config.yaml
providers:
  - name: openai-prod
    base_url: "https://api.openai.com/v1"
    api_key: "${OPENAI_API_KEY}"   # 从环境变量展开
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

storage:
  backend: sqlite
  sqlite:
    path: "data/luner.db"
```

> **热重载**：保存 `config.yaml` 后，网关原子切换路由表，不会断开任何活跃连接。  
> **例外**：`server.listen`、`read_timeout`、`write_timeout` 变更需重启进程才能生效。

---

## Web 控制台

Web 控制台是一个暗色主题的 React SPA，通过 **`http://localhost:8080/`** 访问。构建时嵌入 Go 二进制，无需独立部署。

| 页面 | 说明 |
|---|---|
| **Dashboard** | 汇总指标（Traces 数、Spans 数、成本、延迟、错误率）+ 实时缓存指标面板（命中率、驱逐数、当前大小，每 5 秒刷新）+ 按状态的请求统计和按类型的 Token 统计图表 |
| **Traces** | 分页 Trace 列表，支持 Agent/User 筛选和状态 Tab 切换。点击任意 Trace 查看 Span 时间线 |
| **Trace Detail** | 完整 Span 树，含每个 Span 的 Token 数、成本、耗时及比例时间线 |
| **Policies** | 完整 CRUD：列出、创建、删除、切换 CEL 策略。每条策略展示表达式、执行动作、优先级和启用状态 |
| **Settings** | 网关配置查看器（只读，提示热重载路径） |


### Demo
![demo](./assets/demo/demo-web-v0.5.0.png)

---

## CEL 策略引擎

策略是对每条入站请求执行求值的 CEL 表达式。匹配成功时触发三种动作之一：`block`（返回 403 拒绝）、`alert`（记录日志后放行）、`downgrade`（替换为更便宜的模型）。

**可用变量：**

| 变量 | 类型 | 说明 |
|---|---|---|
| `model` | string | 请求中的模型名 |
| `user_id` | string | `X-User-ID` 请求头 |
| `tenant_id` | string | `X-Tenant-ID` 请求头 |
| `request_count` | int | 该用户最近一分钟的请求次数 |
| `cost_usd` | double | 该用户最近一分钟的累计成本（美元） |
| `tokens_used` | int | 该用户最近一分钟消耗的 Token 数 |

**策略示例：**

```json
// 拦截不在白名单的模型
{ "name": "model-allowlist", "expression": "!(model in ['gpt-4o-mini', 'claude-haiku-4-5'])", "action": "block" }

// 单用户一分钟内消费超过 $0.10 时告警
{ "name": "spend-alert", "expression": "cost_usd > 0.10", "action": "alert" }

// 高频用户自动降级到更便宜的模型
{ "name": "auto-downgrade", "expression": "request_count > 100", "action": "downgrade" }
```

策略存储于 SQLite，可通过 REST API 或 Web 控制台管理，变更即时生效无需重启。

---

## API 接口

所有 REST 接口与代理、Web 控制台共用 `:8080` 端口。

标有 ★ 的接口无需配置 SQLite 存储，始终可用。

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/health` ★ | 健康检查（K8s 存活探针） |
| `GET` | `/api/metrics/live` ★ | 实时 JSON 快照：缓存命中率、驱逐数、按状态请求数、按类型 Token 数 |
| `GET` | `/api/dashboard/summary` | 汇总统计：Traces、Spans、成本、延迟、错误率 |
| `GET` | `/api/dashboard/cost` | 按 Agent 分类的成本明细 |
| `GET` | `/api/traces` | 分页 Trace 列表（`?page=1&page_size=20&agent_name=&user_id=`） |
| `GET` | `/api/traces/{trace_id}` | Trace 详情：汇总 + Span 树 + 时间线 |
| `GET` | `/api/policies` | 获取所有策略 |
| `POST` | `/api/policies` | 创建策略 |
| `GET` | `/api/policies/{id}` | 获取单条策略 |
| `PUT` | `/api/policies/{id}` | 更新策略 |
| `DELETE` | `/api/policies/{id}` | 删除策略 |
| `POST` | `/api/policies/reload` | 强制重新编译 CEL 程序 |
| `POST` | `/v1/chat/completions` ★ | 代理接口（兼容 OpenAI） |
| `GET` | `/metrics` ★ | Prometheus 指标 |

---

## 可观测性

### Prometheus 指标（`:9090/metrics`）

| 指标 | 标签 | 说明 |
|---|---|---|
| `luner_requests_total` | `provider`, `model`, `status` | 请求计数 |
| `luner_request_duration_seconds` | `provider`, `model` | 延迟直方图 |
| `luner_tokens_used_total` | `provider`, `model`, `type` | Token 统计（`prompt`/`completion`/`total`） |
| `luner_cache_hits_total` | — | LRU 缓存命中次数 |
| `luner_cache_misses_total` | — | LRU 缓存未命中次数 |
| `luner_cache_evictions_total` | `reason`（`ttl`/`capacity`） | 因 TTL 过期或容量溢出被驱逐的缓存条目数 |
| `luner_cache_size` | — | 当前 LRU 缓存中的条目数（Gauge） |

### Grafana 仪表盘
![demo](./assets/demo/demo-grafana-0.4.0.png)

### OpenTelemetry 链路追踪

设置 `OTEL_EXPORTER_OTLP_ENDPOINT` 即可将 Span 导出到任意 OTLP 兼容后端（Jaeger、Grafana Tempo、Honeycomb 等）。未设置时静默跳过，开发环境不会报错。

---

## 客户端集成

luner 是**零侵入代理**——应用代码唯一需要修改的就是 `base_url`。真实 API Key 保存在网关的 `config.yaml` 中；客户端传任意非空字符串即可。

### Python（OpenAI SDK）

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://your-luner-host:8080/v1",
    api_key="any-value",   # 真实 Key 在网关 config.yaml 中配置
)

response = client.chat.completions.create(
    model="qwen-turbo",    # 或 gpt-4o-mini、claude-haiku-4-5 等
    messages=[{"role": "user", "content": "你好"}],
    temperature=0,         # temperature=0 启用 LRU 缓存
)
print(response.choices[0].message.content)
```

### 链路追踪请求头

在每个请求中附加可选的 Header，以在 Web 控制台中丰富 Trace 信息，并为 CEL 策略提供用户级上下文：

```python
client = OpenAI(
    base_url="http://your-luner-host:8080/v1",
    api_key="any-value",
    default_headers={
        "X-Luner-Agent":  "my-agent",       # 在 Traces 页面显示的 Agent 名称
        "X-Luner-User":   "user-123",        # 填充 CEL 策略中的 user_id 变量
        "X-Luner-Tenant": "acme-corp",       # 填充 CEL 策略中的 tenant_id 变量
    },
)
```

### LangChain

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    model="qwen-turbo",
    base_url="http://your-luner-host:8080/v1",
    api_key="any-value",
    temperature=0,
)
```

### 流式输出

流式响应开箱即用。luner 会解析 SSE 块以提取 Token 用量，并记录到 Trace 中：

```python
with client.chat.completions.create(
    model="qwen-turbo",
    messages=[{"role": "user", "content": "给我讲个故事"}],
    stream=True,
) as stream:
    for chunk in stream:
        print(chunk.choices[0].delta.content or "", end="", flush=True)
```

> **注意：** 流式响应**不会被缓存**（设计如此）。只有 `temperature=0` 的非流式请求才会命中 LRU 缓存。

### 生产封装模式

在生产服务中，将网关 URL 和追踪 Header 集中在一处管理：

```python
# llm_client.py
import os
from openai import OpenAI

_client = OpenAI(
    base_url=os.environ["LUNER_URL"] + "/v1",
    api_key="gateway",
    default_headers={
        "X-Luner-Agent":  os.environ.get("SERVICE_NAME", "unknown"),
        "X-Luner-Tenant": os.environ.get("TENANT_ID", "default"),
    },
)

def chat(messages, *, model="qwen-turbo", user_id=None, **kwargs):
    headers = {"X-Luner-User": user_id} if user_id else {}
    return _client.chat.completions.create(
        model=model, messages=messages, extra_headers=headers, **kwargs
    )
```

### 端到端演示脚本

`examples/production-demo/demo.py` 对线上实例演示所有网关特性，无需额外依赖（OpenAI SDK 可选）：

```bash
pip install openai
DASHSCOPE_API_KEY=sk-... LUNER_URL=http://localhost:8080 python examples/production-demo/demo.py
```

演示内容：健康检查 → 多 Agent 链路追踪 → LRU 缓存命中 → 限流 → 策略 CRUD + 拦截验证 → 实时指标快照 → 最近 Trace 列表 → 流式 SDK 调用。

---

## 📈 性能基准测试

测试环境：**Ubuntu 22.04 / 8 核 vCPU / 16 GB 内存**  
测试工具：`hey -c 50 -n 1000` | [复现脚本](scripts/bench.sh)

| 场景 | QPS | P50 | P99 | 缓存命中 | 内存 |
|---|---|---|---|---|---|
| 缓存命中（`temp=0`，重复请求） | **32 082** | **1.3 ms** | **6.9 ms** | 100% | ~42 MB |
| 冷启动（首次请求） | ~95 | ~380 ms | ~1.1 s | 0% | ~45 MB |
| 限流（`qps=10, burst=2`） | ~10 | ~45 ms | ~180 ms | — | ~43 MB |

> 缓存命中从内存 LRU 直接返回，无任何网络开销。  
> 结果因操作系统调度器和 Docker 运行时不同而有所差异，请使用 `scripts/bench.sh` 在自己的环境测试。

---

## 参与贡献

欢迎提交 PR、Issue 和反馈。请查阅 [CONTRIBUTING.md](CONTRIBUTING.md) 了解环境搭建指引、提交规范以及 `good first issue` 标签的入门任务。
