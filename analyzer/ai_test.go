package analyzer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Catker/chaoleme/config"
)

func TestAIAnalyzerReloadsAIConfigOnNextAnalyze(t *testing.T) {
	t.Parallel()

	var (
		mu        sync.Mutex
		models    []string
		authHeads []string
		streams   []bool
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解析请求失败: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()
		models = append(models, req.Model)
		authHeads = append(authHeads, r.Header.Get("Authorization"))
		streams = append(streams, req.Stream)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hot reload ok"}}]}`))
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	modTime := time.Now().Add(-2 * time.Minute)

	writeAIConfig(t, configPath, modTime, false, server.URL, "old-key", "old-model")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("加载初始配置失败: %v", err)
	}

	analyzer := NewAIAnalyzer(&cfg.AI, configPath)
	stats := &PeriodStats{}

	result, err := analyzer.Analyze(stats, "daily")
	if err != nil {
		t.Fatalf("首次分析失败: %v", err)
	}
	if result != "" {
		t.Fatalf("禁用 AI 时不应返回分析结果，实际为 %q", result)
	}

	writeAIConfig(t, configPath, modTime.Add(2*time.Minute), true, server.URL, "new-key", "new-model")

	result, err = analyzer.Analyze(stats, "daily")
	if err != nil {
		t.Fatalf("热重载后分析失败: %v", err)
	}
	if result != "hot reload ok" {
		t.Fatalf("热重载后返回结果不符合预期: %q", result)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(models) != 1 {
		t.Fatalf("期望命中 1 次 API，请求次数=%d", len(models))
	}
	if models[0] != "new-model" {
		t.Fatalf("期望使用热重载后的模型 new-model，实际为 %s", models[0])
	}
	if authHeads[0] != "Bearer new-key" {
		t.Fatalf("期望使用热重载后的 API Key，实际为 %s", authHeads[0])
	}
	if streams[0] {
		t.Fatalf("期望显式关闭流式响应，实际 stream=%t", streams[0])
	}
}

func TestAIAnalyzerKeepsPreviousConfigWhenReloadFails(t *testing.T) {
	t.Parallel()

	var (
		mu        sync.Mutex
		models    []string
		authHeads []string
		streams   []bool
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解析请求失败: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()
		models = append(models, req.Model)
		authHeads = append(authHeads, r.Header.Get("Authorization"))
		streams = append(streams, req.Stream)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"still old config"}}]}`))
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	modTime := time.Now().Add(-2 * time.Minute)

	writeAIConfig(t, configPath, modTime, true, server.URL, "stable-key", "stable-model")

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("加载初始配置失败: %v", err)
	}

	analyzer := NewAIAnalyzer(&cfg.AI, configPath)
	stats := &PeriodStats{}

	if _, err := analyzer.Analyze(stats, "daily"); err != nil {
		t.Fatalf("首次分析失败: %v", err)
	}

	writeInvalidAIConfig(t, configPath, modTime.Add(2*time.Minute), server.URL)

	result, err := analyzer.Analyze(stats, "daily")
	if err != nil {
		t.Fatalf("无效配置后的分析失败: %v", err)
	}
	if result != "still old config" {
		t.Fatalf("应继续使用旧配置，实际返回 %q", result)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(models) != 2 {
		t.Fatalf("期望命中 2 次 API，请求次数=%d", len(models))
	}
	if models[0] != "stable-model" || models[1] != "stable-model" {
		t.Fatalf("无效重载后模型不应变化，实际=%v", models)
	}
	if authHeads[0] != "Bearer stable-key" || authHeads[1] != "Bearer stable-key" {
		t.Fatalf("无效重载后 API Key 不应变化，实际=%v", authHeads)
	}
	if streams[0] || streams[1] {
		t.Fatalf("期望所有请求都显式关闭流式响应，实际=%v", streams)
	}
}

func writeAIConfig(t *testing.T, path string, modTime time.Time, enabled bool, apiURL, apiKey, model string) {
	t.Helper()

	content := []byte(`hostname: "test-host"
telegram:
  bot_token: "test-bot-token"
  chat_id: "test-chat-id"
report:
  daily: true
  daily_time: "09:00"
  weekly: true
  weekly_day: 0
  monthly: true
  monthly_day: 1
storage:
  db_path: "/tmp/chaoleme-test.db"
  retention_days: 30
collect:
  cpu_steal_interval: "5m"
  cpu_bench_interval: "30m"
  io_test_interval: "15m"
  io_test_size_mb: 4
ai:
  enabled: ` + boolToYAML(enabled) + `
  api_url: "` + apiURL + `"
  api_key: "` + apiKey + `"
  model: "` + model + `"
  daily: true
  weekly: true
  monthly: true
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("设置配置文件时间失败: %v", err)
	}
}

func writeInvalidAIConfig(t *testing.T, path string, modTime time.Time, apiURL string) {
	t.Helper()

	content := []byte(`hostname: "test-host"
telegram:
  bot_token: "test-bot-token"
  chat_id: "test-chat-id"
report:
  daily: true
  daily_time: "09:00"
  weekly: true
  weekly_day: 0
  monthly: true
  monthly_day: 1
storage:
  db_path: "/tmp/chaoleme-test.db"
  retention_days: 30
collect:
  cpu_steal_interval: "5m"
  cpu_bench_interval: "30m"
  io_test_interval: "15m"
  io_test_size_mb: 4
ai:
  enabled: true
  api_url: "` + apiURL + `"
  api_key: ""
  model: "broken-model"
  daily: true
  weekly: true
  monthly: true
`)

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("写入无效配置失败: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("设置无效配置时间失败: %v", err)
	}
}

func boolToYAML(v bool) string {
	if v {
		return "true"
	}

	return "false"
}
