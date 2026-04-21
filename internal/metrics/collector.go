package metrics

/*
	Prometheus 指标收集器
*/
import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 定义两个核心指标 (全局变量)
var (
	/*
		请求总数指标（Counter）
		计数器: 只增不减, 用来统计请求总量
		标签(维度):
			- model: 模型名 (qwen-turbo/deepseek)
			- provider: 厂商 (ali/openai/volc)
			- status: 状态码 (200/400/500/502)

		aigw_requests_total{model="qwen-turbo",provider="ali",status="200"} 123
	*/
	RequestTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aigw_requests_total",                    // 指标名称
		Help: "Total number of LLM requests processed", // 说明
	}, []string{"model", "provider", "status"}) // 标签维度

	/*
		请求耗时指标（Histogram）
			直方图: 统计请求耗时分布 (慢请求、快请求)
			标签:
				- model
				- provider

		aigw_request_duration_seconds_bucket{model="qwen-turbo",provider="ali",le="0.1"} 89
	*/
	RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aigw_request_duration_seconds",
		Help:    "Request latency in seconds",
		Buckets: prometheus.DefBuckets, // 默认耗时区间 [.005, .01, .025, .05, ... , 10]
	}, []string{"model", "provider"})

	TokensUsed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aigw_tokens_used_total",
		Help: "Total tokens consumed by model",
	}, []string{"model", "type"})
)

// 注册指标
func Init() {
	prometheus.MustRegister(RequestTotal, RequestDuration, TokensUsed)
}

// Handler 返回 Prometheus metrics HTTP handler
// 提供 /metrics 接口
// 创建一个HTTP处理器, 访问 /metrics 就能看到所有监控数据
func Handler() http.Handler {
	return promhttp.Handler()
}
