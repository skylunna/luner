package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	RequestTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aigw_requests_total",
		Help: "Total number of LLM requests processed",
	}, []string{"model", "provider", "status"})

	RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aigw_request_duration_seconds",
		Help:    "Request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"model", "provider"})
)

func Init() {
	prometheus.MustRegister(RequestTotal, RequestDuration)
}

// Handler 返回 Prometheus metrics HTTP handler
func Handler() http.Handler {
	return promhttp.Handler()
}
