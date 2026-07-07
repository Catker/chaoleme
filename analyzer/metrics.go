package analyzer

import (
	"math"
	"sort"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func calculateCPUThrottlePercents(metrics []*storage.Metric) []float64 {
	var percents []float64
	for i := 1; i < len(metrics); i++ {
		prevPeriods, okPrevPeriods := extraFloat(metrics[i-1], storage.ExtraPeriods)
		currPeriods, okCurrPeriods := extraFloat(metrics[i], storage.ExtraPeriods)
		prevThrottled, okPrevThrottled := extraFloat(metrics[i-1], storage.ExtraThrottledPeriods)
		currThrottled, okCurrThrottled := extraFloat(metrics[i], storage.ExtraThrottledPeriods)
		if !okPrevPeriods || !okCurrPeriods || !okPrevThrottled || !okCurrThrottled {
			continue
		}
		periodDelta := currPeriods - prevPeriods
		throttledDelta := currThrottled - prevThrottled
		if periodDelta <= 0 || throttledDelta < 0 {
			continue
		}
		percents = append(percents, clampPercent(throttledDelta/periodDelta*100))
	}
	if len(percents) > 0 {
		return percents
	}
	return extractValues(metrics)
}

func calculateDiskBusyPercents(metrics []*storage.Metric) []float64 {
	var busyPercents []float64

	for _, m := range metrics {
		if bp, ok := extraFloat(m, "busy_percent"); ok {
			busyPercents = append(busyPercents, clampPercent(bp))
		}
	}
	if len(busyPercents) > 0 {
		return busyPercents
	}

	for i := 1; i < len(metrics); i++ {
		prev := metrics[i-1]
		curr := metrics[i]
		wallMs := curr.Timestamp.Sub(prev.Timestamp).Seconds() * 1000
		if wallMs <= 0 || curr.Value < prev.Value {
			continue
		}
		busy := (curr.Value - prev.Value) / wallMs * 100
		busyPercents = append(busyPercents, clampPercent(busy))
	}

	return busyPercents
}

func extraFloat(m *storage.Metric, key string) (float64, bool) {
	if m.Extra == nil {
		return 0, false
	}
	v, ok := m.Extra[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

func extraBool(m *storage.Metric, key string) (bool, bool) {
	if m.Extra == nil {
		return false, false
	}
	v, ok := m.Extra[key]
	if !ok {
		return false, false
	}
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		return b == "true", true
	default:
		return false, false
	}
}

func extraString(m *storage.Metric, key string) (string, bool) {
	if m.Extra == nil {
		return "", false
	}
	v, ok := m.Extra[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// scoreCPUSteal CPU Steal 评分

func extractValues(metrics []*storage.Metric) []float64 {
	values := make([]float64, len(metrics))
	for i, m := range metrics {
		values[i] = m.Value
	}
	return values
}

func avg(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	index := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func coefficientOfVariation(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	mean := avg(values)
	if mean == 0 {
		return 0
	}

	// 计算标准差
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	stdDev := math.Sqrt(sumSquares / float64(len(values)))

	return stdDev / mean
}

// findMaxWithTime 找到最大值及其发生时间

func findMaxWithTime(metrics []*storage.Metric) (float64, time.Time) {
	if len(metrics) == 0 {
		return 0, time.Time{}
	}

	maxVal := metrics[0].Value
	maxTime := metrics[0].Timestamp

	for _, m := range metrics[1:] {
		if m.Value > maxVal {
			maxVal = m.Value
			maxTime = m.Timestamp
		}
	}

	return maxVal, maxTime
}

// calculateHourlyBreakdown 按小时聚合 CPU Steal 和 IOWait 统计

func calculateHourlyBreakdown(stealMetrics, iowaitMetrics []*storage.Metric) []HourlyStats {
	// 按小时分组数据
	type hourData struct {
		stealValues  []float64
		iowaitValues []float64
	}

	hourlyData := make(map[int]*hourData)

	// 收集 CPU Steal 数据
	for _, m := range stealMetrics {
		hour := m.Timestamp.Hour()
		if hourlyData[hour] == nil {
			hourlyData[hour] = &hourData{}
		}
		hourlyData[hour].stealValues = append(hourlyData[hour].stealValues, m.Value)
	}

	// 收集 IOWait 数据
	for _, m := range iowaitMetrics {
		hour := m.Timestamp.Hour()
		if hourlyData[hour] == nil {
			hourlyData[hour] = &hourData{}
		}
		hourlyData[hour].iowaitValues = append(hourlyData[hour].iowaitValues, m.Value)
	}

	// 生成按小时的统计结果
	var result []HourlyStats
	for hour := 0; hour < 24; hour++ {
		data := hourlyData[hour]
		if data == nil {
			continue
		}

		hs := HourlyStats{Hour: hour}

		if len(data.stealValues) > 0 {
			hs.SampleCount = len(data.stealValues)
			hs.CPUStealAvg = avg(data.stealValues)
			hs.CPUStealMax = max(data.stealValues)
		}

		if len(data.iowaitValues) > 0 {
			if len(data.iowaitValues) > hs.SampleCount {
				hs.SampleCount = len(data.iowaitValues)
			}
			hs.CPUIoWaitAvg = avg(data.iowaitValues)
			hs.CPUIoWaitMax = max(data.iowaitValues)
		}

		result = append(result, hs)
	}

	return result
}
