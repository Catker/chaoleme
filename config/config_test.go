package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAIUsesDefaultModelAndURL(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`ai:
  enabled: true
  api_key: "test-key"
  daily: false
`)

	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	aiCfg, err := LoadAI(configPath)
	if err != nil {
		t.Fatalf("加载 AI 配置失败: %v", err)
	}

	defaultAI := DefaultConfig().AI
	if aiCfg.APIURL != defaultAI.APIURL {
		t.Fatalf("期望默认 API URL=%s，实际=%s", defaultAI.APIURL, aiCfg.APIURL)
	}
	if aiCfg.Model != defaultAI.Model {
		t.Fatalf("期望默认模型=%s，实际=%s", defaultAI.Model, aiCfg.Model)
	}
	if aiCfg.Daily {
		t.Fatalf("期望 daily=false，实际=%t", aiCfg.Daily)
	}
}

func TestDefaultRetentionSupportsMonthlyTrend(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.Storage.RetentionDays != minMonthlyRetentionDays {
		t.Fatalf("默认保留天数应支撑月报历史趋势: got=%d want=%d", cfg.Storage.RetentionDays, minMonthlyRetentionDays)
	}
}

func TestDefaultBurstSamplingConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.Collect.BurstInterval != "30s" || cfg.Collect.BurstDuration != "10m" {
		t.Fatalf("密集采样默认值不符合预期: %+v", cfg.Collect)
	}
	if cfg.GetBurstInterval().Seconds() != 30 || cfg.GetBurstDuration().Minutes() != 10 {
		t.Fatalf("密集采样时长解析不符合预期: interval=%s duration=%s", cfg.GetBurstInterval(), cfg.GetBurstDuration())
	}
}

func TestValidateRejectsRetentionTooShortForMonthlyTrend(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig()
	cfg.Storage.RetentionDays = 30
	cfg.Report.Monthly = true

	err := cfg.Validate()
	if err == nil {
		t.Fatal("月报开启且保留天数不足时应返回错误")
	}
	if !strings.Contains(err.Error(), "至少需要 90 天") {
		t.Fatalf("错误信息不符合预期: %v", err)
	}
}

func TestValidateRetentionMatchesEnabledReports(t *testing.T) {
	t.Parallel()

	cfg := validTestConfig()
	cfg.Report.Monthly = false
	cfg.Report.Weekly = true
	cfg.Report.Daily = true
	cfg.Storage.RetentionDays = minWeeklyRetentionDays

	if err := cfg.Validate(); err != nil {
		t.Fatalf("周报开启时保留 %d 天应通过: %v", minWeeklyRetentionDays, err)
	}
}

func TestValidateRejectsInvalidCollectValuesInFixedOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*Config)
		wantError string
	}{
		{
			name: "cpu steal interval zero",
			configure: func(cfg *Config) {
				cfg.Collect.CPUStealInterval = "0s"
			},
			wantError: "cpu_steal_interval 必须大于 0",
		},
		{
			name: "cpu bench interval negative",
			configure: func(cfg *Config) {
				cfg.Collect.CPUBenchInterval = "-1m"
			},
			wantError: "cpu_bench_interval 必须大于 0",
		},
		{
			name: "io test interval zero",
			configure: func(cfg *Config) {
				cfg.Collect.IOTestInterval = "0s"
			},
			wantError: "io_test_interval 必须大于 0",
		},
		{
			name: "burst interval zero",
			configure: func(cfg *Config) {
				cfg.Collect.BurstInterval = "0s"
			},
			wantError: "burst_interval 必须大于 0",
		},
		{
			name: "burst duration negative",
			configure: func(cfg *Config) {
				cfg.Collect.BurstDuration = "-1m"
			},
			wantError: "burst_duration 必须大于 0",
		},
		{
			name: "burst interval equal to duration",
			configure: func(cfg *Config) {
				cfg.Collect.BurstInterval = "10m"
				cfg.Collect.BurstDuration = "10m"
			},
			wantError: "burst_interval 必须小于 burst_duration",
		},
		{
			name: "io test size zero",
			configure: func(cfg *Config) {
				cfg.Collect.IOTestSizeMB = 0
			},
			wantError: "io_test_size_mb 必须大于 0",
		},
		{
			name: "first invalid interval wins",
			configure: func(cfg *Config) {
				cfg.Collect.CPUStealInterval = "0s"
				cfg.Collect.CPUBenchInterval = "0s"
			},
			wantError: "cpu_steal_interval 必须大于 0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig()
			tt.configure(cfg)

			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("错误不符合预期: got=%v want contains %q", err, tt.wantError)
			}
		})
	}
}

func TestLoadAIValidatesAPIKeyWhenEnabled(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`ai:
  enabled: true
  api_key: ""
`)

	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	_, err := LoadAI(configPath)
	if err == nil {
		t.Fatal("期望缺少 API Key 时返回错误")
	}
	if !strings.Contains(err.Error(), "ai.api_key 未配置") {
		t.Fatalf("错误信息不符合预期: %v", err)
	}
}

func validTestConfig() *Config {
	cfg := DefaultConfig()
	cfg.Telegram.BotToken = "test-token"
	cfg.Telegram.ChatID = "test-chat"
	return cfg
}
