package reporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Catker/chaoleme/analyzer"
	"github.com/Catker/chaoleme/config"
)

// TelegramReporter Telegram 报告器
type TelegramReporter struct {
	botToken string
	chatID   string
	client   *http.Client
}

// NewTelegramReporter 创建 Telegram 报告器
func NewTelegramReporter(cfg *config.TelegramConfig) *TelegramReporter {
	return &TelegramReporter{
		botToken: cfg.BotToken,
		chatID:   cfg.ChatID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendReport 发送报告
func (r *TelegramReporter) SendReport(stats *analyzer.PeriodStats, aiAnalysis string) error {
	message := r.formatReport(stats, aiAnalysis)
	return r.sendMessage(message)
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

	buf.WriteString(title + "\n")
	buf.WriteString(fmt.Sprintf("📅 %s\n\n", stats.EndTime.Format("2006-01-02")))
	buf.WriteString("━━━━━━━━━━━━━━━━━━\n")

	// CPU Steal
	cpuRisk := stats.RiskDetails["cpu_steal"]
	buf.WriteString(fmt.Sprintf("🖥️ CPU 超售风险: %s\n", cpuRisk))
	buf.WriteString(fmt.Sprintf("   • Steal Time 平均: %.2f%%\n", stats.CPUStealAvg))
	buf.WriteString(fmt.Sprintf("   • Steal Time 峰值: %.2f%%\n", stats.CPUStealMax))
	buf.WriteString(fmt.Sprintf("   • 性能波动系数: %.3f\n\n", stats.CPUBenchCV))

	// CPU IOWait
	iowaitRisk := stats.RiskDetails["cpu_iowait"]
	buf.WriteString(fmt.Sprintf("⏳ CPU IOWait 风险: %s\n", iowaitRisk))
	buf.WriteString(fmt.Sprintf("   • IOWait 平均: %.2f%%\n", stats.CPUIoWaitAvg))
	buf.WriteString(fmt.Sprintf("   • IOWait 峰值: %.2f%%\n\n", stats.CPUIoWaitMax))

	// I/O
	ioRisk := stats.RiskDetails["io_latency"]
	buf.WriteString(fmt.Sprintf("💾 I/O 超售风险: %s\n", ioRisk))
	buf.WriteString(fmt.Sprintf("   • 写延迟 P95: %.2fms\n", stats.IOLatencyP95))
	buf.WriteString(fmt.Sprintf("   • 写延迟 P99: %.2fms\n", stats.IOLatencyP99))
	if stats.StorageType != "" {
		buf.WriteString(fmt.Sprintf("   • 存储类型: %s\n", stats.StorageType))
	}
	buf.WriteString("\n")

	// Memory
	memRisk := stats.RiskDetails["memory"]
	buf.WriteString(fmt.Sprintf("🧠 内存状态: %s\n", memRisk))
	buf.WriteString(fmt.Sprintf("   • 可用率: %.1f%%\n\n", stats.MemoryAvailablePercent))

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

	// AI 分析
	if aiAnalysis != "" {
		buf.WriteString("\n🤖 AI 分析:\n")
		buf.WriteString(aiAnalysis)
		buf.WriteString("\n")
	}

	buf.WriteString("━━━━━━━━━━━━━━━━━━\n")

	return buf.String()
}

// sendMessage 发送消息到 Telegram
func (r *TelegramReporter) sendMessage(text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", r.botToken)

	payload := map[string]interface{}{
		"chat_id":    r.chatID,
		"text":       text,
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
