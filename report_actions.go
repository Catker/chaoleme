package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Catker/chaoleme/analyzer"
	"github.com/Catker/chaoleme/collector"
	"github.com/Catker/chaoleme/reporter"
)

func reportRange(reportType string, now time.Time) (time.Time, time.Time, error) {
	switch reportType {
	case "daily":
		return now.AddDate(0, 0, -1), now, nil
	case "weekly":
		return now.AddDate(0, 0, -7), now, nil
	case "monthly":
		return now.AddDate(0, -1, 0), now, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("无效的报告类型: %s", reportType)
	}
}

type reportJSONOutput struct {
	Period              string                         `json:"period"`
	StartTime           time.Time                      `json:"start_time"`
	EndTime             time.Time                      `json:"end_time"`
	OversellVerdict     analyzer.OversellVerdict       `json:"oversell_verdict"`
	EvidenceLevel       analyzer.EvidenceLevel         `json:"evidence_level"`
	EvidenceSummary     []string                       `json:"evidence_summary"`
	MissingMetrics      []string                       `json:"missing_metrics"`
	QueryErrors         []string                       `json:"query_errors"`
	CoreSampleSpanHours float64                        `json:"core_sample_span_hours"`
	CoreCoveragePercent float64                        `json:"core_coverage_percent"`
	CPUStealSamples     int                            `json:"cpu_steal_samples"`
	CPUIoWaitSamples    int                            `json:"cpu_iowait_samples"`
	CPUBenchSamples     int                            `json:"cpu_bench_samples"`
	IOLatencySamples    int                            `json:"io_latency_samples"`
	RandomIOSamples     int                            `json:"random_io_samples"`
	DiskStatsSamples    int                            `json:"disk_stats_samples"`
	MemorySamples       int                            `json:"memory_samples"`
	CPULoadSamples      int                            `json:"cpu_load_samples"`
	CPUPressureSamples  int                            `json:"cpu_pressure_samples"`
	IOPressureSamples   int                            `json:"io_pressure_samples"`
	CPUThrottleSamples  int                            `json:"cpu_throttle_samples"`
	HostContextSamples  int                            `json:"host_context_samples"`
	CPUStealAvg         float64                        `json:"cpu_steal_avg"`
	CPUStealMax         float64                        `json:"cpu_steal_max"`
	CPUStealP95         float64                        `json:"cpu_steal_p95"`
	CPUIoWaitAvg        float64                        `json:"cpu_iowait_avg"`
	CPUIoWaitMax        float64                        `json:"cpu_iowait_max"`
	CPUIoWaitP95        float64                        `json:"cpu_iowait_p95"`
	CPUBenchAvg         float64                        `json:"cpu_bench_avg"`
	CPUBenchCV          float64                        `json:"cpu_bench_cv"`
	IOLatencyAvg        float64                        `json:"io_latency_avg"`
	IOLatencyP95        float64                        `json:"io_latency_p95"`
	IOLatencyP99        float64                        `json:"io_latency_p99"`
	RandomIOWriteAvg    float64                        `json:"random_io_write_avg"`
	RandomIOReadAvg     float64                        `json:"random_io_read_avg"`
	RandomIOP95         float64                        `json:"random_io_p95"`
	RandomIODirectIO    int                            `json:"random_io_direct_samples"`
	CPULoadAvg          float64                        `json:"cpu_load_avg"`
	CPULoadMax          float64                        `json:"cpu_load_max"`
	CPUPressureSomeAvg  float64                        `json:"cpu_pressure_some_avg"`
	CPUPressureSomeP95  float64                        `json:"cpu_pressure_some_p95"`
	IOPressureSomeAvg   float64                        `json:"io_pressure_some_avg"`
	IOPressureSomeP95   float64                        `json:"io_pressure_some_p95"`
	CPUThrottleAvg      float64                        `json:"cpu_throttle_avg"`
	CPUThrottleP95      float64                        `json:"cpu_throttle_p95"`
	DiskBusyAvg         float64                        `json:"disk_busy_avg"`
	DiskBusyP95         float64                        `json:"disk_busy_p95"`
	MemoryAvailablePct  float64                        `json:"memory_available_percent"`
	BaselineDeviation   float64                        `json:"baseline_deviation"`
	BaselineStatus      string                         `json:"baseline_status"`
	BaselineMinDays     int                            `json:"baseline_min_days"`
	BaselineQuality     analyzer.BaselineQuality       `json:"baseline_quality"`
	BaselineReason      string                         `json:"baseline_reason"`
	BaselineMetrics     []analyzer.BaselineMetricTrend `json:"baseline_metrics"`
	StorageType         collector.StorageType          `json:"storage_type"`
	TotalScore          float64                        `json:"health_score"`
	RiskLevel           analyzer.RiskLevel             `json:"health_level"`
	VirtualizationType  string                         `json:"virtualization_type"`
	HypervisorDetected  bool                           `json:"hypervisor_detected"`
	ContainerDetected   bool                           `json:"container_detected"`
	StealDirect         bool                           `json:"steal_directly_interpretable"`
}

func buildReportJSON(stats *analyzer.PeriodStats) ([]byte, error) {
	return json.MarshalIndent(reportJSONOutput{
		Period:              stats.Period,
		StartTime:           stats.StartTime,
		EndTime:             stats.EndTime,
		OversellVerdict:     stats.OversellVerdict,
		EvidenceLevel:       stats.EvidenceLevel,
		EvidenceSummary:     stats.EvidenceSummary,
		MissingMetrics:      stats.MissingMetrics,
		QueryErrors:         stats.QueryErrors,
		CoreSampleSpanHours: stats.CoreSampleSpanHours,
		CoreCoveragePercent: stats.CoreCoveragePercent,
		CPUStealSamples:     stats.CPUStealSamples,
		CPUIoWaitSamples:    stats.CPUIoWaitSamples,
		CPUBenchSamples:     stats.CPUBenchSamples,
		IOLatencySamples:    stats.IOLatencySamples,
		RandomIOSamples:     stats.RandomIOSamples,
		DiskStatsSamples:    stats.DiskStatsSamples,
		MemorySamples:       stats.MemorySamples,
		CPULoadSamples:      stats.CPULoadSamples,
		CPUPressureSamples:  stats.CPUPressureSamples,
		IOPressureSamples:   stats.IOPressureSamples,
		CPUThrottleSamples:  stats.CPUThrottleSamples,
		HostContextSamples:  stats.HostContextSamples,
		CPUStealAvg:         stats.CPUStealAvg,
		CPUStealMax:         stats.CPUStealMax,
		CPUStealP95:         stats.CPUStealP95,
		CPUIoWaitAvg:        stats.CPUIoWaitAvg,
		CPUIoWaitMax:        stats.CPUIoWaitMax,
		CPUIoWaitP95:        stats.CPUIoWaitP95,
		CPUBenchAvg:         stats.CPUBenchAvg,
		CPUBenchCV:          stats.CPUBenchCV,
		IOLatencyAvg:        stats.IOLatencyAvg,
		IOLatencyP95:        stats.IOLatencyP95,
		IOLatencyP99:        stats.IOLatencyP99,
		RandomIOWriteAvg:    stats.RandomIOWriteAvg,
		RandomIOReadAvg:     stats.RandomIOReadAvg,
		RandomIOP95:         stats.RandomIOP95,
		RandomIODirectIO:    stats.RandomIODirectIOSamples,
		CPULoadAvg:          stats.CPULoadAvg,
		CPULoadMax:          stats.CPULoadMax,
		CPUPressureSomeAvg:  stats.CPUPressureSomeAvg,
		CPUPressureSomeP95:  stats.CPUPressureSomeP95,
		IOPressureSomeAvg:   stats.IOPressureSomeAvg,
		IOPressureSomeP95:   stats.IOPressureSomeP95,
		CPUThrottleAvg:      stats.CPUThrottleAvg,
		CPUThrottleP95:      stats.CPUThrottleP95,
		DiskBusyAvg:         stats.DiskBusyPercent,
		DiskBusyP95:         stats.DiskBusyP95,
		MemoryAvailablePct:  stats.MemoryAvailablePercent,
		BaselineDeviation:   stats.BaselineDeviation,
		BaselineStatus:      stats.BaselineStatus,
		BaselineMinDays:     stats.BaselineMinDays,
		BaselineQuality:     stats.BaselineQuality,
		BaselineReason:      stats.BaselineReason,
		BaselineMetrics:     stats.BaselineMetrics,
		StorageType:         stats.StorageType,
		TotalScore:          stats.TotalScore,
		RiskLevel:           stats.RiskLevel,
		VirtualizationType:  stats.VirtualizationType,
		HypervisorDetected:  stats.HypervisorDetected,
		ContainerDetected:   stats.ContainerDetected,
		StealDirect:         stats.StealDirectlyInterpretable,
	}, "", "  ")
}

func printReportJSON(reportType string, scoreAnalyzer *analyzer.Analyzer) {
	start, end, err := reportRange(reportType, time.Now())
	if err != nil {
		log.Fatal(err)
	}

	stats, err := scoreAnalyzer.AnalyzePeriod(reportType, start, end)
	if err != nil {
		log.Fatalf("分析数据失败: %v", err)
	}
	payload, err := buildReportJSON(stats)
	if err != nil {
		log.Fatalf("生成 JSON 报告失败: %v", err)
	}
	fmt.Println(string(payload))
}

const (
	reportCheckOK = iota
	reportCheckRisk
	reportCheckInsufficient
)

func checkReport(reportType string, scoreAnalyzer *analyzer.Analyzer) int {
	start, end, err := reportRange(reportType, time.Now())
	if err != nil {
		log.Print(err)
		return reportCheckInsufficient
	}

	stats, err := scoreAnalyzer.AnalyzePeriod(reportType, start, end)
	if err != nil {
		log.Printf("分析数据失败: %v", err)
		return reportCheckInsufficient
	}

	code, message := reportCheckResult(stats)
	fmt.Print(message)
	return code
}

func verifyOversellEvidence(reportType string, scoreAnalyzer *analyzer.Analyzer) int {
	start, end, err := reportRange(reportType, time.Now())
	if err != nil {
		log.Print(err)
		return reportCheckInsufficient
	}

	fmt.Println("🔎 1/2 环境诊断")
	diagnostics := collector.CollectDiagnostics()
	fmt.Print(formatDiagnostics(diagnostics))
	if !diagnostics.ReadyForOversellDetection() {
		fmt.Println("🔴 结果: 环境不满足关键采集要求")
		return reportCheckInsufficient
	}

	fmt.Println("🔎 2/2 报告证据检查")
	stats, err := scoreAnalyzer.AnalyzePeriod(reportType, start, end)
	if err != nil {
		log.Printf("分析数据失败: %v", err)
		return reportCheckInsufficient
	}
	return printVerifiedReportCheck(stats)
}

func printVerifiedReportCheck(stats *analyzer.PeriodStats) int {
	code, message := reportCheckResult(stats)
	fmt.Print(message)
	return code
}

func reportCheckResult(stats *analyzer.PeriodStats) (int, string) {
	var buf strings.Builder
	buf.WriteString("🔎 报告证据检查\n")
	buf.WriteString("━━━━━━━━━━━━━━━━━━\n")
	buf.WriteString(fmt.Sprintf("🧭 超售判定: %s\n", stats.OversellVerdict.Label()))
	buf.WriteString(fmt.Sprintf("🔎 证据等级: %s\n", stats.EvidenceLevel.Label()))
	buf.WriteString(fmt.Sprintf("🧪 核心样本: %d/%d，覆盖 %.1f%%\n",
		stats.CPUStealSamples, stats.CPUIoWaitSamples, stats.CoreCoveragePercent))
	buf.WriteString(fmt.Sprintf("🖥️ CPU Steal: 平均 %.2f%% / P95 %.2f%%\n",
		stats.CPUStealAvg, stats.CPUStealP95))
	buf.WriteString(fmt.Sprintf("📊 Load/PSI: Load %.2f，CPU PSI P95 %.2f%%，IO PSI P95 %.2f%%\n",
		stats.CPULoadAvg, stats.CPUPressureSomeP95, stats.IOPressureSomeP95))
	buf.WriteString(fmt.Sprintf("🧱 环境: virt=%s container=%t steal_direct=%t\n",
		stats.VirtualizationType, stats.ContainerDetected, stats.StealDirectlyInterpretable))
	buf.WriteString(fmt.Sprintf("🎲 随机 I/O: 样本 %d，O_DIRECT %d，P95 %.2fms\n",
		stats.RandomIOSamples, stats.RandomIODirectIOSamples, stats.RandomIOP95))

	if len(stats.MissingMetrics) > 0 {
		buf.WriteString(fmt.Sprintf("⚪ 缺失指标: %s\n", strings.Join(stats.MissingMetrics, ", ")))
	}
	if len(stats.QueryErrors) > 0 {
		buf.WriteString(fmt.Sprintf("🔴 查询错误: %s\n", strings.Join(stats.QueryErrors, " | ")))
	}
	for _, item := range stats.EvidenceSummary {
		buf.WriteString(fmt.Sprintf("   • %s\n", item))
	}
	buf.WriteString("━━━━━━━━━━━━━━━━━━\n")

	if stats.OversellVerdict == analyzer.OversellInsufficient || stats.EvidenceLevel == analyzer.EvidenceInsufficient || len(stats.QueryErrors) > 0 {
		buf.WriteString("⚪ 结果: 数据不足，不能判定\n")
		return reportCheckInsufficient, buf.String()
	}

	if stats.EvidenceLevel == analyzer.EvidenceLow {
		if stats.HostContextSamples == 0 || stats.ContainerDetected || !stats.StealDirectlyInterpretable {
			buf.WriteString("⚪ 结果: 运行环境无法可靠解释 CPU Steal，不能形成可靠判定\n")
			return reportCheckInsufficient, buf.String()
		}
		buf.WriteString("⚪ 结果: 证据等级偏低，不能形成可靠判定\n")
		return reportCheckInsufficient, buf.String()
	}

	if stats.OversellVerdict == analyzer.OversellLikely || stats.OversellVerdict == analyzer.OversellPossible {
		buf.WriteString("⚠️ 结果: 存在超售或资源争抢风险\n")
		return reportCheckRisk, buf.String()
	}

	if stats.OversellVerdict == analyzer.OversellLocalLoad {
		buf.WriteString("✅ 结果: 报告证据有效，当前更像本机负载导致\n")
		return reportCheckOK, buf.String()
	}

	buf.WriteString("✅ 结果: 报告证据有效，未判定超售\n")
	return reportCheckOK, buf.String()
}

func previewReport(reportType string, scoreAnalyzer *analyzer.Analyzer, telegramReporter *reporter.TelegramReporter) {
	start, end, err := reportRange(reportType, time.Now())
	if err != nil {
		log.Fatal(err)
	}

	stats, err := scoreAnalyzer.AnalyzePeriod(reportType, start, end)
	if err != nil {
		log.Fatalf("分析数据失败: %v", err)
	}

	fmt.Print(telegramReporter.FormatReport(stats, ""))
}

// generateReport 生成并发送报告

func generateReport(reportType string, scoreAnalyzer *analyzer.Analyzer, aiAnalyzer *analyzer.AIAnalyzer, telegramReporter *reporter.TelegramReporter) {
	start, end, err := reportRange(reportType, time.Now())
	if err != nil {
		log.Fatal(err)
	}

	stats, err := scoreAnalyzer.AnalyzePeriod(reportType, start, end)
	if err != nil {
		log.Fatalf("分析数据失败: %v", err)
	}

	// AI 分析
	aiAnalysis, err := aiAnalyzer.Analyze(stats, reportType)
	if err != nil {
		log.Printf("AI 分析失败 (降级为规则判定): %v", err)
	}

	// 发送报告
	if err := telegramReporter.SendReport(stats, aiAnalysis); err != nil {
		log.Fatalf("发送报告失败: %v", err)
	}

	fmt.Printf("✅ %s 报告已发送\n", reportType)
}

// runDaemon 守护进程模式
