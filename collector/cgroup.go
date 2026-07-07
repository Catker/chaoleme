package collector

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CPUThrottleResult 表示 cgroup CPU 限额节流数据。
type CPUThrottleResult struct {
	Periods          uint64
	ThrottledPeriods uint64
	ThrottledUsec    uint64
}

func (r *CPUThrottleResult) ThrottledPercent() float64 {
	if r.Periods == 0 {
		return 0
	}
	return float64(r.ThrottledPeriods) / float64(r.Periods) * 100
}

func CollectCPUThrottle() (*CPUThrottleResult, error) {
	var lastErr error
	for _, path := range cgroupCPUStatPaths {
		result, err := collectCPUThrottleFile(path)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("无法读取 cgroup cpu.stat: %w", lastErr)
}

func collectCPUThrottleFile(path string) (*CPUThrottleResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	result := &CPUThrottleResult{}
	hasThrottleField := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("解析 cpu.stat 失败: %w", err)
		}
		switch fields[0] {
		case "nr_periods":
			result.Periods = value
			hasThrottleField = true
		case "nr_throttled":
			result.ThrottledPeriods = value
			hasThrottleField = true
		case "throttled_usec":
			result.ThrottledUsec = value
			hasThrottleField = true
		case "throttled_time":
			result.ThrottledUsec = value / 1000
			hasThrottleField = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !hasThrottleField {
		return nil, fmt.Errorf("cpu.stat 中没有节流字段")
	}
	return result, nil
}
