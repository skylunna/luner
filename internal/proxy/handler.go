package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/skylunna/luner/internal/api"
	"github.com/skylunna/luner/internal/cache"
	"github.com/skylunna/luner/internal/config"
	"github.com/skylunna/luner/internal/limiter"
	"github.com/skylunna/luner/internal/metrics"
	"github.com/skylunna/luner/internal/policy"
	lunerTrace "github.com/skylunna/luner/internal/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("luner-proxy")

type Handler struct {
	loader       *config.Loader
	client       *http.Client
	logger       *slog.Logger
	cache        *cache.LRU
	limiter      *limiter.Manager
	collector    *lunerTrace.Collector
	policyEngine policy.Engine
}

func NewHandler(loader *config.Loader, logger *slog.Logger, c *cache.LRU, lm *limiter.Manager, collector *lunerTrace.Collector) *Handler {
	return &Handler{
		loader:    loader,
		client:    &http.Client{},
		logger:    logger,
		cache:     c,
		limiter:   lm,
		collector: collector,
	}
}

func (h *Handler) WithPolicyEngine(engine policy.Engine) *Handler {
	h.policyEngine = engine
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := h.extractRequestID(r)
	// 包装 ResponseWriter，保留 context
	rw := &responseWriter{ResponseWriter: w, ctx: r.Context()}

	if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
		h.writeAPIError(rw, api.NewError(http.StatusNotFound, api.ErrInvalidRequest, "endpoint not found", requestID))
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
		h.writeAPIError(rw, api.NewError(http.StatusBadRequest, api.ErrInvalidRequest, "invalid request body", requestID))
		return
	}
	defer r.Body.Close()

	var reqPayload map[string]any
	if err := json.Unmarshal(bodyBytes, &reqPayload); err != nil {
		h.writeAPIError(rw, api.NewError(http.StatusBadRequest, api.ErrInvalidRequest, "invalid JSON payload", requestID))
		return
	}

	model, _ := reqPayload["model"].(string)
	stream, _ := reqPayload["stream"].(bool)
	temp, _ := reqPayload["temperature"].(float64)

	if model == "" {
		metrics.RequestTotal.WithLabelValues("unknown", "unknown", "400").Inc()
		h.writeAPIError(rw, api.NewError(http.StatusBadRequest, api.ErrInvalidRequest, "field 'model' is required", requestID))
		return
	}

	span.SetAttributes(attribute.String("llm.model", model), attribute.Bool("llm.stream", stream))

	// 3. 路由匹配
	cfg := h.loader.Get()
	provider := h.resolveProvider(cfg, model)
	if provider == nil {
		metrics.RequestTotal.WithLabelValues(model, "unknown", "400").Inc()
		h.writeAPIError(rw, api.NewError(http.StatusBadRequest, api.ErrInvalidRequest, fmt.Sprintf("unsupported model: %s", model), requestID))
		return
	}
	span.SetAttributes(attribute.String("llm.provider", provider.Name))

	// 限流检查
	if h.limiter != nil {
		bucket := h.limiter.GetBucket(provider.Name)
		if bucket != nil && !bucket.Allow() {
			metrics.RequestTotal.WithLabelValues(model, provider.Name, "429").Inc()
			h.writeAPIError(rw, api.NewError(http.StatusTooManyRequests, api.ErrRateLimited, "rate limit exceeded", requestID))
			return
		}
	}

	// Policy evaluation
	if h.policyEngine != nil {
		evalCtx := &policy.EvaluationContext{
			Model:    model,
			UserID:   r.Header.Get("X-User-Id"),
			TenantID: r.Header.Get("X-Tenant-Id"),
		}
		result, err := h.policyEngine.Evaluate(ctx, evalCtx)
		if err != nil {
			h.logger.Warn("policy evaluation error, failing open", "err", err)
		} else if result != nil && result.Action == policy.ActionBlock {
			metrics.RequestTotal.WithLabelValues(model, provider.Name, "403").Inc()
			h.writeAPIError(rw, api.NewError(http.StatusForbidden, api.ErrInvalidRequest,
				"request blocked by policy: "+result.Message, requestID))
			return
		}
	}

	// 缓存检查 (仅非流式)
	if h.cache != nil && !stream {
		msgJSON, _ := json.Marshal(reqPayload["messages"])
		cacheKey := cache.GenerateKey(model, msgJSON, temp)
		if cached, ok := h.cache.Get(cacheKey); ok {
			span.SetAttributes(attribute.Bool("cache.hit", true))
			rw.Header().Set("Content-Type", "application/json; charset=utf-8")
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write(cached)
			metrics.RequestTotal.WithLabelValues(model, provider.Name, "200-cache").Inc()
			metrics.RequestDuration.WithLabelValues(model, provider.Name, "cache").Observe(time.Since(start).Seconds())

			// 记录 span：cache 命中仍需保留 agent/user 上下文，用合成 resp 表示 200
			h.collectSpan(r, &http.Response{StatusCode: http.StatusOK}, bodyBytes, cached, start)

			return
		}
	}
	// 流式请求自动注入 stream_options，确保上游返回 usage 供指标统计
	if stream {
		if _, ok := reqPayload["stream_options"]; !ok {
			reqPayload["stream_options"] = map[string]any{
				"include_usage": true,
			}
			// 重新序列化 body
			bodyBytes, err = json.Marshal(reqPayload)
			if err != nil {
				h.writeAPIError(rw, api.NewError(http.StatusBadRequest, api.ErrInvalidRequest, "invalid stream options", requestID))
				return
			}
		}
	}

	// 4. 构造上游请求
	timeout := provider.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	upstreamURL, err := url.JoinPath(provider.BaseURL, r.URL.Path)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "build upstream url failed")
		h.writeAPIError(rw, api.NewError(http.StatusInternalServerError, api.ErrInternal, "invalid upstream URL", requestID))
		return
	}
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(bodyBytes))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "build request failed")
		h.writeAPIError(rw, api.NewError(http.StatusInternalServerError, api.ErrInternal, "internal server error", requestID))
		return
	}

	// 注入 Trace 上下文
	upstreamReq.Header = r.Header.Clone()
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(upstreamReq.Header))
	upstreamReq.Header.Set("Accept-Encoding", "identity")
	upstreamReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	upstreamReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
	upstreamReq.ContentLength = int64(len(bodyBytes))

	h.logger.Debug("forwarding to upstream",
		"url", upstreamReq.URL.String(),
		"model", model,
		"provider", provider.Name)

	// 5. 执行请求
	// start := time.Now()
	resp, err := ExecuteWithRetry(h.client, upstreamReq, 1, h.logger)
	duration := time.Since(start).Seconds()

	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}
	metrics.RequestTotal.WithLabelValues(model, provider.Name, fmt.Sprintf("%d", statusCode)).Inc()
	metrics.RequestDuration.WithLabelValues(model, provider.Name, "upstream").Observe(duration)
	span.SetAttributes(semconv.HTTPResponseStatusCodeKey.Int(statusCode))

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "upstream failed")
		h.writeAPIError(rw, api.NewError(http.StatusBadGateway, api.ErrProviderDown, "upstream timeout or network error", requestID))
		return
	}
	defer resp.Body.Close()

	// 6. 响应透传 & Token 统计
	// Handle upstream errors before writing any headers so Content-Type can be set correctly.
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		h.collectSpan(r, resp, bodyBytes, body, start)
		h.writeAPIError(rw, api.FromUpstream(resp.StatusCode, body, requestID))
		return
	}

	rw.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	rw.WriteHeader(resp.StatusCode)

	if !stream && h.cache != nil {
		respBody, err := io.ReadAll(resp.Body)
		if err == nil {
			_, _ = rw.Write(respBody)
			// 缓存成功响应
			msgJSON, _ := json.Marshal(reqPayload["messages"])
			cacheKey := cache.GenerateKey(model, msgJSON, temp)
			h.cache.Set(cacheKey, respBody)
			span.SetAttributes(attribute.Bool("cache.set", true))
			h.parseAndRecordTokens(model, respBody)
			h.collectSpan(r, resp, bodyBytes, respBody, start)
		} else {
			span.RecordError(err)
		}
	} else if stream {
		// 流式解析 & 透传（提取 Token 指标）
		if err := h.streamUsageParser(rw, resp, model, provider.Name, requestID); err != nil {
			h.logger.Warn("stream parse/forward failed", "err", err, "request_id", requestID)
		}
		h.collectSpan(r, resp, bodyBytes, nil, start)
	} else {
		// 非流式但未启用缓存，直接透传
		respBody, _ := io.ReadAll(resp.Body)
		_, _ = rw.Write(respBody)
		h.collectSpan(r, resp, bodyBytes, respBody, start)
	}
}

// streamUsageParser 拦截 SSE 流，透传给客户端并提取 token usage
// flush=true 时强制每次 chunk 立即发送，保持低延迟体验
func (h *Handler) streamUsageParser(w http.ResponseWriter, resp *http.Response, model, provider, requestID string) error {
	flusher, canFlush := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)

	var usageFound atomic.Bool
	var totalPrompt, totalCompletion int64

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read stream chunk: %w", err)
		}

		// 1. 透传 chunk 给客户端
		if _, err := w.Write(line); err != nil {
			return fmt.Errorf("write to client: %w", err)
		}

		// 2. 强制刷新（保持流式低延迟）
		if canFlush {
			flusher.Flush()
		}

		// 3. 解析 data 行提取 usage
		lineStr := string(line)
		if strings.HasPrefix(strings.TrimSpace(lineStr), "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(lineStr, "data:"))
			if data == "[DONE]" {
				break
			}

			// 优化：仅当未找到 usage 时解析 JSON，降低 CPU 开销
			if !usageFound.Load() {
				var chunk struct {
					Usage struct {
						PromptTokens     int `json:"prompt_tokens"`
						CompletionTokens int `json:"completion_tokens"`
						TotalTokens      int `json:"total_tokens"`
					} `json:"usage"`
				}
				if err := json.Unmarshal([]byte(data), &chunk); err == nil && chunk.Usage.TotalTokens > 0 {
					totalPrompt = int64(chunk.Usage.PromptTokens)
					totalCompletion = int64(chunk.Usage.CompletionTokens)

					metrics.TokensUsed.WithLabelValues(model, "prompt").Add(float64(totalPrompt))
					metrics.TokensUsed.WithLabelValues(model, "completion").Add(float64(totalCompletion))
					metrics.TokensUsed.WithLabelValues(model, "total").Add(float64(chunk.Usage.TotalTokens))

					usageFound.Store(true)
					h.logger.Debug("stream usage extracted", "model", model, "provider", provider, "total_tokens", chunk.Usage.TotalTokens, "request_id", requestID)
				}
			}
		}
	}

	// 4. 若流结束仍未获取 usage（上游不支持或配置缺失），记录兜底指标
	if !usageFound.Load() {
		metrics.TokensUsed.WithLabelValues(model, "unknown").Add(1)
		h.logger.Debug("stream usage not found, recorded as unknown", "model", model, "provider", provider, "request_id", requestID)
	}
	return nil
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

func (h *Handler) collectSpan(req *http.Request, resp *http.Response, reqBody, respBody []byte, start time.Time) {
	_ = h.collector.CollectLLMSpan(req, resp, reqBody, respBody, start, time.Now())
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

// writeAPIError 统一错误入口（自动注入 TraceID）
func (h *Handler) writeAPIError(w http.ResponseWriter, err *api.APIError) {
	if err.Detail.RequestID == "" {
		if rw, ok := w.(*responseWriter); ok {
			if span := oteltrace.SpanFromContext(rw.ctx); span.SpanContext().HasTraceID() {
				err.Detail.RequestID = span.SpanContext().TraceID().String()
			}
		}
	}
	err.WriteJSON(w)
}

// extractRequestID 从请求头或 OTel Span 中提取 TraceID
func (h *Handler) extractRequestID(r *http.Request) string {
	if rid := r.Header.Get("X-Request-Id"); rid != "" {
		return rid
	}

	if span := oteltrace.SpanFromContext(r.Context()); span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}

	return fmt.Sprintf("luner-%d-%04x", time.Now().UnixNano(), rand.Int31n(0xFFFF))
}

type responseWriter struct {
	http.ResponseWriter
	ctx context.Context
}

func (rw *responseWriter) Context() context.Context {
	return rw.ctx
}
