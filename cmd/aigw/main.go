package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skylunna/ai-gateway/internal/cache"
	"github.com/skylunna/ai-gateway/internal/config"
	"github.com/skylunna/ai-gateway/internal/limiter"
	"github.com/skylunna/ai-gateway/internal/metrics"
	"github.com/skylunna/ai-gateway/internal/proxy"
	"github.com/skylunna/ai-gateway/internal/trace"
)

func main() {
	// 1. 解析启动参数
	configPath := flag.String("config", "config/config.yaml", "path to configuration file")
	flag.Parse()

	// 2. 初始化结构化日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// 3. 加载初始配置 & 创建热更新 Loader
	loader, err := config.NewLoader(*configPath)
	if err != nil {
		logger.Error("failed to load configuration", "err", err, "path", *configPath)
		os.Exit(1)
	}
	logger.Info("initial configuration loaded", "path", *configPath)

	// 4. 后台启动配置监听（文件修改后自动原子替换路由表）
	go func() {
		if err := loader.Watch(context.Background(), *configPath, logger); err != nil {
			logger.Error("config watcher exited unexpectedly", "err", err)
		}
	}()

	// 5. 初始化 Prometheus 指标收集器
	metrics.Init()

	otelShutdown, err := trace.InitTracer(context.Background(), logger)
	if err != nil {
		logger.Error("failed to init OpenTelemetry", "err", err)
		os.Exit(1)
	}
	cfg := loader.Get()
	var gwCache *cache.LRU
	if cfg.Cache.Enabled {
		gwCache = cache.NewLRU(cfg.Cache.MaxItems, cfg.Cache.TTL)
		logger.Info("LRU cache enabled", "capacity", cfg.Cache.MaxItems, "ttl", cfg.Cache.TTL)
	}

	// 初始化限流器
	gwLimiter := limiter.NewManager()
	if cfg.RateLimit.Enabled {
		for _, rl := range cfg.RateLimit.Providers {
			gwLimiter.SetBucket(rl.Name, limiter.NewBucket(float64(rl.Burst), rl.QPS))
		}
		logger.Info("rate limiter enabled", "providers", len(cfg.RateLimit.Providers))
	}

	// 6. 注册路由
	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", proxy.NewHandler(loader, logger, gwCache, gwLimiter))
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	// 7. 构建 HTTP Server（注意：网络层参数不支持热重载，需重启生效）

	server := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second, // 保持长连接空闲超时
	}

	// 8. 异步启动服务
	go func() {
		logger.Info("starting ai-gateway", "listen", server.Addr, "version", cfg.Version)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed", "err", err)
			os.Exit(1)
		}
	}()

	logger.Info("shutdown signal received, draining connections and flushing traces...")
	if otelShutdown != nil {
		_ = otelShutdown(context.Background()) // 等待 Trace 数据上报完成
	}

	// 9. 监听系统信号，准备优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutdown signal received, draining active connections...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 停止接收新请求，等待处理中的请求完成
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", "err", err)
	}
	logger.Info("server exited gracefully")
}
