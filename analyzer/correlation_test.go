package analyzer

import (
	"fmt"
	"testing"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func TestCorrelateCPUStealEventRequiresDirectContextAndThrottleDelta(t *testing.T) {
	start := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	ids := []string{testBurstBatchID(1), testBurstBatchID(2)}
	metrics := burstMetricSeries(start, ids, storage.MetricTypeCPUSteal, 10, nil)
	iowait := burstMetricSeries(start, ids, storage.MetricTypeCPUIoWait, 1, nil)
	load := burstMetricSeries(start, ids, storage.MetricTypeCPULoad, 0.2, nil)
	cpuPSI := burstMetricSeries(start, ids, storage.MetricTypeCPUPressure, 1, nil)
	ioPSI := burstMetricSeries(start, ids, storage.MetricTypeIOPressure, 1, nil)
	context := burstMetricSeries(start, ids, storage.MetricTypeHostContext, 1, storage.NewHostContextExtra(true, false, "kvm", true))
	throttle := []*storage.Metric{
		{Timestamp: start, Extra: burstExtra(storage.NewCPUThrottleExtra(100, 5, 0), ids[0])},
		{Timestamp: start.Add(time.Second), Extra: burstExtra(storage.NewCPUThrottleExtra(200, 15, 0), ids[1])},
	}
	events := correlateContentionEvents([]ContentionEvent{{Type: string(storage.MetricTypeCPUSteal), StartTime: start, EndTime: start.Add(time.Second)}}, metrics, iowait, load, cpuPSI, ioPSI, throttle, context)
	if events[0].SynchronizedSamples != 2 || events[0].Correlation != "符合宿主机 CPU 争抢特征" {
		t.Fatalf("CPU 事件关联不符合预期: %+v", events[0])
	}
	context[1].Extra = burstExtra(storage.NewHostContextExtra(true, false, "kvm", false), ids[1])
	events = correlateContentionEvents(events, metrics, iowait, load, cpuPSI, ioPSI, throttle, context)
	if events[0].Correlation != "环境数据不足" {
		t.Fatalf("缺少直接环境上下文时应拒绝归因: %+v", events[0])
	}
}

func TestCorrelateCPUStealEventRejectsMissingOrResetThrottleCounters(t *testing.T) {
	start := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	ids := []string{testBurstBatchID(1), testBurstBatchID(2)}
	steal := burstMetricSeries(start, ids, storage.MetricTypeCPUSteal, 10, nil)
	iowait := burstMetricSeries(start, ids, storage.MetricTypeCPUIoWait, 1, nil)
	load := burstMetricSeries(start, ids, storage.MetricTypeCPULoad, 0.2, nil)
	cpuPSI := burstMetricSeries(start, ids, storage.MetricTypeCPUPressure, 1, nil)
	ioPSI := burstMetricSeries(start, ids, storage.MetricTypeIOPressure, 1, nil)
	context := burstMetricSeries(start, ids, storage.MetricTypeHostContext, 1, storage.NewHostContextExtra(true, false, "kvm", true))
	event := []ContentionEvent{{Type: string(storage.MetricTypeCPUSteal), StartTime: start, EndTime: start.Add(time.Second)}}

	missing := []*storage.Metric{{Timestamp: start, Extra: burstExtra(storage.Extra{}, ids[0])}, {Timestamp: start.Add(time.Second), Extra: burstExtra(storage.Extra{}, ids[1])}}
	events := correlateContentionEvents(event, steal, iowait, load, cpuPSI, ioPSI, missing, context)
	if events[0].Correlation != "节流数据不足" {
		t.Fatalf("缺少节流计数器应拒绝归因: %+v", events[0])
	}

	reset := []*storage.Metric{
		{Timestamp: start, Extra: burstExtra(storage.NewCPUThrottleExtra(100, 10, 0), ids[0])},
		{Timestamp: start.Add(time.Second), Extra: burstExtra(storage.NewCPUThrottleExtra(50, 5, 0), ids[1])},
	}
	events = correlateContentionEvents(event, steal, iowait, load, cpuPSI, ioPSI, reset, context)
	if events[0].Correlation != "节流数据不足" {
		t.Fatalf("节流计数器回退应拒绝归因: %+v", events[0])
	}
}

func TestCorrelateIOWaitEventLeavesSourceUnknown(t *testing.T) {
	start := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	ids := []string{testBurstBatchID(1), testBurstBatchID(2)}
	events := correlateContentionEvents(
		[]ContentionEvent{{Type: string(storage.MetricTypeCPUIoWait), StartTime: start, EndTime: start.Add(time.Second)}},
		burstMetricSeries(start, ids, storage.MetricTypeCPUSteal, 1, nil), burstMetricSeries(start, ids, storage.MetricTypeCPUIoWait, 35, nil), burstMetricSeries(start, ids, storage.MetricTypeCPULoad, 0.2, nil),
		burstMetricSeries(start, ids, storage.MetricTypeCPUPressure, 1, nil), burstMetricSeries(start, ids, storage.MetricTypeIOPressure, 10, nil),
		[]*storage.Metric{{Timestamp: start, Extra: burstExtra(storage.NewCPUThrottleExtra(100, 1, 0), ids[0])}, {Timestamp: start.Add(time.Second), Extra: burstExtra(storage.NewCPUThrottleExtra(200, 2, 0), ids[1])}},
		burstMetricSeries(start, ids, storage.MetricTypeHostContext, 1, storage.NewHostContextExtra(true, false, "kvm", true)),
	)
	if events[0].SynchronizedSamples != 2 || events[0].Correlation != "持续 I/O wait，来源未知" {
		t.Fatalf("IOWait 事件说明不符合预期: %+v", events[0])
	}
}

func TestCorrelateDoesNotCrossBatchIDsWithinSameSecond(t *testing.T) {
	start := time.Date(2026, 7, 10, 12, 0, 0, 100_000_000, time.UTC)
	event := []ContentionEvent{{Type: string(storage.MetricTypeCPUSteal), StartTime: start, EndTime: start.Add(100 * time.Millisecond)}}
	// A 批次缺少环境上下文；B 批次只有上下文。两者在同一 Unix 秒内，不能混合。
	idsA := []string{testBurstBatchID(1), testBurstBatchID(2)}
	steal := burstMetricSeries(start, idsA, storage.MetricTypeCPUSteal, 10, nil)
	iowait := burstMetricSeries(start, idsA, storage.MetricTypeCPUIoWait, 1, nil)
	load := burstMetricSeries(start, idsA, storage.MetricTypeCPULoad, 0.2, nil)
	cpuPSI := burstMetricSeries(start, idsA, storage.MetricTypeCPUPressure, 1, nil)
	ioPSI := burstMetricSeries(start, idsA, storage.MetricTypeIOPressure, 1, nil)
	throttle := []*storage.Metric{
		{Timestamp: start, Extra: burstExtra(storage.NewCPUThrottleExtra(100, 1, 0), idsA[0])},
		{Timestamp: start.Add(50 * time.Millisecond), Extra: burstExtra(storage.NewCPUThrottleExtra(200, 2, 0), idsA[1])},
	}
	context := []*storage.Metric{
		{Timestamp: start, Extra: burstExtra(storage.NewHostContextExtra(true, false, "kvm", true), testBurstBatchID(101))},
		{Timestamp: start.Add(50 * time.Millisecond), Extra: burstExtra(storage.NewHostContextExtra(true, false, "kvm", true), testBurstBatchID(102))},
	}
	events := correlateContentionEvents(event, steal, iowait, load, cpuPSI, ioPSI, throttle, context)
	if events[0].SynchronizedSamples != 0 || events[0].Correlation != "环境数据不足" {
		t.Fatalf("不同批次 ID 不应交叉关联: %+v", events[0])
	}
}

func TestCorrelateSortsThrottleByBatchIDAfterSQLiteSecondTruncation(t *testing.T) {
	store := newTestStore(t)
	start := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	firstID := fmt.Sprintf("%d-1", start.UnixNano())
	secondID := fmt.Sprintf("%d-2", start.Add(100*time.Millisecond).UnixNano())

	// 反序写入，SQLite 查询后两批时间都会是同一秒。
	writeSQLiteBurstBatch(t, store, start.Add(100*time.Millisecond), secondID, 200, 15)
	writeSQLiteBurstBatch(t, store, start, firstID, 100, 5)
	query := func(metricType storage.MetricType) []*storage.Metric {
		metrics, err := store.Query(metricType, start.Add(-time.Second), start.Add(time.Second))
		if err != nil {
			t.Fatalf("查询 %s 失败: %v", metricType, err)
		}
		return metrics
	}
	secondStart := start.Truncate(time.Second)
	events := correlateContentionEvents(
		[]ContentionEvent{{Type: string(storage.MetricTypeCPUSteal), StartTime: secondStart, EndTime: secondStart}},
		query(storage.MetricTypeCPUSteal), query(storage.MetricTypeCPUIoWait), query(storage.MetricTypeCPULoad),
		query(storage.MetricTypeCPUPressure), query(storage.MetricTypeIOPressure), query(storage.MetricTypeCPUThrottle), query(storage.MetricTypeHostContext),
	)
	if events[0].SynchronizedSamples != 2 || events[0].Correlation != "符合宿主机 CPU 争抢特征" {
		t.Fatalf("秒级持久化后应按 batch ID 顺序计算节流增量: %+v", events[0])
	}
}

func writeSQLiteBurstBatch(t *testing.T, store *storage.Storage, timestamp time.Time, batchID string, periods, throttled uint64) {
	t.Helper()
	mode := storage.SamplingModeBurst
	saveMetric(t, store, timestamp, storage.MetricTypeCPUSteal, 10, storage.WithSamplingModeAndBatchID(nil, mode, batchID))
	saveMetric(t, store, timestamp, storage.MetricTypeCPUIoWait, 1, storage.WithSamplingModeAndBatchID(nil, mode, batchID))
	saveMetric(t, store, timestamp, storage.MetricTypeCPULoad, 0.2, storage.WithSamplingModeAndBatchID(nil, mode, batchID))
	saveMetric(t, store, timestamp, storage.MetricTypeCPUPressure, 1, storage.WithSamplingModeAndBatchID(nil, mode, batchID))
	saveMetric(t, store, timestamp, storage.MetricTypeIOPressure, 1, storage.WithSamplingModeAndBatchID(nil, mode, batchID))
	saveMetric(t, store, timestamp, storage.MetricTypeCPUThrottle, 0, storage.WithSamplingModeAndBatchID(storage.NewCPUThrottleExtra(periods, throttled, 0), mode, batchID))
	saveMetric(t, store, timestamp, storage.MetricTypeHostContext, 1, storage.WithSamplingModeAndBatchID(storage.NewHostContextExtra(true, false, "kvm", true), mode, batchID))
}

func burstMetricSeries(start time.Time, ids []string, metricType storage.MetricType, value float64, extra storage.Extra) []*storage.Metric {
	metrics := make([]*storage.Metric, 0, len(ids))
	for index, id := range ids {
		metrics = append(metrics, &storage.Metric{Type: metricType, Timestamp: start.Add(time.Duration(index) * time.Second), Value: value, Extra: burstExtra(extra, id)})
	}
	return metrics
}

func burstExtra(extra storage.Extra, batchID string) storage.Extra {
	return storage.WithSamplingModeAndBatchID(extra, storage.SamplingModeBurst, batchID)
}

func testBurstBatchID(sequence int) string {
	return fmt.Sprintf("1760000000000000000-%d", sequence)
}
