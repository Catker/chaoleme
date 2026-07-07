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
