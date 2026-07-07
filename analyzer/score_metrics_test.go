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
