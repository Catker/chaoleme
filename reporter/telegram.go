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

const (
	telegramSafeMessageLimit = 3900
	telegramErrorBodyLimit   = 8192
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
	message := r.FormatReport(stats, aiAnalysis)
	return r.sendMessageWithRetry(message, 3)
}

// FormatReport 格式化报告，可用于 Telegram 发送或本地预览。
func (r *TelegramReporter) FormatReport(stats *analyzer.PeriodStats, aiAnalysis string) string {
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
	buf.WriteString(fmt.Sprintf("🧭 超售判定: %s\n", stats.OversellVerdict.Label()))
	buf.WriteString(fmt.Sprintf("🔎 证据等级: %s\n", stats.EvidenceLevel.Label()))
	for _, item := range stats.EvidenceSummary {
		buf.WriteString(fmt.Sprintf("   • %s\n", item))
	}
	if len(stats.MissingMetrics) > 0 {
		buf.WriteString(fmt.Sprintf("   • 缺失指标: %s\n", strings.Join(stats.MissingMetrics, ", ")))
	}
	if len(stats.QueryErrors) > 0 {
		buf.WriteString(fmt.Sprintf("   • 查询错误: %s\n", strings.Join(stats.QueryErrors, " | ")))
	}
	buf.WriteString("\n")

	buf.WriteString("🧪 样本覆盖:\n")
	buf.WriteString(fmt.Sprintf("   • CPU Steal/IOWait 样本: %d/%d\n", stats.CPUStealSamples, stats.CPUIoWaitSamples))
	buf.WriteString(fmt.Sprintf("   • 核心覆盖: %.1f小时 / %.1f%%\n\n", stats.CoreSampleSpanHours, stats.CoreCoveragePercent))

	if stats.HostContextSamples > 0 {
		buf.WriteString("🧱 运行环境:\n")
		buf.WriteString(fmt.Sprintf("   • 虚拟化类型: %s\n", stats.VirtualizationType))
		buf.WriteString(fmt.Sprintf("   • Hypervisor: %t\n", stats.HypervisorDetected))
		buf.WriteString(fmt.Sprintf("   • 容器环境: %t\n", stats.ContainerDetected))
		buf.WriteString(fmt.Sprintf("   • Steal 可直接解释: %t\n\n", stats.StealDirectlyInterpretable))
	}

	// CPU Steal
	cpuRisk := stats.RiskDetails["cpu_steal"]
	buf.WriteString(fmt.Sprintf("🖥️ CPU 争抢证据: %s\n", cpuRisk))
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
	if stats.RandomIOSamples > 0 {
		buf.WriteString(fmt.Sprintf("   • O_DIRECT 有效样本: %d/%d\n", stats.RandomIODirectIOSamples, stats.RandomIOSamples))
	}
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
	buf.WriteString(fmt.Sprintf("   • 峰值 (归一化): %.2f\n", stats.CPULoadMax))
	if stats.CPUPressureSamples > 0 {
		buf.WriteString(fmt.Sprintf("   • CPU PSI some: 平均 %.2f%% / P95 %.2f%%\n", stats.CPUPressureSomeAvg, stats.CPUPressureSomeP95))
	}
	if stats.CPUThrottleSamples > 0 {
		buf.WriteString(fmt.Sprintf("   • CPU 限额节流: 平均 %.2f%% / P95 %.2f%%\n", stats.CPUThrottleAvg, stats.CPUThrottleP95))
	}
	if stats.IOPressureSamples > 0 {
		buf.WriteString(fmt.Sprintf("   • IO PSI some: 平均 %.2f%% / P95 %.2f%%\n", stats.IOPressureSomeAvg, stats.IOPressureSomeP95))
	}
	buf.WriteString("\n")

	// 历史趋势
	baselineRisk := stats.RiskDetails["baseline"]
	buf.WriteString(fmt.Sprintf("📈 历史趋势: %s\n", baselineRisk))
	if stats.BaselineDeviation > 0 {
		buf.WriteString(fmt.Sprintf("   • 偏离度: %.1f%%\n", stats.BaselineDeviation))
	}
	if stats.BaselineReason != "" {
		buf.WriteString(fmt.Sprintf("   • 依据: %s\n", stats.BaselineReason))
	}
	for _, item := range stats.BaselineMetrics {
		buf.WriteString(fmt.Sprintf("   • %s: 当前 %.2f / 历史中位 %.2f / 偏离 %.1f%% / %s\n",
			item.Name, item.Current, item.BaselineMedian, item.DeviationPercent, item.Status))
	}
	buf.WriteString("\n")

	buf.WriteString("━━━━━━━━━━━━━━━━━━\n")

	// 健康评分只表示资源状态，不直接等同于超售结论。
	buf.WriteString(fmt.Sprintf("📈 健康评分: %.0f/100\n", stats.TotalScore))

	var riskDesc string
	switch stats.RiskLevel {
	case analyzer.RiskLevelUnknown:
		riskDesc = "⚪ 数据不足"
	case analyzer.RiskLevelExcellent:
		riskDesc = "✅ 优秀"
	case analyzer.RiskLevelGood:
		riskDesc = "🟢 良好"
	case analyzer.RiskLevelMedium:
		riskDesc = "⚠️ 中等"
	case analyzer.RiskLevelSevere:
		riskDesc = "🔴 严重"
	default:
		riskDesc = "⚪ 数据不足"
	}
	buf.WriteString(fmt.Sprintf("📋 健康等级: %s\n", riskDesc))

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
	chunks := splitTelegramMessage(escapeHTML(text), telegramSafeMessageLimit)
	for i, chunk := range chunks {
		if err := r.sendMessageChunkWithRetry(chunk, maxRetries); err != nil {
			return fmt.Errorf("发送第 %d/%d 段失败: %w", i+1, len(chunks), err)
		}
	}
	return nil
}

func (r *TelegramReporter) sendMessageChunkWithRetry(escapedText string, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			// 指数退避：1s, 2s, 4s...
			wait := time.Duration(1<<uint(i-1)) * time.Second
			time.Sleep(wait)
		}
		if err := r.sendMessage(escapedText); err != nil {
			lastErr = err
			// 记录重试日志（内部不再 import log，通过返回错误传递）
			continue
		}
		return nil
	}
	return fmt.Errorf("发送失败（重试 %d 次）: %w", maxRetries, lastErr)
}

// sendMessage 发送消息到 Telegram
func (r *TelegramReporter) sendMessage(escapedText string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", r.botToken)

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
		return fmt.Errorf("发送消息失败: %s", r.sanitizeErrorText(err.Error()))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, telegramErrorBodyLimit))
		return fmt.Errorf("Telegram API 错误 (%d): %s", resp.StatusCode, r.sanitizeErrorText(string(body)))
	}

	return nil
}

// TestConnection 测试 Telegram 连接
func (r *TelegramReporter) TestConnection() error {
	return r.sendMessageWithRetry("✅ 超了么 (chaoleme) 已连接成功！", 1)
}

func (r *TelegramReporter) sanitizeErrorText(text string) string {
	if r.botToken == "" {
		return text
	}
	return strings.ReplaceAll(text, r.botToken, "<telegram-bot-token>")
}

func splitTelegramMessage(text string, limit int) []string {
	if limit <= 0 {
		return []string{text}
	}

	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(runes) > 0 {
		end := limit
		if len(runes) < end {
			end = len(runes)
		} else {
			for end > 0 && runes[end-1] != '\n' {
				end--
			}
			if end == 0 {
				end = limit
			}
		}

		chunk := strings.TrimSpace(string(runes[:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		runes = runes[end:]
	}

	return chunks
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
