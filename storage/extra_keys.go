package storage

type Extra map[string]interface{}

const (
	// ExtraSamplingMode 标识采样频率。缺失该字段的历史样本按 regular 处理。
	ExtraSamplingMode   = "sampling_mode"
	ExtraBurstBatchID   = "burst_batch_id"
	SamplingModeRegular = "regular"
	SamplingModeBurst   = "burst"

	ExtraWriteLatencyMS = "write_latency_ms"
	ExtraSyncLatencyMS  = "sync_latency_ms"
	ExtraReadLatencyMS  = "read_latency_ms"
	ExtraDirectIOWrite  = "direct_io_write"
	ExtraDirectIORead   = "direct_io_read"

	// 随机 I/O 测试方法与落盘验证。旧样本缺少这些字段，存在稀疏文件空洞读问题。
	ExtraRandomIOMethod        = "random_io_method"
	ExtraRandomIODeviceCheck   = "device_check"
	ExtraRandomIODeviceReadOps = "device_read_ops"
	ExtraRandomIOReads         = "random_reads"
	ExtraRandomIOWrites        = "random_writes"
	ExtraReadP50MS             = "read_p50_ms"
	ExtraReadP99MS             = "read_p99_ms"
	ExtraWriteP50MS            = "write_p50_ms"
	ExtraWriteP99MS            = "write_p99_ms"

	ExtraTotalKB          = "total_kb"
	ExtraAvailableKB      = "available_kb"
	ExtraAvailablePercent = "available_percent"
	ExtraSwapUsage        = "swap_usage"

	ExtraReadOps      = "read_ops"
	ExtraWriteOps     = "write_ops"
	ExtraReadBytes    = "read_bytes"
	ExtraWriteBytes   = "write_bytes"
	ExtraIOTimeMS     = "io_time_ms"
	ExtraWeightedIOMS = "weighted_io_ms"
	ExtraDeviceName   = "device_name"

	ExtraLoad1  = "load1"
	ExtraLoad5  = "load5"
	ExtraLoad15 = "load15"
	ExtraNumCPU = "num_cpu"

	ExtraHypervisorDetected         = "hypervisor_detected"
	ExtraContainerDetected          = "container_detected"
	ExtraVirtualizationType         = "virtualization_type"
	ExtraStealDirectlyInterpretable = "steal_directly_interpretable"

	ExtraPeriods          = "periods"
	ExtraThrottledPeriods = "throttled_periods"
	ExtraThrottledUsec    = "throttled_usec"

	ExtraSomeAvg10  = "some_avg10"
	ExtraSomeAvg60  = "some_avg60"
	ExtraSomeAvg300 = "some_avg300"
	ExtraSomeTotal  = "some_total"
	ExtraFullAvg10  = "full_avg10"
	ExtraFullAvg60  = "full_avg60"
	ExtraFullAvg300 = "full_avg300"
	ExtraFullTotal  = "full_total"
	ExtraHasFull    = "has_full"
)

// WithSamplingMode 为指标附加采样模式，不修改传入的 Extra。
func WithSamplingMode(extra Extra, mode string) Extra {
	result := make(Extra, len(extra)+1)
	for key, value := range extra {
		result[key] = value
	}
	result[ExtraSamplingMode] = mode
	return result
}

// WithSamplingModeAndBatchID 为密集采样指标附加模式和批次标识。
func WithSamplingModeAndBatchID(extra Extra, mode, batchID string) Extra {
	result := WithSamplingMode(extra, mode)
	if batchID != "" {
		result[ExtraBurstBatchID] = batchID
	}
	return result
}

func NewIOLatencyExtra(writeLatencyMS, syncLatencyMS float64) Extra {
	return Extra{
		ExtraWriteLatencyMS: writeLatencyMS,
		ExtraSyncLatencyMS:  syncLatencyMS,
	}
}

func NewRandomIOExtra(writeLatencyMS, readLatencyMS float64, directIOWrite, directIORead bool) Extra {
	return Extra{
		ExtraWriteLatencyMS: writeLatencyMS,
		ExtraReadLatencyMS:  readLatencyMS,
		ExtraDirectIOWrite:  directIOWrite,
		ExtraDirectIORead:   directIORead,
	}
}

// RandomIODetails 是随机 I/O 测试的分布与落盘验证信息。
type RandomIODetails struct {
	Method        string
	DeviceCheck   string
	DeviceReadOps uint64
	Reads         int
	Writes        int
	ReadP50MS     float64
	ReadP99MS     float64
	WriteP50MS    float64
	WriteP99MS    float64
}

func NewRandomIODetailExtra(writeLatencyMS, readLatencyMS float64, directIOWrite, directIORead bool, details RandomIODetails) Extra {
	extra := NewRandomIOExtra(writeLatencyMS, readLatencyMS, directIOWrite, directIORead)
	extra[ExtraRandomIOMethod] = details.Method
	extra[ExtraRandomIODeviceCheck] = details.DeviceCheck
	extra[ExtraRandomIODeviceReadOps] = details.DeviceReadOps
	extra[ExtraRandomIOReads] = details.Reads
	extra[ExtraRandomIOWrites] = details.Writes
	extra[ExtraReadP50MS] = details.ReadP50MS
	extra[ExtraReadP99MS] = details.ReadP99MS
	extra[ExtraWriteP50MS] = details.WriteP50MS
	extra[ExtraWriteP99MS] = details.WriteP99MS
	return extra
}

func NewMemoryExtra(totalKB, availableKB uint64, availablePercent, swapUsage float64) Extra {
	return Extra{
		ExtraTotalKB:          totalKB,
		ExtraAvailableKB:      availableKB,
		ExtraAvailablePercent: availablePercent,
		ExtraSwapUsage:        swapUsage,
	}
}

func NewDiskStatsExtra(deviceName string, readOps, writeOps, readBytes, writeBytes, ioTimeMS, weightedIOMS uint64) Extra {
	return Extra{
		ExtraDeviceName:   deviceName,
		ExtraReadOps:      readOps,
		ExtraWriteOps:     writeOps,
		ExtraReadBytes:    readBytes,
		ExtraWriteBytes:   writeBytes,
		ExtraIOTimeMS:     ioTimeMS,
		ExtraWeightedIOMS: weightedIOMS,
	}
}

func NewLoadExtra(load1, load5, load15, numCPU float64) Extra {
	return Extra{
		ExtraLoad1:  load1,
		ExtraLoad5:  load5,
		ExtraLoad15: load15,
		ExtraNumCPU: numCPU,
	}
}

func NewHostContextExtra(hypervisorDetected, containerDetected bool, virtualizationType string, stealDirectlyInterpretable bool) Extra {
	return Extra{
		ExtraHypervisorDetected:         hypervisorDetected,
		ExtraContainerDetected:          containerDetected,
		ExtraVirtualizationType:         virtualizationType,
		ExtraStealDirectlyInterpretable: stealDirectlyInterpretable,
	}
}

func NewCPUThrottleExtra(periods, throttledPeriods, throttledUsec uint64) Extra {
	return Extra{
		ExtraPeriods:          periods,
		ExtraThrottledPeriods: throttledPeriods,
		ExtraThrottledUsec:    throttledUsec,
	}
}

func NewPressureExtra(someAvg10, someAvg60, someAvg300 float64, someTotal uint64, fullAvg10, fullAvg60, fullAvg300 float64, fullTotal uint64, hasFull bool) Extra {
	return Extra{
		ExtraSomeAvg10:  someAvg10,
		ExtraSomeAvg60:  someAvg60,
		ExtraSomeAvg300: someAvg300,
		ExtraSomeTotal:  someTotal,
		ExtraFullAvg10:  fullAvg10,
		ExtraFullAvg60:  fullAvg60,
		ExtraFullAvg300: fullAvg300,
		ExtraFullTotal:  fullTotal,
		ExtraHasFull:    hasFull,
	}
}
