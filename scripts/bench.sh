#!/usr/bin/env bash
set -e

# 基础配置
BASE_URL="http://localhost:8080"
TARGET="$BASE_URL/v1/chat/completions"
PAYLOAD='{"model":"qwen-turbo","messages":[{"role":"user","content":"Hello"}]}'

# 1. 预检：确保服务在线（使用绝对路径）
echo "🔍 检查服务健康..."
if ! curl -sf "$BASE_URL/health" > /dev/null 2>&1; then
  echo "❌ luner not running at $BASE_URL"
  echo "💡 请先在另一个终端执行: go run cmd/luner/main.go -config config/config.yaml"
  exit 1
fi
echo "✅ 服务在线"

# 2. 单请求预热（触发缓存 + 验证链路）
echo "🔥 预热请求..."
if ! curl -sf -X POST -H "Content-Type: application/json" -d "$PAYLOAD" "$TARGET" > /dev/null 2>&1; then
  echo "⚠️  预热请求失败，可能是 API Key 无效或网络问题，继续压测..."
fi

# 3. 正式压测
echo "🚀 开始压测 (hey -n 1000 -c 50)..."
hey -n 1000 -c 50 \
  -m POST \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  -T 30s \
  "$TARGET"

# 4. 缓存 & 限流指标
echo -e "\n📈 关键指标统计:"
echo "── 请求状态分布 ──"
curl -s "$BASE_URL/metrics" | grep luner_requests_total | grep -v "^#" | head -10
echo -e "\n── 缓存命中情况 ──"
curl -s "$BASE_URL/metrics" | grep "200-cache" || echo "（暂无缓存命中，尝试重复相同请求）"
echo -e "\n── 限流拦截情况 ──"
curl -s "$BASE_URL/metrics" | grep "429" || echo "（未触发限流）"