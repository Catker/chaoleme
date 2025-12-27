package reporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Catker/chaoleme/analyzer"
	"github.com/Catker/chaoleme/config"
)

// TelegramReporter Telegram 报告器
type TelegramReporter struct {
	botToken string
	chatID   string
	hostname string
	client   *http.Client
}

// NewTelegramReporter 创建 Telegram 报告器
func NewTelegramReporter(cfg *config.TelegramConfig, hostname string) *TelegramReporter {
	return &TelegramReporter{
		botToken: cfg.BotToken,
		chatID:   cfg.ChatID,
		hostname: hostname,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendReport 发送报告
func (r *TelegramReporter) SendReport(stats *analyzer.PeriodStats, aiAnalysis string) error {
	message := r.formatReport(stats, aiAnalysis)
	return r.sendMessageWithRetry(message, 3)
}

// formatReport 格式化报告
func (r *TelegramReporter) formatReport(stats *analyzer.PeriodStats, aiAnalysis string) string {
	var buf bytes.Buffer

	// 标题
	var title string
	switch stats.Period {
	case "daily":
		title = "📊 超了么日报"
	case "weekly":
		title = "📊 超了么周报"
	case "monthly":
		title = "📊 超了么月报"
	default:
		title = "📊 超了么报告"
	}

	// 添加主机标识
	buf.WriteString(fmt.Sprintf("%s | 🖥️ %s\n", title, r.hostname))
	buf.WriteString(fmt.Sprintf("📅 %s\n\n", stats.EndTime.Format("2006-01-02")))
	buf.WriteString("━━━━━━━━━━━━━━━━━━\n")

	// CPU Steal
	cpuRisk := stats.RiskDetails["cpu_steal"]
	buf.WriteString(fmt.Sprintf("🖥️ CPU 超售风险: %s\n", cpuRisk))
	buf.WriteString(fmt.Sprintf("   • Steal Time 平均: %.2f%%\n", stats.CPUStealAvg))
	buf.WriteString(fmt.Sprintf("   • Steal Time 峰值: %.2f%%\n", stats.CPUStealMax))
	if !stats.CPUStealMaxTime.IsZero() {
		buf.WriteString(fmt.Sprintf("   • 峰值时段: %s\n", formatHourRange(stats.CPUStealMaxTime)))
	}
	buf.WriteString(fmt.Sprintf("   • 性能波动系数: %.3f\n\n", stats.CPUBenchCV))

	// CPU IOWait
	iowaitRisk := stats.RiskDetails["cpu_iowait"]
	buf.WriteString(fmt.Sprintf("⏳ CPU IOWait 风险: %s\n", iowaitRisk))
	buf.WriteString(fmt.Sprintf("   • IOWait 平均: %.2f%%\n", stats.CPUIoWaitAvg))
	buf.WriteString(fmt.Sprintf("   • IOWait 峰值: %.2f%%\n", stats.CPUIoWaitMax))
	if !stats.CPUIoWaitMaxTime.IsZero() {
		buf.WriteString(fmt.Sprintf("   • 峰值时段: %s\n", formatHourRange(stats.CPUIoWaitMaxTime)))
	}
	buf.WriteString("\n")

	// I/O 顺序写
	ioRisk := stats.RiskDetails["io_latency"]
	buf.WriteString(fmt.Sprintf("💾 顺序写延迟: %s\n", ioRisk))
	buf.WriteString(fmt.Sprintf("   • P95: %.2fms\n", stats.IOLatencyP95))
	buf.WriteString(fmt.Sprintf("   • P99: %.2fms\n", stats.IOLatencyP99))
	if stats.StorageType != "" {
		buf.WriteString(fmt.Sprintf("   • 存储类型: %s\n", stats.StorageType))
	}
	buf.WriteString("\n")

	// I/O 随机读写
	randomIORisk := stats.RiskDetails["random_io"]
	buf.WriteString(fmt.Sprintf("🎲 随机 I/O: %s\n", randomIORisk))
	buf.WriteString(fmt.Sprintf("   • 写延迟: %.2fms\n", stats.RandomIOWriteAvg))
	buf.WriteString(fmt.Sprintf("   • 读延迟: %.2fms\n", stats.RandomIOReadAvg))
	buf.WriteString("\n")

	// 磁盘繁忙度
	diskBusyRisk := stats.RiskDetails["disk_busy"]
	buf.WriteString(fmt.Sprintf("📀 磁盘繁忙度: %s\n", diskBusyRisk))
	if stats.DiskBusyP95 > 0 {
		buf.WriteString(fmt.Sprintf("   • P95: %.1f%%\n", stats.DiskBusyP95))
	}
	buf.WriteString("\n")

	// Memory
	memRisk := stats.RiskDetails["memory"]
	buf.WriteString(fmt.Sprintf("🧠 内存状态: %s\n", memRisk))
	buf.WriteString(fmt.Sprintf("   • 可用率: %.1f%%\n\n", stats.MemoryAvailablePercent))

	// CPU Load
	loadRisk := stats.RiskDetails["cpu_load"]
	buf.WriteString(fmt.Sprintf("📊 CPU 负载: %s\n", loadRisk))
	buf.WriteString(fmt.Sprintf("   • Load1 (归一化): %.2f\n", stats.CPULoadAvg))
	buf.WriteString(fmt.Sprintf("   • 峰值 (归一化): %.2f\n\n", stats.CPULoadMax))

	// Baseline
	baselineRisk := stats.RiskDetails["baseline"]
	buf.WriteString(fmt.Sprintf("📈 基线对比: %s\n", baselineRisk))
	if stats.BaselineDeviation > 0 {
		buf.WriteString(fmt.Sprintf("   • 偏离度: %.1f%%\n", stats.BaselineDeviation))
	}
	buf.WriteString("\n")

	buf.WriteString("━━━━━━━━━━━━━━━━━━\n")

	// 综合评分
	buf.WriteString(fmt.Sprintf("📈 综合评分: %.0f/100\n", stats.TotalScore))

	// 风险等级描述
	var riskDesc string
	switch stats.RiskLevel {
	case analyzer.RiskLevelExcellent:
		riskDesc = "✅ 优秀，无超售迹象"
	case analyzer.RiskLevelGood:
		riskDesc = "🟢 良好，轻微资源竞争"
	case analyzer.RiskLevelMedium:
		riskDesc = "⚠️ 中等，存在超售可能"
	case analyzer.RiskLevelSevere:
		riskDesc = "🔴 严重超售，建议更换"
	}
	buf.WriteString(fmt.Sprintf("📋 风险等级: %s\n", riskDesc))

	// 时段分析摘要（仅周报/月报显示）
	if (stats.Period == "weekly" || stats.Period == "monthly") && len(stats.HourlyBreakdown) > 0 {
		buf.WriteString("\n📊 时段分析:\n")
		highHours, lowHours := findHighLowLoadHours(stats.HourlyBreakdown)
		if len(highHours) > 0 {
			buf.WriteString(fmt.Sprintf("   • 高负载时段: %s\n", formatHoursList(highHours)))
		}
		if len(lowHours) > 0 {
			buf.WriteString(fmt.Sprintf("   • 低负载时段: %s\n", formatHoursList(lowHours)))
		}
	}

	// AI 分析
	if aiAnalysis != "" {
		buf.WriteString("\n🤖 AI 分析:\n")
		buf.WriteString(aiAnalysis)
		buf.WriteString("\n")
	}

	buf.WriteString("━━━━━━━━━━━━━━━━━━\n")

	return buf.String()
}

// escapeHTML 转义 HTML 特殊字符，避免被 Telegram 解析为 HTML 标签
func escapeHTML(text string) string {
	// 按顺序替换：先 &，再 < 和 >
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

// sendMessageWithRetry 发送消息到 Telegram（带重试机制）
func (r *TelegramReporter) sendMessageWithRetry(text string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			// 指数退避：1s, 2s, 4s...
			wait := time.Duration(1<<uint(i-1)) * time.Second
			time.Sleep(wait)
		}
		if err := r.sendMessage(text); err != nil {
			lastErr = err
			// 记录重试日志（内部不再 import log，通过返回错误传递）
			continue
		}
		return nil
	}
	return fmt.Errorf("发送失败（重试 %d 次）: %w", maxRetries, lastErr)
}

// sendMessage 发送消息到 Telegram
func (r *TelegramReporter) sendMessage(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", r.botToken)

	// 转义 HTML 特殊字符
	escapedText := escapeHTML(text)

	payload := map[string]interface{}{
		"chat_id":    r.chatID,
		"text":       escapedText,
		"parse_mode": "HTML",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	resp, err := r.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Telegram API 错误 (%d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// TestConnection 测试 Telegram 连接
func (r *TelegramReporter) TestConnection() error {
	return r.sendMessage("✅ 超了么 (chaoleme) 已连接成功！")
}

// formatHourRange 格式化单个时间点为小时范围（如 14:00-15:00）
func formatHourRange(t time.Time) string {
	hour := t.Hour()
	return fmt.Sprintf("%02d:00-%02d:00", hour, (hour+1)%24)
}

// findHighLowLoadHours 从小时级统计中找出高负载和低负载时段
// 返回高负载时段（Top 3 by steal+iowait 平均）和低负载时段（Bottom 3）
func findHighLowLoadHours(hourly []analyzer.HourlyStats) (high, low []analyzer.HourlyStats) {
	if len(hourly) == 0 {
		return nil, nil
	}

	// 复制并按负载排序（steal + iowait 平均值）
	sorted := make([]analyzer.HourlyStats, len(hourly))
	copy(sorted, hourly)

	sort.Slice(sorted, func(i, j int) bool {
		loadI := sorted[i].CPUStealAvg + sorted[i].CPUIoWaitAvg
		loadJ := sorted[j].CPUStealAvg + sorted[j].CPUIoWaitAvg
		return loadI > loadJ // 降序
	})

	// 取 Top 3 高负载（仅当负载 > 1%）
	for i := 0; i < len(sorted) && i < 3; i++ {
		if sorted[i].CPUStealAvg+sorted[i].CPUIoWaitAvg > 1.0 {
			high = append(high, sorted[i])
		}
	}

	// 取 Bottom 3 低负载（仅当有足够数据）
	if len(sorted) >= 6 {
		for i := len(sorted) - 1; i >= len(sorted)-3 && i >= 0; i-- {
			low = append(low, sorted[i])
		}
	}

	return high, low
}

// formatHoursList 格式化多个小时统计为可读字符串
func formatHoursList(hours []analyzer.HourlyStats) string {
	if len(hours) == 0 {
		return "-"
	}

	var parts []string
	for _, h := range hours {
		parts = append(parts, fmt.Sprintf("%02d:00 (S:%.1f%% W:%.1f%%)",
			h.Hour, h.CPUStealAvg, h.CPUIoWaitAvg))
	}

	return strings.Join(parts, ", ")
}
