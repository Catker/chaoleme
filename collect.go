package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Catker/chaoleme/collector"
	"github.com/Catker/chaoleme/storage"
)

var burstBatchSequence uint64

func saveMetricOrLog(store *storage.Storage, metric *storage.Metric) bool {
	if err := store.Save(metric); err != nil {
		log.Printf("保存指标失败 type=%s time=%s: %v", metric.Type, metric.Timestamp.Format(time.RFC3339), err)
		return false
	}
	return true
}

// collectAll 执行一次完整的常规数据采集。
func collectAll(cpu *collector.CPUCollector, disk *collector.DiskCollector, mem *collector.MemoryCollector, store *storage.Storage) (cpuObservation, bool) {
	now := time.Now()
	observation, collectedCPU := collectCoreMetrics(cpu, store, now)
	collectCPUBenchmark(cpu, store, now)
	collectIOMemoryDiskMetrics(disk, mem, store, now)
	return observation, collectedCPU
}

func parseCollectForOptions(durationText, intervalText, ioIntervalText string, defaultInterval, defaultIOInterval time.Duration) (time.Duration, time.Duration, time.Duration, error) {
	totalDuration, err := parsePositiveDuration("collect-for", durationText)
	if err != nil {
		return 0, 0, 0, err
	}

	sampleInterval := defaultInterval
	if intervalText != "" {
		sampleInterval, err = parsePositiveDuration("collect-interval", intervalText)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	if sampleInterval <= 0 {
		return 0, 0, 0, fmt.Errorf("collect-interval 必须大于 0")
	}

	ioInterval := defaultIOInterval
	if ioIntervalText != "" {
		ioInterval, err = parsePositiveDuration("collect-io-interval", ioIntervalText)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	if ioInterval <= 0 {
		return 0, 0, 0, fmt.Errorf("collect-io-interval 必须大于 0")
	}
	return totalDuration, sampleInterval, ioInterval, nil
}

func parsePositiveDuration(name, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s 格式无效: %w", name, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s 必须大于 0", name)
	}
	return duration, nil
}

func collectForDuration(cpu *collector.CPUCollector, disk *collector.DiskCollector, mem *collector.MemoryCollector, store *storage.Storage, totalDuration, sampleInterval, ioInterval time.Duration) {
	deadline := time.Now().Add(totalDuration)
	expectedSamples := estimateCollectForSamples(totalDuration, sampleInterval)
	expectedIOSamples := estimateCollectForSamples(totalDuration, ioInterval)
	log.Printf("连续采样启动: duration=%s core_interval=%s io_interval=%s expected_core_samples=%d expected_io_samples=%d end=%s",
		totalDuration, sampleInterval, ioInterval, expectedSamples, expectedIOSamples, deadline.Format(time.RFC3339))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	lastCoreSampleAt := time.Now()
	lastExtendedSampleAt := lastCoreSampleAt
	collectAll(cpu, disk, mem, store)

	coreTicker := time.NewTicker(sampleInterval)
	defer coreTicker.Stop()

	ioTicker := time.NewTicker(ioInterval)
	defer ioTicker.Stop()

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	for {
		select {
		case <-coreTicker.C:
			lastCoreSampleAt = time.Now()
			collectCoreMetrics(cpu, store, lastCoreSampleAt)
		case <-ioTicker.C:
			lastExtendedSampleAt = time.Now()
			collectExtendedMetrics(cpu, disk, mem, store, lastExtendedSampleAt)
		case <-timer.C:
			now := time.Now()
			if shouldTakeFinalSample(lastCoreSampleAt, now, sampleInterval) {
				collectCoreMetrics(cpu, store, now)
			}
			if shouldTakeFinalSample(lastExtendedSampleAt, now, ioInterval) {
				collectExtendedMetrics(cpu, disk, mem, store, now)
			}
			fmt.Println("✅ 连续采样完成")
			return
		case sig := <-sigCh:
			log.Printf("收到信号 %v，连续采样提前结束", sig)
			return
		}
	}
}

func shouldTakeFinalSample(lastSampleAt, now time.Time, sampleInterval time.Duration) bool {
	if sampleInterval <= 0 {
		return false
	}
	return now.Sub(lastSampleAt) >= sampleInterval
}

type cpuObservation struct {
	StealPercent  float64
	IOWaitPercent float64
}

// collectCoreMetrics 执行一次常规核心采样。
func collectCoreMetrics(cpu *collector.CPUCollector, store *storage.Storage, now time.Time) (cpuObservation, bool) {
	return collectCoreMetricsWithMode(cpu, store, now, storage.SamplingModeRegular, "")
}

func collectCoreMetricsWithMode(cpu *collector.CPUCollector, store *storage.Storage, now time.Time, samplingMode, batchID string) (cpuObservation, bool) {
	var observation cpuObservation
	collectedCPU := false
	if cpuUsage, err := cpu.Collect(); err == nil {
		observation = cpuObservation{StealPercent: cpuUsage.StealPercent, IOWaitPercent: cpuUsage.IOWaitPercent}
		collectedCPU = true
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUSteal,
			Value:     cpuUsage.StealPercent,
			Extra:     samplingExtra(nil, samplingMode, batchID),
		})
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUIoWait,
			Value:     cpuUsage.IOWaitPercent,
			Extra:     samplingExtra(nil, samplingMode, batchID),
		})
		log.Printf("CPU Steal: %.2f%%, IOWait: %.2f%%", cpuUsage.StealPercent, cpuUsage.IOWaitPercent)
	} else {
		log.Printf("CPU 数据采集失败: %v", err)
	}

	if loadResult, err := collector.CollectLoadAverage(); err == nil {
		numCPU := float64(runtime.NumCPU())
		normalizedLoad := loadResult.Load1 / numCPU
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPULoad,
			Value:     normalizedLoad,
			Extra:     samplingExtra(storage.NewLoadExtra(loadResult.Load1, loadResult.Load5, loadResult.Load15, numCPU), samplingMode, batchID),
		})
		log.Printf("CPU Load: %.2f (normalized: %.2f)", loadResult.Load1, normalizedLoad)
	} else {
		log.Printf("Load Average 采集失败: %v", err)
	}

	collectCPUPressureMetricWithMode(store, now, samplingMode, batchID)
	collectCPUThrottleMetricWithMode(store, now, samplingMode, batchID)
	return observation, collectedCPU
}

// collectBurstMetrics 写入同一秒的异常密集采样批次。
// CPU 采样失败不会影响后续辅助指标，也不会由此结束密集采样。
func collectBurstMetrics(cpu *collector.CPUCollector, store *storage.Storage, now time.Time) (cpuObservation, bool) {
	batchID := newBurstBatchID(now)
	observation, collectedCPU := collectCoreMetricsWithMode(cpu, store, now, storage.SamplingModeBurst, batchID)
	collectIOPressureMetricWithMode(store, now, storage.SamplingModeBurst, batchID)
	collectHostContextMetricWithMode(store, now, storage.SamplingModeBurst, batchID)
	return observation, collectedCPU
}

func newBurstBatchID(now time.Time) string {
	return fmt.Sprintf("%d-%d", now.UnixNano(), atomic.AddUint64(&burstBatchSequence, 1))
}

func samplingExtra(extra storage.Extra, samplingMode, batchID string) storage.Extra {
	if batchID != "" {
		return storage.WithSamplingModeAndBatchID(extra, samplingMode, batchID)
	}
	return storage.WithSamplingMode(extra, samplingMode)
}

func collectExtendedMetrics(cpu *collector.CPUCollector, disk *collector.DiskCollector, mem *collector.MemoryCollector, store *storage.Storage, now time.Time) {
	collectCPUBenchmark(cpu, store, now)
	collectIOMemoryDiskMetrics(disk, mem, store, now)
}

func collectCPUBenchmark(cpu *collector.CPUCollector, store *storage.Storage, now time.Time) {
	if result, err := cpu.RunBenchmark(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUBench,
			Value:     result.DurationMs,
			Extra:     storage.WithSamplingMode(nil, storage.SamplingModeRegular),
		})
		log.Printf("CPU Bench: %.2fms", result.DurationMs)
	} else {
		log.Printf("CPU 基准测试失败: %v", err)
	}
}

func collectIOMemoryDiskMetrics(disk *collector.DiskCollector, mem *collector.MemoryCollector, store *storage.Storage, now time.Time) {
	if result, err := disk.TestWriteLatency(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeIOLatency,
			Value:     result.TotalLatencyMs,
			Extra:     storage.WithSamplingMode(storage.NewIOLatencyExtra(result.WriteLatencyMs, result.SyncLatencyMs), storage.SamplingModeRegular),
		})
		log.Printf("I/O Latency: %.2fms", result.TotalLatencyMs)
	} else {
		log.Printf("I/O 延迟测试失败: %v", err)
	}

	if result, err := disk.TestRandomIO(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeRandomIO,
			Value:     result.RandomWriteLatencyMs,
			Extra: storage.WithSamplingMode(storage.NewRandomIODetailExtra(
				result.RandomWriteLatencyMs,
				result.RandomReadLatencyMs,
				result.DirectIOWrite,
				result.DirectIORead,
				storage.RandomIODetails{
					Method:        collector.RandomIOMethodPrefilled,
					DeviceCheck:   result.DeviceCheck,
					DeviceReadOps: result.DeviceReadOps,
					Reads:         result.Reads,
					Writes:        result.Writes,
					ReadP50MS:     result.ReadP50Ms,
					ReadP99MS:     result.ReadP99Ms,
					WriteP50MS:    result.WriteP50Ms,
					WriteP99MS:    result.WriteP99Ms,
				},
			), storage.SamplingModeRegular),
		})
		log.Printf("Random I/O: Write avg=%.2fms p99=%.2fms, Read avg=%.2fms p99=%.2fms, DirectIO=%t/%t, device=%s(%d/%d)",
			result.RandomWriteLatencyMs, result.WriteP99Ms, result.RandomReadLatencyMs, result.ReadP99Ms,
			result.DirectIOWrite, result.DirectIORead, result.DeviceCheck, result.DeviceReadOps, result.Reads)
	} else {
		log.Printf("随机 I/O 测试失败: %v", err)
	}

	if stats, err := mem.Collect(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeMemory,
			Value:     stats.UsagePercent(),
			Extra:     storage.WithSamplingMode(storage.NewMemoryExtra(stats.MemTotal, stats.MemAvailable, stats.AvailablePercent(), stats.SwapUsagePercent()), storage.SamplingModeRegular),
		})
		log.Printf("Memory Usage: %.1f%%, Available: %.1f%%", stats.UsagePercent(), stats.AvailablePercent())
	} else {
		log.Printf("内存采集失败: %v", err)
	}

	if diskStats, err := disk.CollectDiskStats(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeDiskStats,
			Value:     float64(diskStats.IOTimeMs),
			Extra: storage.WithSamplingMode(storage.NewDiskStatsExtra(
				diskStats.DeviceName,
				diskStats.ReadOps,
				diskStats.WriteOps,
				diskStats.ReadBytes,
				diskStats.WriteBytes,
				diskStats.IOTimeMs,
				diskStats.WeightedIOMs,
			), storage.SamplingModeRegular),
		})
		log.Printf("Disk Stats: ReadOps=%d, WriteOps=%d, IOTime=%dms", diskStats.ReadOps, diskStats.WriteOps, diskStats.IOTimeMs)
	} else {
		log.Printf("磁盘统计采集失败: %v", err)
	}

	collectIOPressureMetricWithMode(store, now, storage.SamplingModeRegular, "")
	collectHostContextMetricWithMode(store, now, storage.SamplingModeRegular, "")
}

func estimateCollectForSamples(totalDuration, sampleInterval time.Duration) int {
	if totalDuration <= 0 || sampleInterval <= 0 {
		return 0
	}
	// collect-for 会在启动时采一次，并按完整间隔补足结束样本。
	return int(totalDuration/sampleInterval) + 1
}

func collectHostContextMetric(store *storage.Storage, now time.Time) {
	collectHostContextMetricWithMode(store, now, storage.SamplingModeRegular, "")
}

func collectHostContextMetricWithMode(store *storage.Storage, now time.Time, samplingMode, batchID string) {
	ctx := collector.CollectHostContext()
	value := 0.0
	if ctx.HypervisorDetected {
		value = 1.0
	}
	saveMetricOrLog(store, &storage.Metric{
		Timestamp: now,
		Type:      storage.MetricTypeHostContext,
		Value:     value,
		Extra:     samplingExtra(storage.NewHostContextExtra(ctx.HypervisorDetected, ctx.ContainerDetected, ctx.VirtualizationType, ctx.StealDirectlyInterpretable), samplingMode, batchID),
	})
	log.Printf("Host Context: virt=%s hypervisor=%t container=%t", ctx.VirtualizationType, ctx.HypervisorDetected, ctx.ContainerDetected)
}

func collectCPUThrottleMetric(store *storage.Storage, now time.Time) {
	collectCPUThrottleMetricWithMode(store, now, storage.SamplingModeRegular, "")
}

func collectCPUThrottleMetricWithMode(store *storage.Storage, now time.Time, samplingMode, batchID string) {
	if throttle, err := collector.CollectCPUThrottle(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUThrottle,
			Value:     throttle.ThrottledPercent(),
			Extra:     samplingExtra(storage.NewCPUThrottleExtra(throttle.Periods, throttle.ThrottledPeriods, throttle.ThrottledUsec), samplingMode, batchID),
		})
		log.Printf("CPU Throttle: %.2f%%", throttle.ThrottledPercent())
	} else {
		log.Printf("CPU Throttle 采集失败: %v", err)
	}
}

func collectPressureMetrics(store *storage.Storage, now time.Time) {
	collectCPUPressureMetric(store, now)
	collectIOPressureMetric(store, now)
}

func collectCPUPressureMetric(store *storage.Storage, now time.Time) {
	collectCPUPressureMetricWithMode(store, now, storage.SamplingModeRegular, "")
}

func collectCPUPressureMetricWithMode(store *storage.Storage, now time.Time, samplingMode, batchID string) {
	if pressure, err := collector.CollectCPUPressure(); err == nil {
		savePressureMetric(store, now, storage.MetricTypeCPUPressure, pressure, samplingMode, batchID)
		log.Printf("CPU Pressure PSI some avg10=%.2f avg60=%.2f", pressure.SomeAvg10, pressure.SomeAvg60)
	} else {
		log.Printf("CPU Pressure 采集失败: %v", err)
	}
}

func collectIOPressureMetric(store *storage.Storage, now time.Time) {
	collectIOPressureMetricWithMode(store, now, storage.SamplingModeRegular, "")
}

func collectIOPressureMetricWithMode(store *storage.Storage, now time.Time, samplingMode, batchID string) {
	if pressure, err := collector.CollectIOPressure(); err == nil {
		savePressureMetric(store, now, storage.MetricTypeIOPressure, pressure, samplingMode, batchID)
		log.Printf("IO Pressure PSI some avg10=%.2f avg60=%.2f", pressure.SomeAvg10, pressure.SomeAvg60)
	} else {
		log.Printf("IO Pressure 采集失败: %v", err)
	}
}

func savePressureMetric(store *storage.Storage, now time.Time, metricType storage.MetricType, pressure *collector.PressureResult, samplingMode, batchID string) {
	saveMetricOrLog(store, &storage.Metric{
		Timestamp: now,
		Type:      metricType,
		Value:     pressure.SomeAvg10,
		Extra: samplingExtra(storage.NewPressureExtra(
			pressure.SomeAvg10,
			pressure.SomeAvg60,
			pressure.SomeAvg300,
			pressure.SomeTotal,
			pressure.FullAvg10,
			pressure.FullAvg60,
			pressure.FullAvg300,
			pressure.FullTotal,
			pressure.HasFull,
		), samplingMode, batchID),
	})
}
