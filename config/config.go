package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 主配置结构
type Config struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Report   ReportConfig   `yaml:"report"`
	Storage  StorageConfig  `yaml:"storage"`
	Collect  CollectConfig  `yaml:"collect"`
	AI       AIConfig       `yaml:"ai"`
}

// TelegramConfig Telegram 通知配置
type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

// ReportConfig 报告配置
type ReportConfig struct {
	Daily      bool   `yaml:"daily"`
	DailyTime  string `yaml:"daily_time"` // 格式: "09:00"
	Weekly     bool   `yaml:"weekly"`
	WeeklyDay  int    `yaml:"weekly_day"` // 0=周日, 1=周一, ...
	Monthly    bool   `yaml:"monthly"`
	MonthlyDay int    `yaml:"monthly_day"` // 1-28
}

// StorageConfig 存储配置
type StorageConfig struct {
	DBPath        string `yaml:"db_path"`
	RetentionDays int    `yaml:"retention_days"`
}

// CollectConfig 采集配置
type CollectConfig struct {
	CPUStealInterval string `yaml:"cpu_steal_interval"`
	CPUBenchInterval string `yaml:"cpu_bench_interval"`
	IOTestInterval   string `yaml:"io_test_interval"`
	IOTestSizeMB     int    `yaml:"io_test_size_mb"`
}

// AIConfig AI 分析配置
type AIConfig struct {
	Enabled bool   `yaml:"enabled"`
	APIURL  string `yaml:"api_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
	Daily   bool   `yaml:"daily"`
	Weekly  bool   `yaml:"weekly"`
	Monthly bool   `yaml:"monthly"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Report: ReportConfig{
			Daily:      true,
			DailyTime:  "09:00",
			Weekly:     true,
			WeeklyDay:  0,
			Monthly:    true,
			MonthlyDay: 1,
		},
		Storage: StorageConfig{
			DBPath:        "/var/lib/chaoleme/data.db",
			RetentionDays: 30,
		},
		Collect: CollectConfig{
			CPUStealInterval: "5m",
			CPUBenchInterval: "30m",
			IOTestInterval:   "15m",
			IOTestSizeMB:     4,
		},
		AI: AIConfig{
			Enabled: false,
			APIURL:  "https://api.openai.com/v1/chat/completions",
			Model:   "gpt-4o-mini",
			Daily:   true,
			Weekly:  true,
			Monthly: true,
		},
	}
}

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return cfg, nil
}

// Validate 验证配置有效性
func (c *Config) Validate() error {
	if c.Telegram.BotToken == "" || c.Telegram.BotToken == "YOUR_BOT_TOKEN" {
		return fmt.Errorf("telegram.bot_token 未配置")
	}
	if c.Telegram.ChatID == "" || c.Telegram.ChatID == "YOUR_CHAT_ID" {
		return fmt.Errorf("telegram.chat_id 未配置")
	}

	// 验证时间间隔格式
	intervals := map[string]string{
		"cpu_steal_interval": c.Collect.CPUStealInterval,
		"cpu_bench_interval": c.Collect.CPUBenchInterval,
		"io_test_interval":   c.Collect.IOTestInterval,
	}
	for name, interval := range intervals {
		if _, err := time.ParseDuration(interval); err != nil {
			return fmt.Errorf("%s 格式无效: %s", name, interval)
		}
	}

	// 验证日报时间格式
	if c.Report.Daily {
		if _, err := time.Parse("15:04", c.Report.DailyTime); err != nil {
			return fmt.Errorf("daily_time 格式无效，应为 HH:MM: %s", c.Report.DailyTime)
		}
	}

	// 验证 AI 配置
	if c.AI.Enabled {
		if c.AI.APIKey == "" || c.AI.APIKey == "YOUR_API_KEY" {
			return fmt.Errorf("ai.api_key 未配置")
		}
	}

	return nil
}

// GetCPUStealInterval 获取 CPU steal 采集间隔
func (c *Config) GetCPUStealInterval() time.Duration {
	d, _ := time.ParseDuration(c.Collect.CPUStealInterval)
	return d
}

// GetCPUBenchInterval 获取 CPU 基准测试间隔
func (c *Config) GetCPUBenchInterval() time.Duration {
	d, _ := time.ParseDuration(c.Collect.CPUBenchInterval)
	return d
}

// GetIOTestInterval 获取 I/O 测试间隔
func (c *Config) GetIOTestInterval() time.Duration {
	d, _ := time.ParseDuration(c.Collect.IOTestInterval)
	return d
}
