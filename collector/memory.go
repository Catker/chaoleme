package collector

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MemoryStats 内存统计数据
type MemoryStats struct {
	MemTotal     uint64 // 总内存（KB）
	MemFree      uint64 // 空闲内存（KB）
	MemAvailable uint64 // 可用内存（KB）
	Buffers      uint64 // 缓冲区（KB）
	Cached       uint64 // 缓存（KB）
	SwapTotal    uint64 // 总交换空间（KB）
	SwapFree     uint64 // 空闲交换空间（KB）
}

// UsagePercent 计算内存使用率
func (m *MemoryStats) UsagePercent() float64 {
	if m.MemTotal == 0 {
		return 0
	}
	used := m.MemTotal - m.MemAvailable
	return float64(used) / float64(m.MemTotal) * 100
}

// AvailablePercent 计算内存可用率
func (m *MemoryStats) AvailablePercent() float64 {
	if m.MemTotal == 0 {
		return 0
	}
	return float64(m.MemAvailable) / float64(m.MemTotal) * 100
}

// SwapUsagePercent 计算交换空间使用率
func (m *MemoryStats) SwapUsagePercent() float64 {
	if m.SwapTotal == 0 {
		return 0
	}
	used := m.SwapTotal - m.SwapFree
	return float64(used) / float64(m.SwapTotal) * 100
}

// MemoryCollector 内存采集器
type MemoryCollector struct{}

// NewMemoryCollector 创建内存采集器
func NewMemoryCollector() *MemoryCollector {
	return &MemoryCollector{}
}

// Collect 采集内存统计
func (c *MemoryCollector) Collect() (*MemoryStats, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil, fmt.Errorf("无法打开 /proc/meminfo: %w", err)
	}
	defer file.Close()

	stats := &MemoryStats{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		switch key {
		case "MemTotal":
			stats.MemTotal = value
		case "MemFree":
			stats.MemFree = value
		case "MemAvailable":
			stats.MemAvailable = value
		case "Buffers":
			stats.Buffers = value
		case "Cached":
			stats.Cached = value
		case "SwapTotal":
			stats.SwapTotal = value
		case "SwapFree":
			stats.SwapFree = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 /proc/meminfo 失败: %w", err)
	}

	// 如果 MemAvailable 不存在（老内核），估算它
	if stats.MemAvailable == 0 {
		stats.MemAvailable = stats.MemFree + stats.Buffers + stats.Cached
	}

	return stats, nil
}
