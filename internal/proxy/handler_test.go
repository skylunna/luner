package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"
	"os"

	"github.com/skylunna/luner/internal/config"
	"github.com/skylunna/luner/internal/metrics"
)

func TestProxyHandler(t *testing.T) {
	// Mock LLM Server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "test-123",
			"choices": []map[string]any{{"message": map[string]string{"content": "pong"}}},
		})
	}))
	defer mockServer.Close()

	// Init Config
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{
				Name:    "mock-llm",
				BaseURL: mockServer.URL,
				APIKey:  "test-key",
				Models:  []string{"gpt-test"},
			},
		},
	}

	metrics.Init()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	handler := NewHandler(config.NewLoaderFromCfg(cfg), logger, nil, nil) // 需补充 NewLoaderFromCfg 用于测试

	tests := []struct {
		name       string
		payload    map[string]any
		wantStatus int
	}{
		{"valid request", map[string]any{"model": "gpt-test", "messages": []map[string]string{{"role": "user", "content": "ping"}}}, 200},
		{"missing model", map[string]any{"messages": []map[string]string{{"role": "user", "content": "ping"}}}, 400},
		{"unsupported model", map[string]any{"model": "unknown", "messages": []map[string]string{{"role": "user", "content": "ping"}}}, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("handler returned wrong status: got %v want %v", rr.Code, tt.wantStatus)
			}
		})
	}
}
