package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/skylunna/ai-gateway/internal/config"
	"github.com/skylunna/ai-gateway/internal/metrics"
)

type Handler struct {
	modelMap   map[string]*config.ProviderConfig
	httpClient *http.Client
	logger     *slog.Logger
}

func NewHandler(cfg *config.Config, logger *slog.Logger) *Handler {
	m := make(map[string]*config.ProviderConfig)
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		for _, mod := range p.Models {
			m[mod] = p
		}
	}

	return &Handler{
		modelMap:   m,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. 仅处理 OpenAI 兼容端点
	if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	// 2. 读取并校验 Body（必须缓冲，用于后续转发 & 提取 model）
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("read request body", "err", err)
		h.writeError(w, "invalid request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var reqPayload map[string]any
	if err := json.Unmarshal(bodyBytes, &reqPayload); err != nil {
		h.writeError(w, "invalid json", http.StatusBadRequest)
		return
	}

	model, _ := reqPayload["model"].(string)
	if model == "" {
		h.writeError(w, "model is required", http.StatusBadRequest)
		metrics.RequestTotal.WithLabelValues("unknown", "unknown", "400").Inc()
		return
	}

	// 3. 路由匹配
	prov, ok := h.modelMap[model]
	if !ok {
		h.writeError(w, fmt.Sprintf("unsupported model: %s", model), http.StatusBadRequest)
		metrics.RequestTotal.WithLabelValues(model, "unknown", "400").Inc()
		return
	}

	// 4. 构造上游请求（带超时控制）
	timeout := prov.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	targetURL := prov.BaseURL + r.URL.Path
	targetURL = strings.ReplaceAll(targetURL, "/v1/v1", "/v1")

	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		h.logger.Error("create upstream request", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// 复制原 Header 并注入 API Key
	upstreamReq.Header = r.Header.Clone()
	upstreamReq.Header.Set("Authorization", "Bearer "+prov.APIKey)
	upstreamReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
	upstreamReq.ContentLength = int64(len(bodyBytes))

	// 5. 发起请求 & 记录指标
	start := time.Now()
	resp, err := h.httpClient.Do(upstreamReq)
	duration := time.Since(start).Seconds()

	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}
	metrics.RequestTotal.WithLabelValues(model, prov.Name, fmt.Sprintf("%d", statusCode)).Inc()
	metrics.RequestDuration.WithLabelValues(model, prov.Name).Observe(duration)

	if err != nil {
		h.logger.Error("upstream request failed", "err", err)
		http.Error(w, `{"error":"upstream timeout or network error"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 6. 透传响应（完美支持流式 SSE）
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *Handler) writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":"%s"}`, msg)
}
