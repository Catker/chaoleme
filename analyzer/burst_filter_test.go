package analyzer

import (
	"reflect"
	"testing"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func TestAnalyzePeriodBurstSamplesDoNotAffectLegacyFields(t *testing.T) {
	start := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	regularStore := newTestStore(t)
	withBurstStore := newTestStore(t)
	writeRegularPeriod(t, regularStore, start)
	writeRegularPeriod(t, withBurstStore, start)
	writeBurstPeriod(t, withBurstStore, start.Add(6*time.Hour))

	regularStats, err := NewAnalyzer(regularStore).AnalyzePeriod("daily", start, end)
	if err != nil {
		t.Fatalf("常规数据分析失败: %v", err)
	}
	burstStats, err := NewAnalyzer(withBurstStore).AnalyzePeriod("daily", start, end)
	if err != nil {
		t.Fatalf("包含 burst 的数据分析失败: %v", err)
	}
	regularStats.ContentionEvents = nil
	burstStats.ContentionEvents = nil
	// 时间加权的 Steal 统计按设计包含 burst 样本；3 秒的 burst 不应改变结论。
	if burstStats.CPUStealHighTimeHours <= regularStats.CPUStealHighTimeHours {
		t.Fatalf("burst 样本应计入时间加权统计: regular=%.6f burst=%.6f", regularStats.CPUStealHighTimeHours, burstStats.CPUStealHighTimeHours)
	}
	clearStealTimelineFields(regularStats)
	clearStealTimelineFields(burstStats)
	if !reflect.DeepEqual(regularStats, burstStats) {
		t.Fatalf("burst 样本改变了常规统计或结论:\n常规=%+v\nburst=%+v", regularStats, burstStats)
	}
}

func clearStealTimelineFields(stats *PeriodStats) {
	stats.CPUStealHighTimePercent = 0
	stats.CPUStealHighTimeHours = 0
	stats.CPUStealWorstHourAvg = 0
	stats.CPUStealWorstHourStart = time.Time{}
}

func writeRegularPeriod(t *testing.T, store *storage.Storage, start time.Time) {
	t.Helper()
	saveHostContext(t, store, start, true, false, true)
	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * time.Hour)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUBench, 100, nil)
		saveMetric(t, store, ts, storage.MetricTypeIOLatency, 5, nil)
		saveMetric(t, store, ts, storage.MetricTypeRandomIO, 2, storage.NewRandomIOExtra(2, 1, true, true))
		saveMetric(t, store, ts, storage.MetricTypeMemory, 20, storage.NewMemoryExtra(100, 80, 80, 0))
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, storage.NewLoadExtra(0.2, 0.2, 0.2, 1))
		saveMetric(t, store, ts, storage.MetricTypeCPUPressure, 1, storage.NewPressureExtra(1, 1, 1, 1, 0, 0, 0, 0, false))
		saveMetric(t, store, ts, storage.MetricTypeIOPressure, 1, storage.NewPressureExtra(1, 1, 1, 1, 0, 0, 0, 0, false))
		saveMetric(t, store, ts, storage.MetricTypeCPUThrottle, 0, storage.NewCPUThrottleExtra(uint64(100+i*100), uint64(i), 0))
		saveMetric(t, store, ts, storage.MetricTypeDiskStats, float64(i*1000), storage.NewDiskStatsExtra("vda", 0, 0, 0, 0, uint64(i*1000), 0))
	}
}

func writeBurstPeriod(t *testing.T, store *storage.Storage, start time.Time) {
	t.Helper()
	for i := 0; i < 3; i++ {
		ts := start.Add(time.Duration(i) * time.Second)
		mode := storage.SamplingModeBurst
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 100, storage.WithSamplingMode(nil, mode))
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 100, storage.WithSamplingMode(nil, mode))
		saveMetric(t, store, ts, storage.MetricTypeCPUBench, 10000, storage.WithSamplingMode(nil, mode))
		saveMetric(t, store, ts, storage.MetricTypeIOLatency, 10000, storage.WithSamplingMode(storage.NewIOLatencyExtra(10000, 10000), mode))
		saveMetric(t, store, ts, storage.MetricTypeRandomIO, 10000, storage.WithSamplingMode(storage.NewRandomIOExtra(10000, 10000, true, true), mode))
		saveMetric(t, store, ts, storage.MetricTypeMemory, 99, storage.WithSamplingMode(storage.NewMemoryExtra(100, 1, 1, 99), mode))
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 100, storage.WithSamplingMode(storage.NewLoadExtra(100, 100, 100, 1), mode))
		saveMetric(t, store, ts, storage.MetricTypeCPUPressure, 100, storage.WithSamplingMode(storage.NewPressureExtra(100, 100, 100, 1, 0, 0, 0, 0, false), mode))
		saveMetric(t, store, ts, storage.MetricTypeIOPressure, 100, storage.WithSamplingMode(storage.NewPressureExtra(100, 100, 100, 1, 0, 0, 0, 0, false), mode))
		saveMetric(t, store, ts, storage.MetricTypeCPUThrottle, 100, storage.WithSamplingMode(storage.NewCPUThrottleExtra(uint64(1000+i*100), uint64(1000+i*100), 0), mode))
		saveMetric(t, store, ts, storage.MetricTypeDiskStats, float64(100000+i), storage.WithSamplingMode(storage.NewDiskStatsExtra("other", 0, 0, 0, 0, uint64(100000+i), 0), mode))
		saveMetric(t, store, ts, storage.MetricTypeHostContext, 0, storage.WithSamplingMode(storage.NewHostContextExtra(false, true, "container", false), mode))
	}
}
