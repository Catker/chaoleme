package analyzer

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Catker/chaoleme/storage"
)

const (
	contentionStealThreshold  = 8.0
	contentionIOWaitThreshold = 30.0
)

// correlateContentionEvents 只使用具备相同 burst_batch_id 的完整批次解释异常事件。
// 这些说明用于事件展示，不参与既有超售结论。
func correlateContentionEvents(events []ContentionEvent, steal, iowait, load, cpuPressure, ioPressure, throttle, hostContext []*storage.Metric) []ContentionEvent {
	batches := makeBurstBatches(steal, iowait, load, cpuPressure, ioPressure, throttle, hostContext)
	for i := range events {
		if events[i].Type == string(storage.MetricTypeCPUSteal) {
			correlateCPUStealEvent(&events[i], batches)
			continue
		}
		if events[i].Type == string(storage.MetricTypeCPUIoWait) {
			events[i].SynchronizedSamples = batches.synchronizedCount(events[i], storage.MetricTypeCPUIoWait)
			events[i].Correlation = "持续 I/O wait，来源未知"
		}
	}
	return events
}

type burstBatch struct {
	id          string
	unixNano    int64
	sequence    uint64
	orderValid  bool
	steal       *storage.Metric
	iowait      *storage.Metric
	load        *storage.Metric
	cpuPressure *storage.Metric
	ioPressure  *storage.Metric
	throttle    *storage.Metric
	hostContext *storage.Metric
}

type burstBatches map[string]*burstBatch

func makeBurstBatches(steal, iowait, load, cpuPressure, ioPressure, throttle, hostContext []*storage.Metric) burstBatches {
	batches := make(burstBatches)
	add := func(metrics []*storage.Metric, setter func(*burstBatch, *storage.Metric)) {
		for _, metric := range metrics {
			batchID, ok := burstBatchID(metric)
			if !ok {
				continue
			}
			batch := batches[batchID]
			if batch == nil {
				unixNano, sequence, orderValid := parseBurstBatchOrder(batchID)
				batch = &burstBatch{id: batchID, unixNano: unixNano, sequence: sequence, orderValid: orderValid}
				batches[batchID] = batch
			}
			setter(batch, metric)
		}
	}
	add(steal, func(batch *burstBatch, metric *storage.Metric) { batch.steal = metric })
	add(iowait, func(batch *burstBatch, metric *storage.Metric) { batch.iowait = metric })
	add(load, func(batch *burstBatch, metric *storage.Metric) { batch.load = metric })
	add(cpuPressure, func(batch *burstBatch, metric *storage.Metric) { batch.cpuPressure = metric })
	add(ioPressure, func(batch *burstBatch, metric *storage.Metric) { batch.ioPressure = metric })
	add(throttle, func(batch *burstBatch, metric *storage.Metric) { batch.throttle = metric })
	add(hostContext, func(batch *burstBatch, metric *storage.Metric) { batch.hostContext = metric })
	return batches
}

func parseBurstBatchOrder(batchID string) (int64, uint64, bool) {
	separator := strings.LastIndexByte(batchID, '-')
	if separator <= 0 || separator == len(batchID)-1 {
		return 0, 0, false
	}
	unixNano, err := strconv.ParseInt(batchID[:separator], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	sequence, err := strconv.ParseUint(batchID[separator+1:], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return unixNano, sequence, true
}

func burstBatchID(metric *storage.Metric) (string, bool) {
	if !isBurstMetric(metric) {
		return "", false
	}
	batchID, ok := extraString(metric, storage.ExtraBurstBatchID)
	return batchID, ok && batchID != ""
}

func (b burstBatches) synchronizedCount(event ContentionEvent, primary storage.MetricType) int {
	count := 0
	for _, batch := range b {
		if batch.primary(primary) != nil && batch.completeWithin(event) {
			count++
		}
	}
	return count
}

func (b *burstBatch) primary(metricType storage.MetricType) *storage.Metric {
	if metricType == storage.MetricTypeCPUSteal {
		return b.steal
	}
	return b.iowait
}

func (b *burstBatch) completeWithin(event ContentionEvent) bool {
	metrics := []*storage.Metric{b.steal, b.iowait, b.load, b.cpuPressure, b.ioPressure, b.throttle, b.hostContext}
	for _, metric := range metrics {
		if metric == nil || !eventContainsTime(event, metric.Timestamp) {
			return false
		}
	}
	return true
}

func correlateCPUStealEvent(event *ContentionEvent, batches burstBatches) {
	event.SynchronizedSamples = batches.synchronizedCount(*event, storage.MetricTypeCPUSteal)
	var matched []*burstBatch
	for _, batch := range batches {
		if batch.steal != nil && batch.completeWithin(*event) {
			matched = append(matched, batch)
		}
	}
	if len(matched) == 0 {
		event.Correlation = "环境数据不足"
		return
	}
	for _, batch := range matched {
		if !batch.orderValid {
			event.Correlation = "节流数据不足"
			return
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].unixNano == matched[j].unixNano {
			return matched[i].sequence < matched[j].sequence
		}
		return matched[i].unixNano < matched[j].unixNano
	})

	throttleMetrics := make([]*storage.Metric, 0, len(matched))
	for _, batch := range matched {
		direct, ok := extraBool(batch.hostContext, storage.ExtraStealDirectlyInterpretable)
		if !ok || !direct {
			event.Correlation = "环境数据不足"
			return
		}
		throttleMetrics = append(throttleMetrics, batch.throttle)
	}

	throttlePercent, ok := throttlePercentForEvent(throttleMetrics)
	if !ok {
		event.Correlation = "节流数据不足"
		return
	}
	if throttlePercent < 20 {
		event.Correlation = "符合宿主机 CPU 争抢特征"
		return
	}
	event.Correlation = "同时有本机限额，不能单独归因"
}

func throttlePercentForEvent(metrics []*storage.Metric) (float64, bool) {
	if len(metrics) < 2 {
		return 0, false
	}
	for i := 1; i < len(metrics); i++ {
		prevPeriods, okPrevPeriods := extraFloat(metrics[i-1], storage.ExtraPeriods)
		currPeriods, okCurrPeriods := extraFloat(metrics[i], storage.ExtraPeriods)
		prevThrottled, okPrevThrottled := extraFloat(metrics[i-1], storage.ExtraThrottledPeriods)
		currThrottled, okCurrThrottled := extraFloat(metrics[i], storage.ExtraThrottledPeriods)
		if !okPrevPeriods || !okCurrPeriods || !okPrevThrottled || !okCurrThrottled {
			return 0, false
		}
		if currPeriods <= prevPeriods || currThrottled < prevThrottled {
			return 0, false
		}
	}
	firstPeriods, _ := extraFloat(metrics[0], storage.ExtraPeriods)
	lastPeriods, _ := extraFloat(metrics[len(metrics)-1], storage.ExtraPeriods)
	firstThrottled, _ := extraFloat(metrics[0], storage.ExtraThrottledPeriods)
	lastThrottled, _ := extraFloat(metrics[len(metrics)-1], storage.ExtraThrottledPeriods)
	return clampPercent((lastThrottled - firstThrottled) / (lastPeriods - firstPeriods) * 100), true
}

func eventContainsTime(event ContentionEvent, timestamp time.Time) bool {
	return !timestamp.Before(event.StartTime) && !timestamp.After(event.EndTime)
}
