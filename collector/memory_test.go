package collector

import "testing"

func TestMemoryStatsPercentages(t *testing.T) {
	t.Parallel()

	stats := &MemoryStats{
		MemTotal:     1000,
		MemAvailable: 250,
		SwapTotal:    200,
		SwapFree:     50,
	}
	if got := stats.UsagePercent(); got != 75 {
		t.Fatalf("内存使用率不符合预期: %.1f", got)
	}
	if got := stats.AvailablePercent(); got != 25 {
		t.Fatalf("内存可用率不符合预期: %.1f", got)
	}
	if got := stats.SwapUsagePercent(); got != 75 {
		t.Fatalf("Swap 使用率不符合预期: %.1f", got)
	}
}

func TestMemoryStatsZeroTotals(t *testing.T) {
	t.Parallel()

	stats := &MemoryStats{}
	if stats.UsagePercent() != 0 || stats.AvailablePercent() != 0 || stats.SwapUsagePercent() != 0 {
		t.Fatalf("总量为 0 时百分比应为 0: %+v", stats)
	}
}
