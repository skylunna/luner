package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/skylunna/ai-gateway/internal/config"
	"github.com/skylunna/ai-gateway/internal/metrics"
)

// Handler 是 LLM 代理网关的核心 HTTP 处理器
type Handler struct {
	loader *config.Loader // 原子配置加载器（支持热更新）
	client *http.Client   // 基础 HTTP 客户端（超时由 context 控制）
	logger *slog.Logger   // 结构化日志
}

// NewHandler 创建代理处理器实例
func NewHandler(loader *config.Loader, logger *slog.Logger) *Handler {
	return &Handler{
		loader: loader,
		client: &http.Client{}, // 不使用 client.Timeout，改用 per-request context 精确控制
		logger: logger,
	}
}

// ServeHTTP 实现 http.Handler 接口
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. 路由守卫：仅处理 OpenAI 兼容端点
	if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
		h.writeError(w, "endpoint not found", http.StatusNotFound)
		return
	}

	// 2. 读取请求体（必须缓冲，用于后续解析 model 及重试）
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read request body", "err", err)
		h.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 3. 解析 JSON 提取 model
	var reqPayload map[string]any
	if err := json.Unmarshal(bodyBytes, &reqPayload); err != nil {
		h.writeError(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	model, _ := reqPayload["model"].(string)
	if model == "" {
		metrics.RequestTotal.WithLabelValues("unknown", "unknown", "400").Inc()
		h.writeError(w, "field 'model' is required", http.StatusBadRequest)
		return
	}

	// 4. 从最新配置中匹配 Provider（原子读取，支持热更新）
	cfg := h.loader.Get()
	provider := h.resolveProvider(cfg, model)
	if provider == nil {
		metrics.RequestTotal.WithLabelValues(model, "unknown", "400").Inc()
		h.writeError(w, fmt.Sprintf("unsupported model: %s", model), http.StatusBadRequest)
		return
	}

	// 5. 构造上游请求（带精确超时控制）
	timeout := provider.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	upstreamURL := provider.BaseURL + r.URL.Path
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(bodyBytes))
	if err != nil {
		h.logger.Error("failed to build upstream request", "err", err)
		h.writeError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// 克隆原始 Header 并注入认证
	upstreamReq.Header = r.Header.Clone()
	upstreamReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	upstreamReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
	upstreamReq.ContentLength = int64(len(bodyBytes))

	// 6. 执行请求（含重试）并记录指标
	start := time.Now()
	resp, err := h.executeWithRetry(upstreamReq, 1)
	duration := time.Since(start).Seconds()

	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}
	metrics.RequestTotal.WithLabelValues(model, provider.Name, fmt.Sprintf("%d", statusCode)).Inc()
	metrics.RequestDuration.WithLabelValues(model, provider.Name).Observe(duration)

	if err != nil {
		h.logger.Warn("upstream request failed after retries", "err", err)
		h.writeError(w, "upstream timeout or network error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 7. 透传响应（完美兼容流式 SSE / chunked 编码）
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// resolveProvider 根据 model 名称查找匹配的 Provider 配置
func (h *Handler) resolveProvider(cfg *config.Config, model string) *config.ProviderConfig {
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		for _, m := range p.Models {
			if m == model {
				return p
			}
		}
	}
	return nil
}

// executeWithRetry 对上游请求执行有限次重试（仅针对 5xx 或网络错误）
func (h *Handler) executeWithRetry(req *http.Request, maxRetries int) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := h.client.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				h.logger.Debug("retrying upstream request", "attempt", attempt+1, "err", err)
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
				continue
			}
			return nil, err
		}

		// 非 5xx 直接返回
		if resp.StatusCode < 500 {
			return resp, nil
		}

		// 5xx 错误：消费 body、关闭连接、准备重试
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lastResp = resp
		lastErr = nil

		if attempt < maxRetries {
			h.logger.Debug("upstream returned 5xx, retrying", "attempt", attempt+1, "status", resp.StatusCode)
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
		}
	}

	// 所有重试耗尽，返回最后一次 5xx 响应
	if lastResp != nil {
		return lastResp, nil
	}
	return nil, lastErr
}

// writeError 统一返回 JSON 格式错误
func (h *Handler) writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	// 符合 OpenAI API 错误格式规范
	fmt.Fprintf(w, `{"error":{"message":"%s","type":"api_error"}}`, msg)
}
