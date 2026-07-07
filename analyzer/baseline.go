package analyzer

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/Catker/chaoleme/storage"
)

type baselinePeriodConfig struct {
	days          int
	minDays       int
	usableMinDays int
}

type baselineMetricSpec struct {
	name              string
	current           float64
	currentMetrics    []*storage.Metric
	baselineMetrics   []*storage.Metric
	minSamples        int
	lowReference      float64
	degradeThreshold  float64
	improveThreshold  float64
	contaminateP95    float64
	evidenceCandidate bool
}

func baselineConfigForPeriod(period string) baselinePeriodConfig {
	switch period {
	case "weekly":
		return baselinePeriodConfig{days: 28, minDays: 7, usableMinDays: 14}
	case "monthly":
		return baselinePeriodConfig{days: 60, minDays: 14, usableMinDays: 30}
	default:
		return baselinePeriodConfig{days: 14, minDays: 3, usableMinDays: 7}
	}
}

// calculateBaselineTrend 计算历史趋势。
// 只把 CPU Steal 与 I/O 延迟作为趋势证据候选；Load 仅展示，不参与趋势证据判定。

func (a *Analyzer) calculateBaselineTrend(stats *PeriodStats, period string, currentSteal, currentIO, currentLoad []*storage.Metric) {
	cfg := baselineConfigForPeriod(period)
	stats.BaselineMinDays = cfg.minDays

	baselineEnd := stats.StartTime.Add(-time.Second)
	baselineStart := baselineEnd.AddDate(0, 0, -cfg.days)

	baselineSteal, errSteal := a.store.Query(storage.MetricTypeCPUSteal, baselineStart, baselineEnd)
	baselineIO, errIO := a.store.Query(storage.MetricTypeIOLatency, baselineStart, baselineEnd)
	baselineLoad, errLoad := a.store.Query(storage.MetricTypeCPULoad, baselineStart, baselineEnd)
	if errSteal != nil || errIO != nil || errLoad != nil {
		stats.BaselineQuality = BaselineUnavailable
		stats.BaselineStatus = "building"
		stats.BaselineReason = "历史指标查询失败"
		return
	}

	daysWithData := a.countDaysWithData(baselineSteal, baselineIO)
	if daysWithData > cfg.days {
		daysWithData = cfg.days
	}
	if daysWithData < cfg.minDays {
		stats.BaselineDeviation = float64(daysWithData)
		stats.BaselineQuality = BaselineUnavailable
		stats.BaselineStatus = "building"
		stats.BaselineReason = "历史样本天数不足"
		return
	}

	specs := []baselineMetricSpec{
		{
			name:              "cpu_steal_p95",
			current:           stats.CPUStealP95,
			currentMetrics:    currentSteal,
			baselineMetrics:   baselineSteal,
			minSamples:        cfg.minDays,
			lowReference:      1.0,
			degradeThreshold:  15,
			improveThreshold:  15,
			contaminateP95:    15,
			evidenceCandidate: true,
		},
		{
			name:              "io_latency_p95",
			current:           stats.IOLatencyP95,
			currentMetrics:    currentIO,
			baselineMetrics:   baselineIO,
			minSamples:        cfg.minDays,
			lowReference:      15,
			degradeThreshold:  15,
			improveThreshold:  15,
			contaminateP95:    100,
			evidenceCandidate: true,
		},
		{
			name:             "cpu_load_avg",
			current:          stats.CPULoadAvg,
			currentMetrics:   currentLoad,
			baselineMetrics:  baselineLoad,
			minSamples:       cfg.minDays,
			lowReference:     0.3,
			degradeThreshold: 30,
			improveThreshold: 30,
		},
	}

	quality := BaselineUsable
	if daysWithData < cfg.usableMinDays {
		quality = BaselineWeak
	}

	var evidenceDeviations []float64
	contaminated := false
	for _, spec := range specs {
		trend, ok := calculateBaselineMetricTrend(spec, quality)
		if !ok {
			continue
		}
		stats.BaselineMetrics = append(stats.BaselineMetrics, trend)
		if trend.EvidenceCandidate && trend.BaselineP95 >= spec.contaminateP95 {
			contaminated = true
		}
		if trend.EvidenceCandidate {
			evidenceDeviations = append(evidenceDeviations, trend.DeviationPercent)
		}
	}

	if len(evidenceDeviations) == 0 {
		stats.BaselineQuality = BaselineUnavailable
		stats.BaselineStatus = "building"
		stats.BaselineReason = "历史趋势候选指标不足"
		return
	}

	if contaminated {
		stats.BaselineQuality = BaselineContaminated
		stats.BaselineStatus = "contaminated"
		stats.BaselineReason = "历史窗口内核心指标已明显异常"
		return
	}

	stats.BaselineQuality = quality
	stats.BaselineDeviation, stats.BaselineStatus = summarizeBaselineStatus(evidenceDeviations)
	stats.BaselineReason = fmt.Sprintf("历史覆盖 %d/%d 天，按当前报告时段匹配样本", daysWithData, cfg.days)
}

func calculateBaselineMetricTrend(spec baselineMetricSpec, quality BaselineQuality) (BaselineMetricTrend, bool) {
	if len(spec.currentMetrics) == 0 {
		return BaselineMetricTrend{}, false
	}
	matched := matchBaselineHours(spec.baselineMetrics, spec.currentMetrics)
	if len(matched) < spec.minSamples {
		return BaselineMetricTrend{}, false
	}

	values := extractValues(matched)
	med := median(values)
	p75 := percentile(values, 75)
	p95 := percentile(values, 95)
	denominator := med
	if denominator < spec.lowReference {
		denominator = spec.lowReference
	}
	if denominator <= 0 {
		return BaselineMetricTrend{}, false
	}

	deviation := (spec.current - med) / denominator * 100
	robustScale := medianAbsoluteDeviation(values, med) * 1.4826
	minScale := spec.lowReference * 0.1
	if robustScale < minScale {
		robustScale = minScale
	}

	status := "stable"
	switch {
	case deviation >= spec.degradeThreshold:
		status = "degrading"
	case deviation <= -spec.improveThreshold:
		status = "improving"
	}

	return BaselineMetricTrend{
		Name:              spec.name,
		Status:            status,
		Quality:           quality,
		Samples:           len(matched),
		Days:              countDaysInMetrics(matched),
		Current:           spec.current,
		BaselineMedian:    med,
		BaselineP75:       p75,
		BaselineP95:       p95,
		DeviationPercent:  deviation,
		RobustZ:           (spec.current - med) / robustScale,
		EvidenceCandidate: spec.evidenceCandidate,
	}, true
}

func matchBaselineHours(baselineMetrics, currentMetrics []*storage.Metric) []*storage.Metric {
	if len(baselineMetrics) == 0 || len(currentMetrics) == 0 {
		return baselineMetrics
	}
	hours := make(map[int]bool)
	for _, m := range currentMetrics {
		hours[m.Timestamp.Hour()] = true
	}
	matched := make([]*storage.Metric, 0, len(baselineMetrics))
	for _, m := range baselineMetrics {
		if hours[m.Timestamp.Hour()] {
			matched = append(matched, m)
		}
	}
	if len(matched) == 0 {
		return baselineMetrics
	}
	return matched
}

func summarizeBaselineStatus(deviations []float64) (float64, string) {
	maxDegrade := 0.0
	maxImprove := 0.0
	for _, d := range deviations {
		if d > maxDegrade {
			maxDegrade = d
		}
		if -d > maxImprove {
			maxImprove = -d
		}
	}
	switch {
	case maxDegrade >= 15:
		return maxDegrade, "degrading"
	case maxImprove >= 15:
		return maxImprove, "improving"
	default:
		if maxDegrade > maxImprove {
			return maxDegrade, "stable"
		}
		return maxImprove, "stable"
	}
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func medianAbsoluteDeviation(values []float64, med float64) float64 {
	if len(values) == 0 {
		return 0
	}
	deviations := make([]float64, len(values))
	for i, v := range values {
		deviations[i] = math.Abs(v - med)
	}
	return median(deviations)
}

func countDaysInMetrics(metrics []*storage.Metric) int {
	daysSet := make(map[string]bool)
	for _, m := range metrics {
		daysSet[m.Timestamp.Format("2006-01-02")] = true
	}
	return len(daysSet)
}

// countDaysWithData 统计有数据的天数

func (a *Analyzer) countDaysWithData(stealMetrics, ioMetrics []*storage.Metric) int {
	daysSet := make(map[string]bool)

	for _, m := range stealMetrics {
		dayKey := m.Timestamp.Format("2006-01-02")
		daysSet[dayKey] = true
	}
	for _, m := range ioMetrics {
		dayKey := m.Timestamp.Format("2006-01-02")
		daysSet[dayKey] = true
	}

	return len(daysSet)
}

// 辅助函数
