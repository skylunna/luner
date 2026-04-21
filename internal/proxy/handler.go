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

	"github.com/skylunna/luner/internal/cache"
	"github.com/skylunna/luner/internal/config"
	"github.com/skylunna/luner/internal/limiter"
	"github.com/skylunna/luner/internal/metrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

var tracer = otel.Tracer("ai-gateway-proxy")

type Handler struct {
	loader  *config.Loader
	client  *http.Client
	logger  *slog.Logger
	cache   *cache.LRU
	limiter *limiter.Manager
}

func NewHandler(loader *config.Loader, logger *slog.Logger, c *cache.LRU, lm *limiter.Manager) *Handler {
	return &Handler{
		loader:  loader,
		client:  &http.Client{},
		logger:  logger,
		cache:   c,
		limiter: lm,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
		h.writeError(w, "endpoint not found", http.StatusNotFound)
		return
	}

	// 1. 启动 OTel Span（自动提取 Traceparent）
	ctx, span := tracer.Start(r.Context(), "proxy.forward")
	defer span.End()
	span.SetAttributes(semconv.HTTPRequestMethodKey.String(r.Method))

	// 2. 读取请求体
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "read body failed")
		h.writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var reqPayload map[string]any
	if err := json.Unmarshal(bodyBytes, &reqPayload); err != nil {
		h.writeError(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	model, _ := reqPayload["model"].(string)
	stream, _ := reqPayload["stream"].(bool)
	temp, _ := reqPayload["temperature"].(float64)

	if model == "" {
		metrics.RequestTotal.WithLabelValues("unknown", "unknown", "400").Inc()
		h.writeError(w, "field 'model' is required", http.StatusBadRequest)
		return
	}

	span.SetAttributes(attribute.String("llm.model", model), attribute.Bool("llm.stream", stream))

	// 3. 路由匹配
	cfg := h.loader.Get()
	provider := h.resolveProvider(cfg, model)
	if provider == nil {
		metrics.RequestTotal.WithLabelValues(model, "unknown", "400").Inc()
		h.writeError(w, fmt.Sprintf("unsupported model: %s", model), http.StatusBadRequest)
		return
	}
	span.SetAttributes(attribute.String("llm.provider", provider.Name))

	// 限流检查
	if h.limiter != nil {
		bucket := h.limiter.GetBucket(provider.Name)
		if bucket != nil && !bucket.Allow() {
			metrics.RequestTotal.WithLabelValues(model, provider.Name, "429").Inc()
			h.writeError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
	}

	// 缓存检查 (仅非流式)
	if h.cache != nil && !stream {
		msgJSON, _ := json.Marshal(reqPayload["messages"])
		cacheKey := cache.GenerateKey(model, msgJSON, temp)
		if cached, ok := h.cache.Get(cacheKey); ok {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			metrics.RequestTotal.WithLabelValues(model, provider.Name, "200-cache").Inc()
			return
		}
	}

	// 4. 构造上游请求
	timeout := provider.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	upstreamURL := provider.BaseURL + r.URL.Path
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(bodyBytes))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "build request failed")
		h.writeError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// 注入 Trace 上下文
	upstreamReq.Header = r.Header.Clone()
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(upstreamReq.Header))
	upstreamReq.Header.Set("Accept-Encoding", "identity")
	upstreamReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	upstreamReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
	upstreamReq.ContentLength = int64(len(bodyBytes))

	// 5. 执行请求
	start := time.Now()
	resp, err := h.executeWithRetry(upstreamReq, 1)
	duration := time.Since(start).Seconds()

	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}
	metrics.RequestTotal.WithLabelValues(model, provider.Name, fmt.Sprintf("%d", statusCode)).Inc()
	metrics.RequestDuration.WithLabelValues(model, provider.Name).Observe(duration)
	span.SetAttributes(semconv.HTTPResponseStatusCodeKey.Int(statusCode))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "upstream failed")
		h.writeError(w, "upstream timeout or network error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 6. 响应透传 & Token 统计
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(resp.StatusCode)

	if !stream && h.cache != nil {
		respBody, err := io.ReadAll(resp.Body)
		if err == nil {
			_, _ = w.Write(respBody)
			// 缓存成功响应
			msgJSON, _ := json.Marshal(reqPayload["messages"])
			cacheKey := cache.GenerateKey(model, msgJSON, temp)
			h.cache.Set(cacheKey, respBody)
			span.SetAttributes(attribute.Bool("cache.set", true))
			h.parseAndRecordTokens(model, respBody)
		} else {
			span.RecordError(err)
		}
	} else {
		_, _ = io.Copy(w, resp.Body) // 流式直接管道，不解析 Token（v0.3.0 补充）
	}

	if statusCode >= 400 {
		span.SetStatus(codes.Error, fmt.Sprintf("http %d", statusCode))
	}
}

func (h *Handler) parseAndRecordTokens(model string, body []byte) {
	var resp struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}
	if resp.Usage.TotalTokens > 0 {
		metrics.TokensUsed.WithLabelValues(model, "prompt").Add(float64(resp.Usage.PromptTokens))
		metrics.TokensUsed.WithLabelValues(model, "completion").Add(float64(resp.Usage.CompletionTokens))
		metrics.TokensUsed.WithLabelValues(model, "total").Add(float64(resp.Usage.TotalTokens))
	}
}

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

		if resp.StatusCode < 500 {
			return resp, nil
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lastResp = resp
		lastErr = nil

		if attempt < maxRetries {
			h.logger.Debug("upstream returned 5xx, retrying", "attempt", attempt+1, "status", resp.StatusCode)
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
		}
	}

	if lastResp != nil {
		return lastResp, nil
	}
	return nil, lastErr
}

func (h *Handler) writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":{"message":"%s","type":"api_error"}}`, msg)
}
