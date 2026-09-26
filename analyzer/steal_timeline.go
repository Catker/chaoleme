package analyzer

import (
	"fmt"
	"sort"
	"time"

	"github.com/Catker/chaoleme/storage"
)

const (
	// stealHighThreshold 单个时间片 Steal 达到该值视为高争抢时段。
	stealHighThreshold = 10.0
	// stealWindow 与 stealWindowMinCoverage 定义"最差 1 小时"：窗口内样本覆盖不足一半时不参与比较，
	// 避免少量 burst 样本单独决定窗口均值。
	stealWindow            = time.Hour
	stealWindowMinCoverage = 0.5

	// 峰值时段证据阈值。高争抢时长按覆盖时间的占比计算，日报 4% 约等于 1 小时，
	// 周报/月报按比例放大，因此每天固定高峰也能被识别。
	stealPeakStrongWindowAvg = 15.0
	stealPeakStrongHighTime  = 4.0
	stealPeakMediumWindowAvg = 8.0
	stealPeakMediumHighTime  = 2.0
)

// stealSlice 是一个不重叠的 Steal 时间片，覆盖 (end-weight, end]。
type stealSlice struct {
	end    time.Time
	weight time.Duration
	value  float64
}

// stealTimelineStats 是按时间加权的 Steal 统计。
type stealTimelineStats struct {
	HighTimePercent float64
	HighTimeHours   float64
	WorstWindowAvg  float64
	WorstWindowEnd  time.Time
}

// buildStealTimeline 把常规与 burst 样本合并成不重叠的时间片。
// 两种采样共用同一个 CPUCollector，每个样本都是距上一次读取 /proc/stat 的累计增量，
// 所以按时间排序后样本 i 恰好覆盖 (t[i-1], t[i]]。
// 首个样本和长时间中断后的样本实际只覆盖很短的预热时间，权重上限取常规采样的典型间隔。
func buildStealTimeline(all, regular []*storage.Metric) []stealSlice {
	if len(all) == 0 {
		return nil
	}
	sorted := append([]*storage.Metric(nil), all...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})
	maxWeight := typicalSampleInterval(regular)

	slices := make([]stealSlice, 0, len(sorted))
	for i, metric := range sorted {
		weight := maxWeight
		if i > 0 {
			if gap := metric.Timestamp.Sub(sorted[i-1].Timestamp); gap < weight {
				weight = gap
			}
		}
		if weight <= 0 {
			continue
		}
		slices = append(slices, stealSlice{end: metric.Timestamp, weight: weight, value: metric.Value})
	}
	return slices
}

// typicalSampleInterval 取常规样本间隔的中位数，样本不足时使用默认采样间隔。
func typicalSampleInterval(metrics []*storage.Metric) time.Duration {
	var intervals []time.Duration
	for i := 1; i < len(metrics); i++ {
		if interval := metrics[i].Timestamp.Sub(metrics[i-1].Timestamp); interval > 0 {
			intervals = append(intervals, interval)
		}
	}
	if len(intervals) == 0 {
		return defaultContentionSampleInterval
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i] < intervals[j] })
	return intervals[(len(intervals)-1)/2]
}

// analyzeStealTimeline 计算高争抢时长占比和最差 1 小时窗口均值。
func analyzeStealTimeline(slices []stealSlice) stealTimelineStats {
	var result stealTimelineStats
	var total, high time.Duration
	for _, slice := range slices {
		total += slice.weight
		if slice.value >= stealHighThreshold {
			high += slice.weight
		}
	}
	if total <= 0 {
		return result
	}
	result.HighTimePercent = float64(high) / float64(total) * 100
	result.HighTimeHours = high.Hours()

	// 双指针滑动窗口：窗口包含结束时间落在 (end_j-1h, end_j] 的时间片。
	var windowWeight time.Duration
	var windowWeighted float64
	left := 0
	for right, slice := range slices {
		windowWeight += slice.weight
		windowWeighted += slice.value * float64(slice.weight)
		for !slices[left].end.After(slice.end.Add(-stealWindow)) {
			windowWeight -= slices[left].weight
			windowWeighted -= slices[left].value * float64(slices[left].weight)
			left++
		}
		if float64(windowWeight) < float64(stealWindow)*stealWindowMinCoverage {
			continue
		}
		if avg := windowWeighted / float64(windowWeight); avg > result.WorstWindowAvg || result.WorstWindowEnd.IsZero() {
			result.WorstWindowAvg = avg
			result.WorstWindowEnd = slices[right].end
		}
	}
	return result
}

// StealWorstHourLabel 格式化最差 1 小时窗口，没有足够覆盖的窗口时返回 N/A。
func (s *PeriodStats) StealWorstHourLabel() string {
	if s.CPUStealWorstHourStart.IsZero() {
		return "N/A"
	}
	end := s.CPUStealWorstHourStart.Add(stealWindow)
	return fmt.Sprintf("%s~%s", s.CPUStealWorstHourStart.Format("01-02 15:04"), end.Format("15:04"))
}
