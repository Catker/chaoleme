package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Catker/chaoleme/config"
)

const maxAIResponseBytes = 1 << 20

// AIAnalyzer AI 分析器
type AIAnalyzer struct {
	client     *http.Client
	configPath string

	mu              sync.RWMutex
	config          config.AIConfig
	lastSeenModTime time.Time
}

// NewAIAnalyzer 创建 AI 分析器
func NewAIAnalyzer(cfg *config.AIConfig, configPath string) *AIAnalyzer {
	analyzer := &AIAnalyzer{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		configPath: configPath,
	}

	if cfg != nil {
		analyzer.config = *cfg
	}

	if configPath != "" {
		if info, err := os.Stat(configPath); err == nil {
			analyzer.lastSeenModTime = info.ModTime()
		}
	}

	return analyzer
}

// currentConfig 获取当前生效的 AI 配置快照
func (a *AIAnalyzer) currentConfig() config.AIConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.config
}

// reloadConfigIfChanged 在生成 AI 分析前按需热重载配置
func (a *AIAnalyzer) reloadConfigIfChanged() {
	if a.configPath == "" {
		return
	}

	info, err := os.Stat(a.configPath)
	if err != nil {
		log.Printf("检查 AI 配置文件变更失败，继续使用旧配置: %v", err)
		return
	}

	a.mu.Lock()
	if !info.ModTime().After(a.lastSeenModTime) {
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()

	cfg, err := config.LoadAI(a.configPath)
	if err != nil {
		log.Printf("AI 配置热重载失败，继续使用旧配置: %v", err)
		return
	}

	a.mu.Lock()
	a.config = *cfg
	a.lastSeenModTime = info.ModTime()
	a.mu.Unlock()

	log.Printf("AI 配置已热重载: enabled=%t, model=%s", cfg.Enabled, cfg.Model)
}

// Analyze 使用 AI 分析统计数据
func (a *AIAnalyzer) Analyze(stats *PeriodStats, reportType string) (string, error) {
	a.reloadConfigIfChanged()
	cfg := a.currentConfig()

	if !cfg.Enabled {
		return "", nil
	}

	// 检查是否启用该类型的 AI 评价
	switch reportType {
	case "daily":
		if !cfg.Daily {
			return "", nil
		}
	case "weekly":
		if !cfg.Weekly {
			return "", nil
		}
	case "monthly":
		if !cfg.Monthly {
			return "", nil
		}
	}

	prompt := a.buildPrompt(stats, reportType)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return a.callAPI(ctx, prompt, cfg)
}

// buildPrompt 构建 AI prompt
func (a *AIAnalyzer) buildPrompt(stats *PeriodStats, reportType string) string {
	var periodDesc string
	switch reportType {
	case "daily":
		periodDesc = "24 小时"
	case "weekly":
		periodDesc = "7 天"
	case "monthly":
		periodDesc = "30 天"
	}

	storageType := "未知"
	if stats.StorageType != "" {
		storageType = string(stats.StorageType)
	}
	wordLimit := 150
	if reportType == "weekly" || reportType == "monthly" {
		wordLimit = 260
	}

	// 格式化峰值时间（只显示时分）
	stealPeakTime := "N/A"
	if !stats.CPUStealMaxTime.IsZero() {
		stealPeakTime = stats.CPUStealMaxTime.Format("15:04")
	}
	iowaitPeakTime := "N/A"
	if !stats.CPUIoWaitMaxTime.IsZero() {
		iowaitPeakTime = stats.CPUIoWaitMaxTime.Format("15:04")
	}

	prompt := fmt.Sprintf(`你是一个 VPS 性能分析专家。请根据以下 %s 监控数据，基于证据等级评估 VPS 是否存在超售或资源争抢，并给出简洁建议。不要把缺失数据解释为正常。

## 规则判定
- 超售判定: %s
- 证据等级: %s
- 缺失指标: %s
- 查询错误: %s

## 运行环境
- 虚拟化类型: %s
- 检测到 Hypervisor: %t
- 容器环境: %t
- Steal 可直接解释: %t

## 数据摘要
- CPU Steal Time: 平均 %.2f%%，P95 %.2f%%，峰值 %.2f%% @ %s
- CPU IOWait: 平均 %.2f%%，P95 %.2f%%，峰值时间 %s
- CPU 基准测试: 平均耗时 %.2fms，变异系数 %.3f
- CPU Load (归一化): 平均 %.2f，最大 %.2f
- Linux PSI: CPU some 平均 %.2f%% / P95 %.2f%%，IO some 平均 %.2f%% / P95 %.2f%%
- cgroup CPU 限额节流: 平均 %.2f%% / P95 %.2f%%
- I/O 顺序写延迟: 平均 %.2fms，P95 %.2fms，P99 %.2fms
- I/O 随机延迟: 写 %.2fms，读 %.2fms，P95 %.2fms，O_DIRECT 有效样本 %d/%d
- 磁盘繁忙度: 平均 %.1f%%，P95 %.1f%%
- 内存可用率: %.1f%%
- 存储类型: %s
- 历史趋势: 偏离 %.1f%%，状态 %s，质量 %s，说明 %s
- 健康评分: %.0f/100

请用中文回复，限制在 %d 字以内。格式：
1. 一句话总结证据判定
2. 最值得关注的 1-2 个问题
3. 一条建议`,
		periodDesc,
		stats.OversellVerdict.Label(), stats.EvidenceLevel.Label(),
		strings.Join(stats.MissingMetrics, ", "), strings.Join(stats.QueryErrors, " | "),
		stats.VirtualizationType, stats.HypervisorDetected, stats.ContainerDetected, stats.StealDirectlyInterpretable,
		stats.CPUStealAvg, stats.CPUStealP95, stats.CPUStealMax, stealPeakTime,
		stats.CPUIoWaitAvg, stats.CPUIoWaitP95, iowaitPeakTime,
		stats.CPUBenchAvg, stats.CPUBenchCV,
		stats.CPULoadAvg, stats.CPULoadMax,
		stats.CPUPressureSomeAvg, stats.CPUPressureSomeP95, stats.IOPressureSomeAvg, stats.IOPressureSomeP95,
		stats.CPUThrottleAvg, stats.CPUThrottleP95,
		stats.IOLatencyAvg, stats.IOLatencyP95, stats.IOLatencyP99,
		stats.RandomIOWriteAvg, stats.RandomIOReadAvg, stats.RandomIOP95, stats.RandomIODirectIOSamples, stats.RandomIOSamples,
		stats.DiskBusyPercent, stats.DiskBusyP95,
		stats.MemoryAvailablePercent,
		storageType,
		stats.BaselineDeviation, stats.BaselineStatus, stats.BaselineQuality.Label(), stats.BaselineReason,
		stats.TotalScore,
		wordLimit,
	)

	// 周报/月报增加趋势分析提示
	if reportType == "weekly" {
		prompt += "\n\n请额外分析本周的性能趋势。"
	} else if reportType == "monthly" {
		prompt += "\n\n请额外分析长期趋势，并说明是否需要迁移前继续观察或压测验证。"
	}

	return prompt
}

// OpenAI API 请求/响应结构
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// callAPI 调用 OpenAI 兼容 API
func (a *AIAnalyzer) callAPI(ctx context.Context, prompt string, cfg config.AIConfig) (string, error) {
	reqBody := chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.APIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAIResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	if len(body) > maxAIResponseBytes {
		return "", fmt.Errorf("API 响应过大，超过 %d 字节", maxAIResponseBytes)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("API HTTP 状态异常 (%d): %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("API 错误: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("API 返回空响应")
	}

	return chatResp.Choices[0].Message.Content, nil
}
