package analyzer

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func TestBuildStealTimelineTilesRegularAndBurstSamples(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	regular := []*storage.Metric{
		{Timestamp: start, Value: 1},
		{Timestamp: start.Add(5 * time.Minute), Value: 1},
		{Timestamp: start.Add(10 * time.Minute), Value: 1},
		// 中断 3 小时后的样本只覆盖预热时间，权重不应超过典型间隔。
		{Timestamp: start.Add(190 * time.Minute), Value: 1},
	}
	burst := []*storage.Metric{
		{Timestamp: start.Add(5*time.Minute + 30*time.Second), Value: 50},
		{Timestamp: start.Add(6 * time.Minute), Value: 50},
	}
	all := append(append([]*storage.Metric(nil), regular...), burst...)

	slices := buildStealTimeline(all, regular)
	want := []time.Duration{5 * time.Minute, 5 * time.Minute, 30 * time.Second, 30 * time.Second, 4 * time.Minute, 5 * time.Minute}
	if len(slices) != len(want) {
		t.Fatalf("时间片数量不符: got=%d want=%d", len(slices), len(want))
	}
	for i, slice := range slices {
		if slice.weight != want[i] {
			t.Fatalf("第 %d 个时间片权重=%s，期望 %s", i, slice.weight, want[i])
		}
	}
}

func TestAnalyzeStealTimelineFindsWorstHourAndHighTime(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	var slices []stealSlice
	for i := 1; i <= 288; i++ {
		value := 0.5
		// 20:00-21:00 的 12 个时间片 Steal 为 20%。
		if i > 240 && i <= 252 {
			value = 20
		}
		slices = append(slices, stealSlice{end: start.Add(time.Duration(i) * 5 * time.Minute), weight: 5 * time.Minute, value: value})
	}

	stats := analyzeStealTimeline(slices)
	if math.Abs(stats.HighTimeHours-1) > 1e-9 || math.Abs(stats.HighTimePercent-100.0/24) > 1e-9 {
		t.Fatalf("高争抢时长应为 1 小时 / 4.17%%: %+v", stats)
	}
	if stats.WorstWindowAvg != 20 || !stats.WorstWindowEnd.Equal(start.Add(21*time.Hour)) {
		t.Fatalf("最差 1 小时应为 20:00-21:00 均值 20%%: %+v", stats)
	}
}

func TestAnalyzeStealTimelineIgnoresSparselyCoveredWindows(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	slices := []stealSlice{
		{end: start.Add(time.Minute), weight: 30 * time.Second, value: 100},
		{end: start.Add(2 * time.Minute), weight: 30 * time.Second, value: 100},
	}

	stats := analyzeStealTimeline(slices)
	if !stats.WorstWindowEnd.IsZero() || stats.WorstWindowAvg != 0 {
		t.Fatalf("覆盖不足一半的窗口不应参与最差窗口比较: %+v", stats)
	}
	if stats.HighTimePercent != 100 {
		t.Fatalf("高争抢占比仍应按已覆盖时间计算: %+v", stats)
	}
}

func TestAnalyzePeriodDetectsOneHourDailyStealPeak(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Now().Add(-24 * time.Hour).Truncate(time.Minute)
	writeStealDay(t, store, start, func(i int) float64 {
		if i >= 240 && i < 252 {
			return 20
		}
		return 0.5
	})

	stats, err := NewAnalyzer(store).AnalyzePeriod("daily", start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	// 1 小时高峰只占 4.2% 的样本，全周期均值和 P95 都识别不到。
	if stats.CPUStealAvg >= 3 || stats.CPUStealP95 >= 8 {
		t.Fatalf("前提不成立，全周期统计已能识别: avg=%.2f p95=%.2f", stats.CPUStealAvg, stats.CPUStealP95)
	}
	if stats.OversellVerdict != OversellLikely {
		t.Fatalf("每天 1 小时 20%% Steal 应判定为高度可能超售，实际=%s summary=%v", stats.OversellVerdict, stats.EvidenceSummary)
	}
	if len(stats.EvidenceSummary) == 0 || !strings.Contains(stats.EvidenceSummary[0], "高峰时段") {
		t.Fatalf("证据说明应指出高峰时段: %v", stats.EvidenceSummary)
	}
}

func TestAnalyzePeriodIgnoresShortStealSpike(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Now().Add(-24 * time.Hour).Truncate(time.Minute)
	writeStealDay(t, store, start, func(i int) float64 {
		// 10 分钟 30% 的短时抖动。
		if i == 100 || i == 101 {
			return 30
		}
		return 0.5
	})

	stats, err := NewAnalyzer(store).AnalyzePeriod("daily", start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if stats.OversellVerdict != OversellUnlikely {
		t.Fatalf("单次 10 分钟抖动不应形成超售证据，实际=%s summary=%v worst=%.2f high=%.2f%%",
			stats.OversellVerdict, stats.EvidenceSummary, stats.CPUStealWorstHourAvg, stats.CPUStealHighTimePercent)
	}
}

func TestAnalyzePeriodCountsBurstSamplesByCoveredTime(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	start := time.Now().Add(-24 * time.Hour).Truncate(time.Minute)
	writeStealDay(t, store, start, func(int) float64 { return 0.5 })
	// 常规样本之间的 40 分钟 burst，每 30 秒一个 Steal 25% 的样本。
	burstStart := start.Add(20 * time.Hour)
	for offset := 30 * time.Second; offset < 40*time.Minute; offset += 30 * time.Second {
		ts := burstStart.Add(offset)
		if ts.Sub(start)%(5*time.Minute) == 0 {
			continue
		}
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 25, storage.WithSamplingMode(nil, storage.SamplingModeBurst))
	}

	stats, err := NewAnalyzer(store).AnalyzePeriod("daily", start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if stats.CPUStealAvg != 0.5 {
		t.Fatalf("burst 样本不应改变常规均值，实际=%.2f", stats.CPUStealAvg)
	}
	// 每 5 分钟常规样本只覆盖其前 30 秒，burst 覆盖其余 4.5 分钟，约 36 分钟高争抢。
	if stats.CPUStealHighTimeHours < 0.5 || stats.CPUStealHighTimeHours > 0.7 {
		t.Fatalf("burst 时段应按覆盖时间计入高争抢时长，实际=%.3f 小时", stats.CPUStealHighTimeHours)
	}
	if stats.CPUStealWorstHourAvg < 12 {
		t.Fatalf("最差 1 小时应反映 burst 期间的争抢，实际=%.2f", stats.CPUStealWorstHourAvg)
	}
}

func TestAnalyzePeriodScalesPeakEvidenceForWeeklyReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		peakHour    func(day int) bool
		wantVerdict OversellVerdict
	}{
		{name: "daily recurring peak", peakHour: func(int) bool { return true }, wantVerdict: OversellLikely},
		{name: "single bad hour in week", peakHour: func(day int) bool { return day == 3 }, wantVerdict: OversellPossible},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := newTestStore(t)
			start := time.Now().Add(-7 * 24 * time.Hour).Truncate(time.Minute)
			writeStealSamples(t, store, start, 7*288, func(i int) float64 {
				if slot := i % 288; slot >= 240 && slot < 252 && tt.peakHour(i/288) {
					return 20
				}
				return 0.5
			})

			stats, err := NewAnalyzer(store).AnalyzePeriod("weekly", start, start.Add(7*24*time.Hour))
			if err != nil {
				t.Fatalf("分析失败: %v", err)
			}
			if stats.OversellVerdict != tt.wantVerdict {
				t.Fatalf("判定=%s，期望 %s；worst=%.2f high=%.2f%% summary=%v",
					stats.OversellVerdict, tt.wantVerdict, stats.CPUStealWorstHourAvg, stats.CPUStealHighTimePercent, stats.EvidenceSummary)
			}
		})
	}
}

// writeStealDay 写入 24 小时、每 5 分钟一个的常规核心样本，环境可直接解释 Steal。
func writeStealDay(t *testing.T, store *storage.Storage, start time.Time, steal func(i int) float64) {
	t.Helper()
	writeStealSamples(t, store, start, 288, steal)
}

func writeStealSamples(t *testing.T, store *storage.Storage, start time.Time, count int, steal func(i int) float64) {
	t.Helper()
	saveHostContext(t, store, start, true, false, true)
	for i := 0; i < count; i++ {
		ts := start.Add(time.Duration(i+1) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, steal(i), nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
	}
}
