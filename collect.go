package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Catker/chaoleme/collector"
	"github.com/Catker/chaoleme/storage"
)

func saveMetricOrLog(store *storage.Storage, metric *storage.Metric) bool {
	if err := store.Save(metric); err != nil {
		log.Printf("保存指标失败 type=%s time=%s: %v", metric.Type, metric.Timestamp.Format(time.RFC3339), err)
		return false
	}
	return true
}

// collectAll 执行一次完整的数据采集

func collectAll(cpu *collector.CPUCollector, disk *collector.DiskCollector, mem *collector.MemoryCollector, store *storage.Storage) {
	now := time.Now()

	// CPU Usage (Steal & IOWait)
	if cpuUsage, err := cpu.Collect(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUSteal,
			Value:     cpuUsage.StealPercent,
		})
		log.Printf("CPU Steal: %.2f%%", cpuUsage.StealPercent)

		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUIoWait,
			Value:     cpuUsage.IOWaitPercent,
		})
		log.Printf("CPU IOWait: %.2f%%", cpuUsage.IOWaitPercent)
	} else {
		log.Printf("CPU 数据采集失败: %v", err)
	}

	// CPU 基准测试
	if result, err := cpu.RunBenchmark(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUBench,
			Value:     result.DurationMs,
		})
		log.Printf("CPU Bench: %.2fms", result.DurationMs)
	} else {
		log.Printf("CPU 基准测试失败: %v", err)
	}

	// I/O 顺序延迟
	if result, err := disk.TestWriteLatency(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeIOLatency,
			Value:     result.TotalLatencyMs,
			Extra:     storage.NewIOLatencyExtra(result.WriteLatencyMs, result.SyncLatencyMs),
		})
		log.Printf("I/O Latency: %.2fms", result.TotalLatencyMs)
	} else {
		log.Printf("I/O 延迟测试失败: %v", err)
	}

	// I/O 随机读写
	if result, err := disk.TestRandomIO(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeRandomIO,
			Value:     result.RandomWriteLatencyMs, // 主值使用写延迟
			Extra:     storage.NewRandomIOExtra(result.RandomWriteLatencyMs, result.RandomReadLatencyMs, result.DirectIOWrite, result.DirectIORead),
		})
		log.Printf("Random I/O: Write=%.2fms, Read=%.2fms, DirectIO=%t/%t",
			result.RandomWriteLatencyMs, result.RandomReadLatencyMs, result.DirectIOWrite, result.DirectIORead)
	} else {
		log.Printf("随机 I/O 测试失败: %v", err)
	}

	// 内存
	if stats, err := mem.Collect(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeMemory,
			Value:     stats.UsagePercent(),
			Extra:     storage.NewMemoryExtra(stats.MemTotal, stats.MemAvailable, stats.AvailablePercent(), stats.SwapUsagePercent()),
		})
		log.Printf("Memory Usage: %.1f%%, Available: %.1f%%", stats.UsagePercent(), stats.AvailablePercent())
	} else {
		log.Printf("内存采集失败: %v", err)
	}

	// DiskStats 磁盘统计（从 /proc/diskstats 采集，开销极低）
	if diskStats, err := disk.CollectDiskStats(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeDiskStats,
			Value:     float64(diskStats.IOTimeMs), // 主值使用累计 IO 耗时
			Extra: storage.NewDiskStatsExtra(
				diskStats.ReadOps,
				diskStats.WriteOps,
				diskStats.ReadBytes,
				diskStats.WriteBytes,
				diskStats.IOTimeMs,
				diskStats.WeightedIOMs,
			),
		})
		log.Printf("Disk Stats: ReadOps=%d, WriteOps=%d, IOTime=%dms", diskStats.ReadOps, diskStats.WriteOps, diskStats.IOTimeMs)
	} else {
		log.Printf("磁盘统计采集失败: %v", err)
	}

	// Load Average
	if loadResult, err := collector.CollectLoadAverage(); err == nil {
		numCPU := float64(runtime.NumCPU())
		normalizedLoad := loadResult.Load1 / numCPU
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPULoad,
			Value:     normalizedLoad,
			Extra:     storage.NewLoadExtra(loadResult.Load1, loadResult.Load5, loadResult.Load15, numCPU),
		})
		log.Printf("CPU Load: %.2f (normalized: %.2f)", loadResult.Load1, normalizedLoad)
	} else {
		log.Printf("Load Average 采集失败: %v", err)
	}

	collectPressureMetrics(store, now)
	collectCPUThrottleMetric(store, now)
	collectHostContextMetric(store, now)
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

func collectCoreMetrics(cpu *collector.CPUCollector, store *storage.Storage, now time.Time) {
	if cpuUsage, err := cpu.Collect(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUSteal,
			Value:     cpuUsage.StealPercent,
		})
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUIoWait,
			Value:     cpuUsage.IOWaitPercent,
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
			Extra:     storage.NewLoadExtra(loadResult.Load1, loadResult.Load5, loadResult.Load15, numCPU),
		})
		log.Printf("CPU Load: %.2f (normalized: %.2f)", loadResult.Load1, normalizedLoad)
	} else {
		log.Printf("Load Average 采集失败: %v", err)
	}

	collectCPUPressureMetric(store, now)
	collectCPUThrottleMetric(store, now)
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
			Extra:     storage.NewIOLatencyExtra(result.WriteLatencyMs, result.SyncLatencyMs),
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
			Extra:     storage.NewRandomIOExtra(result.RandomWriteLatencyMs, result.RandomReadLatencyMs, result.DirectIOWrite, result.DirectIORead),
		})
		log.Printf("Random I/O: Write=%.2fms, Read=%.2fms, DirectIO=%t/%t",
			result.RandomWriteLatencyMs, result.RandomReadLatencyMs, result.DirectIOWrite, result.DirectIORead)
	} else {
		log.Printf("随机 I/O 测试失败: %v", err)
	}

	if stats, err := mem.Collect(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeMemory,
			Value:     stats.UsagePercent(),
			Extra:     storage.NewMemoryExtra(stats.MemTotal, stats.MemAvailable, stats.AvailablePercent(), stats.SwapUsagePercent()),
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
			Extra: storage.NewDiskStatsExtra(
				diskStats.ReadOps,
				diskStats.WriteOps,
				diskStats.ReadBytes,
				diskStats.WriteBytes,
				diskStats.IOTimeMs,
				diskStats.WeightedIOMs,
			),
		})
		log.Printf("Disk Stats: ReadOps=%d, WriteOps=%d, IOTime=%dms", diskStats.ReadOps, diskStats.WriteOps, diskStats.IOTimeMs)
	} else {
		log.Printf("磁盘统计采集失败: %v", err)
	}

	collectIOPressureMetric(store, now)
	collectHostContextMetric(store, now)
}

func estimateCollectForSamples(totalDuration, sampleInterval time.Duration) int {
	if totalDuration <= 0 || sampleInterval <= 0 {
		return 0
	}
	// collect-for 会在启动时采一次，并按完整间隔补足结束样本。
	return int(totalDuration/sampleInterval) + 1
}

func collectHostContextMetric(store *storage.Storage, now time.Time) {
	ctx := collector.CollectHostContext()
	value := 0.0
	if ctx.HypervisorDetected {
		value = 1.0
	}
	saveMetricOrLog(store, &storage.Metric{
		Timestamp: now,
		Type:      storage.MetricTypeHostContext,
		Value:     value,
		Extra:     storage.NewHostContextExtra(ctx.HypervisorDetected, ctx.ContainerDetected, ctx.VirtualizationType, ctx.StealDirectlyInterpretable),
	})
	log.Printf("Host Context: virt=%s hypervisor=%t container=%t", ctx.VirtualizationType, ctx.HypervisorDetected, ctx.ContainerDetected)
}

func collectCPUThrottleMetric(store *storage.Storage, now time.Time) {
	if throttle, err := collector.CollectCPUThrottle(); err == nil {
		saveMetricOrLog(store, &storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUThrottle,
			Value:     throttle.ThrottledPercent(),
			Extra:     storage.NewCPUThrottleExtra(throttle.Periods, throttle.ThrottledPeriods, throttle.ThrottledUsec),
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
	if pressure, err := collector.CollectCPUPressure(); err == nil {
		savePressureMetric(store, now, storage.MetricTypeCPUPressure, pressure)
		log.Printf("CPU Pressure PSI some avg10=%.2f avg60=%.2f", pressure.SomeAvg10, pressure.SomeAvg60)
	} else {
		log.Printf("CPU Pressure 采集失败: %v", err)
	}
}

func collectIOPressureMetric(store *storage.Storage, now time.Time) {
	if pressure, err := collector.CollectIOPressure(); err == nil {
		savePressureMetric(store, now, storage.MetricTypeIOPressure, pressure)
		log.Printf("IO Pressure PSI some avg10=%.2f avg60=%.2f", pressure.SomeAvg10, pressure.SomeAvg60)
	} else {
		log.Printf("IO Pressure 采集失败: %v", err)
	}
}

func savePressureMetric(store *storage.Storage, now time.Time, metricType storage.MetricType, pressure *collector.PressureResult) {
	saveMetricOrLog(store, &storage.Metric{
		Timestamp: now,
		Type:      metricType,
		Value:     pressure.SomeAvg10,
		Extra: storage.NewPressureExtra(
			pressure.SomeAvg10,
			pressure.SomeAvg60,
			pressure.SomeAvg300,
			pressure.SomeTotal,
			pressure.FullAvg10,
			pressure.FullAvg60,
			pressure.FullAvg300,
			pressure.FullTotal,
			pressure.HasFull,
		),
	})
}
