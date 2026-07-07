package analyzer

import (
	"testing"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func TestAnalyzePeriodBuildsUsableRobustBaselineTrend(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	saveHostContext(t, store, start, true, false, true)
	saveBaselineDays(t, store, start, 8, 1, 10, 0.2)
	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i*2) * time.Hour)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 9, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeIOLatency, 30, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, end)
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.BaselineQuality != BaselineUsable {
		t.Fatalf("历史趋势应可参考，实际=%s", stats.BaselineQuality)
	}
	if stats.BaselineStatus != "degrading" {
		t.Fatalf("历史趋势应识别下降，实际=%s", stats.BaselineStatus)
	}
	if len(stats.BaselineMetrics) < 2 {
		t.Fatalf("应输出单指标趋势明细，实际=%d", len(stats.BaselineMetrics))
	}
	for _, trend := range stats.BaselineMetrics {
		if trend.Name == "io_latency_p95" && trend.BaselineP95 >= 20 {
			t.Fatalf("历史窗口不应被当前周期 I/O 样本污染，实际 P95=%.2f", trend.BaselineP95)
		}
	}
}

func TestAnalyzePeriodMarksContaminatedBaseline(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	saveBaselineDays(t, store, start, 8, 20, 10, 0.2)
	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i*2) * time.Hour)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 0.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeIOLatency, 10, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, end)
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.BaselineQuality != BaselineContaminated {
		t.Fatalf("异常历史窗口应标记为疑似污染，实际=%s", stats.BaselineQuality)
	}
	if stats.BaselineStatus != "contaminated" {
		t.Fatalf("异常历史窗口状态应为 contaminated，实际=%s", stats.BaselineStatus)
	}
	if stats.OversellVerdict == OversellPossible {
		t.Fatalf("疑似污染的历史趋势不应单独触发可能超售")
	}
}

func TestAnalyzePeriodMarksWeakBaselineQuality(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	saveBaselineDays(t, store, start, 4, 1, 10, 0.2)
	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i*2) * time.Hour)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 1.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeIOLatency, 12, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, end)
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.BaselineQuality != BaselineWeak {
		t.Fatalf("低覆盖历史趋势应为弱参考，实际=%s", stats.BaselineQuality)
	}
}
