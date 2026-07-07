package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Catker/chaoleme/analyzer"
	"github.com/Catker/chaoleme/collector"
	"github.com/Catker/chaoleme/config"
	"github.com/Catker/chaoleme/reporter"
	"github.com/Catker/chaoleme/storage"
)

func TestReportRange(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		reportType string
		wantStart  time.Time
	}{
		{name: "daily", reportType: "daily", wantStart: now.AddDate(0, 0, -1)},
		{name: "weekly", reportType: "weekly", wantStart: now.AddDate(0, 0, -7)},
		{name: "monthly", reportType: "monthly", wantStart: now.AddDate(0, -1, 0)},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start, end, err := reportRange(tt.reportType, now)
			if err != nil {
				t.Fatalf("reportRange 返回错误: %v", err)
			}
			if !start.Equal(tt.wantStart) || !end.Equal(now) {
				t.Fatalf("时间范围不符合预期: start=%s end=%s", start, end)
			}
		})
	}
}

func TestReportRangeRejectsUnknownType(t *testing.T) {
	t.Parallel()

	_, _, err := reportRange("hourly", time.Now())
	if err == nil {
		t.Fatal("未知报告类型应返回错误")
	}
}

func TestBuildReportJSONIncludesEvidenceFields(t *testing.T) {
	t.Parallel()

	payload, err := buildReportJSON(&analyzer.PeriodStats{
		Period:                     "daily",
		OversellVerdict:            analyzer.OversellLikely,
		EvidenceLevel:              analyzer.EvidenceHigh,
		EvidenceSummary:            []string{"CPU Steal 已达到强证据阈值"},
		CoreCoveragePercent:        80,
		CPUStealSamples:            288,
		CPUIoWaitSamples:           288,
		RandomIOSamples:            96,
		RandomIODirectIOSamples:    96,
		CPUPressureSamples:         288,
		CPUThrottleSamples:         288,
		CPUStealP95:                16,
		RandomIOP95:                3.2,
		CPUPressureSomeP95:         12,
		CPUThrottleP95:             0,
		StorageType:                collector.StorageTypeSSD,
		TotalScore:                 72,
		RiskLevel:                  analyzer.RiskLevelGood,
		VirtualizationType:         "kvm",
		HypervisorDetected:         true,
		StealDirectlyInterpretable: true,
	})
	if err != nil {
		t.Fatalf("生成 JSON 失败: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("JSON 无法解析: %v\n%s", err, payload)
	}
	checks := map[string]string{
		"oversell_verdict": "likely",
		"evidence_level":   "high",
		"health_level":     "good",
	}
	for key, want := range checks {
		if got := decoded[key]; got != want {
			t.Fatalf("%s 不符合预期: got=%v want=%s", key, got, want)
		}
	}
	if decoded["core_coverage_percent"] != float64(80) {
		t.Fatalf("核心覆盖率不符合预期: %v", decoded["core_coverage_percent"])
	}
	numberChecks := map[string]float64{
		"cpu_steal_samples":        288,
		"cpu_iowait_samples":       288,
		"random_io_samples":        96,
		"random_io_direct_samples": 96,
		"cpu_pressure_samples":     288,
		"cpu_throttle_samples":     288,
		"cpu_steal_p95":            16,
		"random_io_p95":            3.2,
		"cpu_pressure_some_p95":    12,
	}
	for key, want := range numberChecks {
		if got := decoded[key]; got != want {
			t.Fatalf("%s 不符合预期: got=%v want=%.1f", key, got, want)
		}
	}
	if decoded["storage_type"] != string(collector.StorageTypeSSD) {
		t.Fatalf("存储类型不符合预期: %v", decoded["storage_type"])
	}
}

func TestReportCheckResultExitCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		stats    *analyzer.PeriodStats
		wantCode int
		wantText string
	}{
		{
			name: "insufficient",
			stats: &analyzer.PeriodStats{
				OversellVerdict: analyzer.OversellInsufficient,
				EvidenceLevel:   analyzer.EvidenceInsufficient,
				MissingMetrics:  []string{"cpu_steal"},
			},
			wantCode: reportCheckInsufficient,
			wantText: "数据不足",
		},
		{
			name: "risk",
			stats: &analyzer.PeriodStats{
				OversellVerdict:            analyzer.OversellLikely,
				EvidenceLevel:              analyzer.EvidenceHigh,
				CPUStealSamples:            288,
				CPUIoWaitSamples:           288,
				RandomIOSamples:            96,
				RandomIODirectIOSamples:    96,
				VirtualizationType:         "kvm",
				StealDirectlyInterpretable: true,
			},
			wantCode: reportCheckRisk,
			wantText: "资源争抢风险",
		},
		{
			name: "ok",
			stats: &analyzer.PeriodStats{
				OversellVerdict:  analyzer.OversellUnlikely,
				EvidenceLevel:    analyzer.EvidenceHigh,
				CPUStealSamples:  288,
				CPUIoWaitSamples: 288,
			},
			wantCode: reportCheckOK,
			wantText: "未判定超售",
		},
		{
			name: "local load",
			stats: &analyzer.PeriodStats{
				OversellVerdict:  analyzer.OversellLocalLoad,
				EvidenceLevel:    analyzer.EvidenceHigh,
				CPUStealSamples:  288,
				CPUIoWaitSamples: 288,
				CPULoadAvg:       1.6,
			},
			wantCode: reportCheckOK,
			wantText: "本机负载",
		},
		{
			name: "low evidence unlikely",
			stats: &analyzer.PeriodStats{
				OversellVerdict:   analyzer.OversellUnlikely,
				EvidenceLevel:     analyzer.EvidenceLow,
				CPUStealSamples:   288,
				CPUIoWaitSamples:  288,
				ContainerDetected: true,
				EvidenceSummary:   []string{"运行环境未知或处于容器中，未发现明显证据但不能强证明无超售"},
			},
			wantCode: reportCheckInsufficient,
			wantText: "运行环境无法可靠解释",
		},
		{
			name: "low evidence possible",
			stats: &analyzer.PeriodStats{
				OversellVerdict:            analyzer.OversellPossible,
				EvidenceLevel:              analyzer.EvidenceLow,
				CPUStealSamples:            288,
				CPUIoWaitSamples:           288,
				HostContextSamples:         1,
				StealDirectlyInterpretable: true,
				EvidenceSummary:            []string{"存在资源争抢迹象，但证据链不完整"},
			},
			wantCode: reportCheckInsufficient,
			wantText: "证据等级偏低",
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, message := reportCheckResult(tt.stats)
			if code != tt.wantCode {
				t.Fatalf("退出码不符合预期: got=%d want=%d\n%s", code, tt.wantCode, message)
			}
			if !strings.Contains(message, tt.wantText) {
				t.Fatalf("输出缺少 %q，内容:\n%s", tt.wantText, message)
			}
			for _, want := range []string{"Load/PSI", "环境", "随机 I/O"} {
				if !strings.Contains(message, want) {
					t.Fatalf("报告检查输出缺少辅助证据摘要 %q，内容:\n%s", want, message)
				}
			}
		})
	}
}

func TestSyntheticLikelyOversellReportEndToEnd(t *testing.T) {
	t.Parallel()

	store, err := storage.New(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	end := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -1)
	writeTestMetric(t, store, start, storage.MetricTypeHostContext, 1, map[string]interface{}{
		storage.ExtraHypervisorDetected:         true,
		storage.ExtraContainerDetected:          false,
		storage.ExtraVirtualizationType:         "kvm",
		storage.ExtraStealDirectlyInterpretable: true,
	})

	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 2 * time.Hour)
		writeTestMetric(t, store, ts, storage.MetricTypeCPUSteal, 10, nil)
		writeTestMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		writeTestMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
		writeTestMetric(t, store, ts, storage.MetricTypeCPUPressure, 12, nil)
	}
	for i, value := range []float64{100, 105, 95} {
		writeTestMetric(t, store, start.Add(time.Duration(i)*8*time.Hour), storage.MetricTypeCPUBench, value, nil)
	}
	writeTestMetric(t, store, start.Add(time.Hour), storage.MetricTypeIOLatency, 5, nil)
	writeTestMetric(t, store, start.Add(time.Hour), storage.MetricTypeRandomIO, 2, map[string]interface{}{
		storage.ExtraWriteLatencyMS: 2.0,
		storage.ExtraReadLatencyMS:  1.0,
		storage.ExtraDirectIOWrite:  true,
		storage.ExtraDirectIORead:   true,
	})
	writeTestMetric(t, store, start.Add(time.Hour), storage.MetricTypeMemory, 20, map[string]interface{}{
		storage.ExtraAvailablePercent: 80.0,
	})
	writeTestMetric(t, store, start, storage.MetricTypeDiskStats, 1000, nil)
	writeTestMetric(t, store, start.Add(12*time.Hour), storage.MetricTypeDiskStats, 2000, nil)

	scoreAnalyzer := analyzer.NewAnalyzer(store)
	stats, err := scoreAnalyzer.AnalyzePeriod("daily", start, end)
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if stats.OversellVerdict != analyzer.OversellLikely {
		t.Fatalf("期望高度可能超售，实际=%s", stats.OversellVerdict)
	}
	if stats.EvidenceLevel != analyzer.EvidenceHigh {
		t.Fatalf("期望高证据等级，实际=%s", stats.EvidenceLevel)
	}
	if stats.CoreCoveragePercent < 50 {
		t.Fatalf("核心覆盖率应达标，实际=%.1f", stats.CoreCoveragePercent)
	}

	report := reporter.NewTelegramReporter(&config.TelegramConfig{}, "test-vps").FormatReport(stats, "")
	for _, want := range []string{"高度可能超售", "证据等级: 高", "CPU Steal 已达到强证据阈值", "运行环境"} {
		if !strings.Contains(report, want) {
			t.Fatalf("报告缺少 %q，内容:\n%s", want, report)
		}
	}
}

func writeTestMetric(t *testing.T, store *storage.Storage, ts time.Time, typ storage.MetricType, value float64, extra map[string]interface{}) {
	t.Helper()
	if err := store.Save(&storage.Metric{Timestamp: ts, Type: typ, Value: value, Extra: extra}); err != nil {
		t.Fatalf("保存测试指标失败: %v", err)
	}
}
