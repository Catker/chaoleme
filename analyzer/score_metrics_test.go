package analyzer

import (
	"testing"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func TestCalculateCPUThrottlePercentsFromCumulativeCounters(t *testing.T) {
	t.Parallel()

	start := time.Now()
	metrics := []*storage.Metric{
		{
			Timestamp: start,
			Value:     1,
			Extra: map[string]interface{}{
				storage.ExtraPeriods:          float64(1000),
				storage.ExtraThrottledPeriods: float64(10),
			},
		},
		{
			Timestamp: start.Add(time.Minute),
			Value:     2,
			Extra: map[string]interface{}{
				storage.ExtraPeriods:          float64(1100),
				storage.ExtraThrottledPeriods: float64(60),
			},
		},
	}

	percents := calculateCPUThrottlePercents(metrics)
	if len(percents) != 1 {
		t.Fatalf("期望 1 个节流比例样本，实际=%d", len(percents))
	}
	if percents[0] != 50 {
		t.Fatalf("期望按增量计算得到 50%%，实际=%.1f", percents[0])
	}
}

func TestCalculateDiskBusyPercentsFromCumulativeIOTime(t *testing.T) {
	t.Parallel()

	start := time.Now()
	metrics := []*storage.Metric{
		{Timestamp: start, Value: 1000},
		{Timestamp: start.Add(time.Minute), Value: 31000},
	}

	busy := calculateDiskBusyPercents(metrics)
	if len(busy) != 1 {
		t.Fatalf("期望 1 个繁忙度样本，实际=%d", len(busy))
	}
	if busy[0] != 50 {
		t.Fatalf("期望繁忙度 50%%，实际=%.1f", busy[0])
	}
}

func TestIsHostContextFresh(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	if !isHostContextFresh(now.Add(-7*24*time.Hour), now) {
		t.Fatal("7 天内 host_context 应视为新鲜")
	}
	if isHostContextFresh(now.Add(-7*24*time.Hour-time.Second), now) {
		t.Fatal("超过 7 天的 host_context 应视为过旧")
	}
	if !isHostContextFresh(now.Add(time.Minute), now) {
		t.Fatal("略晚于报告结束时间的 host_context 应可使用")
	}
}

func TestDetectContentionEventsSortsAndDoesNotCrossGaps(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	metrics := []*storage.Metric{
		{Timestamp: start.Add(25 * time.Minute), Value: 10},
		{Timestamp: start.Add(5 * time.Minute), Value: 12},
		{Timestamp: start, Value: 8},
		{Timestamp: start.Add(30 * time.Minute), Value: 9},
	}
	events := detectContentionEvents("cpu_steal", 8, metrics)
	if len(events) != 2 {
		t.Fatalf("期望两个连续事件，实际=%d", len(events))
	}
	if !events[0].StartTime.Equal(start) || !events[0].EndTime.Equal(start.Add(5*time.Minute)) || events[0].SampleCount != 2 || events[0].PeakPercent != 12 {
		t.Fatalf("第一个事件不符合预期: %+v", events[0])
	}
	if !events[1].StartTime.Equal(start.Add(25*time.Minute)) || !events[1].EndTime.Equal(start.Add(30*time.Minute)) || events[1].SampleCount != 2 || events[1].PeakPercent != 10 {
		t.Fatalf("第二个事件不符合预期: %+v", events[1])
	}
}

func TestDetectContentionEventsAcceptsStableTwentyMinuteSampling(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	metrics := []*storage.Metric{
		{Timestamp: start.Add(40 * time.Minute), Value: 10},
		{Timestamp: start, Value: 8},
		{Timestamp: start.Add(20 * time.Minute), Value: 12},
	}
	events := detectContentionEvents("cpu_steal", 8, metrics)
	if len(events) != 1 || events[0].SampleCount != 3 || !events[0].EndTime.Equal(start.Add(40*time.Minute)) {
		t.Fatalf("20 分钟稳定采样应组成一个事件: %+v", events)
	}
}

func TestDetectContentionEventsIgnoresSingleSample(t *testing.T) {
	t.Parallel()

	events := detectContentionEvents("cpu_iowait", 30, []*storage.Metric{{Timestamp: time.Now(), Value: 30}})
	if len(events) != 0 {
		t.Fatalf("单个样本不应生成事件: %+v", events)
	}
}

func TestDetectContentionEventsDoesNotInferIntervalFromTwoSamples(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	events := detectContentionEvents("cpu_steal", 8, []*storage.Metric{
		{Timestamp: start, Value: 10},
		{Timestamp: start.Add(24 * time.Hour), Value: 12},
	})
	if len(events) != 0 {
		t.Fatalf("相隔 24 小时的两条样本不应生成事件: %+v", events)
	}
}

func TestDiskStatsDeviceRequiresCompleteSingleDevice(t *testing.T) {
	t.Parallel()

	metrics := []*storage.Metric{
		{Extra: map[string]interface{}{storage.ExtraDeviceName: "vda1"}},
		{Extra: map[string]interface{}{storage.ExtraDeviceName: "vdb1"}},
	}
	if name, status := diskStatsDevice(metrics); name != "" || status != "multiple" {
		t.Fatalf("多个设备应不可作为单设备: name=%q status=%q", name, status)
	}
	metrics[1].Extra = nil
	if name, status := diskStatsDevice(metrics); name != "" || status != "unknown" {
		t.Fatalf("缺少设备名应不可作为单设备: name=%q status=%q", name, status)
	}
}

func TestFindMissingMetricsMarksUnusableDiskStats(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"unknown", "multiple"} {
		stats := &PeriodStats{DiskStatsSamples: 2, DiskStatsDeviceStatus: status}
		if !containsString(findMissingMetrics(stats), "disk_stats") {
			t.Fatalf("设备状态 %s 时 disk_stats 应标记不可用", status)
		}
	}
	stats := &PeriodStats{DiskStatsSamples: 2, DiskBusyAvailable: true, DiskStatsDeviceStatus: "single"}
	if containsString(findMissingMetrics(stats), "disk_stats") {
		t.Fatal("单设备且已计算 busy 时 disk_stats 不应标记不可用")
	}
}

func TestCalculatePressurePercentsFromCumulativeTotal(t *testing.T) {
	t.Parallel()

	start := time.Now()
	metrics := []*storage.Metric{
		// avg10 快照为 0，但区间内累计等待 60 秒。
		{Timestamp: start, Value: 0, Extra: storage.NewPressureExtra(0, 0, 0, 1_000_000, 0, 0, 0, 0, false)},
		{Timestamp: start.Add(5 * time.Minute), Value: 0, Extra: storage.NewPressureExtra(0, 0, 0, 61_000_000, 0, 0, 0, 0, false)},
		// 计数器回退（重启）的区间应跳过。
		{Timestamp: start.Add(10 * time.Minute), Value: 0, Extra: storage.NewPressureExtra(0, 0, 0, 500, 0, 0, 0, 0, false)},
	}

	percents := calculatePressurePercents(metrics)
	if len(percents) != 1 {
		t.Fatalf("期望 1 个区间样本，实际=%v", percents)
	}
	if percents[0] != 20 {
		t.Fatalf("期望 60s/300s=20%%，实际=%.2f", percents[0])
	}
}

func TestCalculatePressurePercentsFallsBackToAvg10ForLegacySamples(t *testing.T) {
	t.Parallel()

	start := time.Now()
	metrics := []*storage.Metric{
		{Timestamp: start, Value: 12},
		{Timestamp: start.Add(5 * time.Minute), Value: 8},
	}

	percents := calculatePressurePercents(metrics)
	if len(percents) != 2 || percents[0] != 12 || percents[1] != 8 {
		t.Fatalf("缺少 total 的旧样本应回退到 avg10，实际=%v", percents)
	}
}
