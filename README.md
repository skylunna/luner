# ai-gateway

轻量级、云原生友好的 LLM API 代理网关。基于 Go 1.24 构建，支持 OpenAI 兼容协议、动态路由、指标采集与配置热更新。

## 🚀 Quick Start

```bash
git clone https://github.com/skylunna/ai-gateway.git
cd ai-gateway
cp config/config.example.yaml config/config.yaml
# 编辑 config/config.yaml 填入真实的 API Key
go run cmd/aigw/main.go -config config/config.yaml
```

### 测试请求：
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}'
```

## 📊 指标端点
- /metrics：Prometheus 格式指标（请求数、延迟、Token 消耗）
- /health：健康检查

## 🛠️ 开发
```bash
make test   # 运行测试（含 race detection）
make lint   # 静态检查
make build  # 编译二进制
```