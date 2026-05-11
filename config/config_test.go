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
