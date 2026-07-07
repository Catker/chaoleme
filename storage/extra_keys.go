package storage

type Extra map[string]interface{}

const (
	ExtraWriteLatencyMS = "write_latency_ms"
	ExtraSyncLatencyMS  = "sync_latency_ms"
	ExtraReadLatencyMS  = "read_latency_ms"
	ExtraDirectIOWrite  = "direct_io_write"
	ExtraDirectIORead   = "direct_io_read"

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

func NewMemoryExtra(totalKB, availableKB uint64, availablePercent, swapUsage float64) Extra {
	return Extra{
		ExtraTotalKB:          totalKB,
		ExtraAvailableKB:      availableKB,
		ExtraAvailablePercent: availablePercent,
		ExtraSwapUsage:        swapUsage,
	}
}

func NewDiskStatsExtra(readOps, writeOps, readBytes, writeBytes, ioTimeMS, weightedIOMS uint64) Extra {
	return Extra{
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
