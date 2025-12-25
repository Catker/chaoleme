package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Catker/chaoleme/config"
)

// AIAnalyzer AI 分析器
type AIAnalyzer struct {
	client *http.Client
	config *config.AIConfig
}

// NewAIAnalyzer 创建 AI 分析器
func NewAIAnalyzer(cfg *config.AIConfig) *AIAnalyzer {
	return &AIAnalyzer{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		config: cfg,
	}
}

// Analyze 使用 AI 分析统计数据
func (a *AIAnalyzer) Analyze(stats *PeriodStats, reportType string) (string, error) {
	if !a.config.Enabled {
		return "", nil
	}

	// 检查是否启用该类型的 AI 评价
	switch reportType {
	case "daily":
		if !a.config.Daily {
			return "", nil
		}
	case "weekly":
		if !a.config.Weekly {
			return "", nil
		}
	case "monthly":
		if !a.config.Monthly {
			return "", nil
		}
	}

	prompt := a.buildPrompt(stats, reportType)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return a.callAPI(ctx, prompt)
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

	prompt := fmt.Sprintf(`你是一个 VPS 性能分析专家。请根据以下 %s 监控数据，评估该 VPS 是否存在超售问题，并给出简洁建议。

## 数据摘要
- CPU Steal Time: 平均 %.2f%%，最大 %.2f%%，P95 %.2f%%
- CPU 基准测试: 平均耗时 %.2fms，变异系数 %.3f
- I/O 写延迟: 平均 %.2fms，P95 %.2fms，P99 %.2fms
- 内存可用率: %.1f%%
- 存储类型: %s
- 规则评分: %.0f/100

请用中文回复，限制在 150 字以内。格式：
1. 一句话总结超售风险
2. 最值得关注的 1-2 个问题
3. 一条建议`,
		periodDesc,
		stats.CPUStealAvg, stats.CPUStealMax, stats.CPUStealP95,
		stats.CPUBenchAvg, stats.CPUBenchCV,
		stats.IOLatencyAvg, stats.IOLatencyP95, stats.IOLatencyP99,
		stats.MemoryAvailablePercent,
		storageType,
		stats.TotalScore,
	)

	// 周报/月报增加趋势分析提示
	if reportType == "weekly" {
		prompt += "\n\n请额外分析本周的性能趋势。"
	} else if reportType == "monthly" {
		prompt += "\n\n请额外分析长期趋势，并评估是否建议更换服务商。"
	}

	return prompt
}

// OpenAI API 请求/响应结构
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
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
func (a *AIAnalyzer) callAPI(ctx context.Context, prompt string) (string, error) {
	reqBody := chatRequest{
		Model: a.config.Model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.config.APIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.config.APIKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
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
