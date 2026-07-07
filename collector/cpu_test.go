package collector

import "testing"

func TestCPUStatsTotalDoesNotDoubleCountGuestTime(t *testing.T) {
	t.Parallel()

	stats := &CPUStats{
		User:      100,
		Nice:      10,
		System:    20,
		Idle:      300,
		IOWait:    5,
		IRQ:       1,
		SoftIRQ:   2,
		Steal:     7,
		Guest:     50,
		GuestNice: 6,
	}

	want := uint64(445)
	if got := stats.Total(); got != want {
		t.Fatalf("CPU 总时间不应重复计入 guest，got=%d want=%d", got, want)
	}
}

func TestNewCPUCollectorAllowsWarmupDelayOverride(t *testing.T) {
	t.Parallel()

	collector := NewCPUCollectorWithWarmupDelay(0)
	if collector.warmupDelay != 0 {
		t.Fatalf("首次采样间隔应可设置为 0，实际=%s", collector.warmupDelay)
	}
	if NewCPUCollector().warmupDelay <= 0 {
		t.Fatal("默认采集器应保留非零首次采样间隔")
	}
}

func TestCalculateCPUUsageRejectsCounterRollback(t *testing.T) {
	t.Parallel()

	_, err := calculateCPUUsage(
		&CPUStats{User: 100, Idle: 100, Steal: 10},
		&CPUStats{User: 90, Idle: 100, Steal: 9},
	)
	if err == nil {
		t.Fatal("计数器回退时应返回错误")
	}
}

func TestCalculateCPUUsagePercentages(t *testing.T) {
	t.Parallel()

	usage, err := calculateCPUUsage(
		&CPUStats{User: 100, Idle: 100, IOWait: 10, Steal: 10},
		&CPUStats{User: 150, Idle: 130, IOWait: 15, Steal: 15},
	)
	if err != nil {
		t.Fatalf("计算 CPU 使用率失败: %v", err)
	}
	if usage.StealPercent != 5.555555555555555 || usage.IOWaitPercent != 5.555555555555555 {
		t.Fatalf("百分比不符合预期: %+v", usage)
	}
}
