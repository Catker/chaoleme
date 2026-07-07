package analyzer

import (
	"testing"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func TestAnalyzePeriodMarksInsufficientOnQueryError(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	if err := store.Close(); err != nil {
		t.Fatalf("关闭测试存储失败: %v", err)
	}

	stats, err := analyzer.AnalyzePeriod("daily", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("分析不应直接失败，应转为证据不足: %v", err)
	}
	if stats.OversellVerdict != OversellInsufficient {
		t.Fatalf("查询失败时应为证据不足，实际=%s", stats.OversellVerdict)
	}
	if len(stats.QueryErrors) == 0 {
		t.Fatal("查询失败应记录 QueryErrors")
	}
}

func TestAnalyzePeriodMarksInsufficientWhenCoreMetricsMissing(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	now := time.Now()

	stats, err := analyzer.AnalyzePeriod("daily", now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellInsufficient {
		t.Fatalf("缺少核心指标时应为数据不足，实际=%s", stats.OversellVerdict)
	}
	if stats.EvidenceLevel != EvidenceInsufficient {
		t.Fatalf("缺少核心指标时证据等级应不足，实际=%s", stats.EvidenceLevel)
	}
	if stats.TotalScore >= 90 {
		t.Fatalf("缺少指标时不应得到优秀健康分，实际=%.1f", stats.TotalScore)
	}
	if stats.RiskLevel != RiskLevelUnknown {
		t.Fatalf("证据不足时健康等级应为 unknown，实际=%s", stats.RiskLevel)
	}
}

func TestAnalyzePeriodRequiresEnoughCoreSamples(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)

	for i := 0; i < 3; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 20, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.1, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellInsufficient {
		t.Fatalf("核心样本不足时不应直接判定超售，实际=%s", stats.OversellVerdict)
	}
}

func TestAnalyzePeriodRequiresCoreSampleCoverage(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-24 * time.Hour)
	saveHostContext(t, store, start, true, false, true)

	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 20, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.1, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellInsufficient {
		t.Fatalf("覆盖率不足时不应判定整日报告，实际=%s", stats.OversellVerdict)
	}
	if stats.CoreCoveragePercent >= 50 {
		t.Fatalf("测试数据覆盖率应低于 50%%，实际=%.1f", stats.CoreCoveragePercent)
	}
}

func TestCalculateCoreSampleCoverageUsesIntersection(t *testing.T) {
	t.Parallel()

	start := time.Now()
	end := start.Add(10 * time.Hour)
	steal := []*storage.Metric{
		{Timestamp: start.Add(1 * time.Hour)},
		{Timestamp: start.Add(8 * time.Hour)},
	}
	iowait := []*storage.Metric{
		{Timestamp: start.Add(2 * time.Hour)},
		{Timestamp: start.Add(9 * time.Hour)},
	}

	spanHours, coverage := calculateCoreSampleCoverage(steal, iowait, start, end)
	if spanHours != 6 {
		t.Fatalf("期望交集跨度 6 小时，实际 %.1f", spanHours)
	}
	if coverage != 60 {
		t.Fatalf("期望覆盖率 60%%，实际 %.1f", coverage)
	}
}

func TestMinCoreCoveragePercentVariesByPeriod(t *testing.T) {
	t.Parallel()

	checks := map[string]float64{
		"daily":   50,
		"weekly":  60,
		"monthly": 70,
	}
	for period, want := range checks {
		if got := minCoreCoveragePercentForPeriod(period); got != want {
			t.Fatalf("%s 覆盖率阈值不符合预期: got=%.0f want=%.0f", period, got, want)
		}
	}
}

func TestScoreMemoryDistinguishesVeryLowAvailability(t *testing.T) {
	t.Parallel()

	analyzer := NewAnalyzer(nil)
	if got := analyzer.scoreMemory(50); got != 100 {
		t.Fatalf("50%% 可用内存应为健康，实际=%.0f", got)
	}
	if got := analyzer.scoreMemory(4); got != 0 {
		t.Fatalf("极低可用内存应为 0 分，实际=%.0f", got)
	}
}

func TestAnalyzePeriodKeepsCPUStealMaxAndTimeConsistent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)

	for i := 0; i < 12; i++ {
		value := 1.0
		if i == 7 {
			value = 42
		}
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, value, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if stats.CPUStealMax != 42 {
		t.Fatalf("CPUStealMax 应为真实最大值，实际=%.1f", stats.CPUStealMax)
	}
	wantTime := start.Add(7 * 5 * time.Minute).Truncate(time.Second)
	if !stats.CPUStealMaxTime.Equal(wantTime) {
		t.Fatalf("CPUStealMaxTime 应对应最大值时间: got=%s want=%s", stats.CPUStealMaxTime, wantTime)
	}
}
