package collector

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// CPUStats CPU 统计数据
type CPUStats struct {
	User      uint64
	Nice      uint64
	System    uint64
	Idle      uint64
	IOWait    uint64
	IRQ       uint64
	SoftIRQ   uint64
	Steal     uint64
	Guest     uint64
	GuestNice uint64
}

// Total 计算总 CPU 时间
func (s *CPUStats) Total() uint64 {
	return s.User + s.Nice + s.System + s.Idle + s.IOWait + s.IRQ + s.SoftIRQ + s.Steal + s.Guest + s.GuestNice
}

// CPUCollector CPU 数据采集器
type CPUCollector struct {
	lastStats *CPUStats
}

// NewCPUCollector 创建 CPU 采集器
func NewCPUCollector() *CPUCollector {
	return &CPUCollector{}
}

// readCPUStats 从 /proc/stat 读取 CPU 统计
func readCPUStats() (*CPUStats, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil, fmt.Errorf("无法打开 /proc/stat: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 11 {
				return nil, fmt.Errorf("cpu 行字段不足: %s", line)
			}

			stats := &CPUStats{}
			values := make([]uint64, 10)
			for i := 0; i < 10 && i+1 < len(fields); i++ {
				v, err := strconv.ParseUint(fields[i+1], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("解析 CPU 统计失败: %w", err)
				}
				values[i] = v
			}

			stats.User = values[0]
			stats.Nice = values[1]
			stats.System = values[2]
			stats.Idle = values[3]
			stats.IOWait = values[4]
			stats.IRQ = values[5]
			stats.SoftIRQ = values[6]
			stats.Steal = values[7]
			stats.Guest = values[8]
			stats.GuestNice = values[9]

			return stats, nil
		}
	}

	return nil, fmt.Errorf("未找到 cpu 行")
}

// CPUUsageResult CPU 使用率采集结果（统一采集，确保数据准确性）
// CPUUsage 包含单次采集的 CPU 指标
type CPUUsage struct {
	StealPercent  float64
	IOWaitPercent float64
}

// Collect 统一采集 CPU 指标（Steal 和 IOWait）
func (c *CPUCollector) Collect() (*CPUUsage, error) {
	current, err := readCPUStats()
	if err != nil {
		return nil, err
	}

	if c.lastStats == nil {
		c.lastStats = current
		// 等待一小段时间再采集，确保有时间差
		time.Sleep(100 * time.Millisecond)
		current, err = readCPUStats()
		if err != nil {
			return nil, err
		}
	}

	totalDelta := current.Total() - c.lastStats.Total()
	stealDelta := current.Steal - c.lastStats.Steal
	iowaitDelta := current.IOWait - c.lastStats.IOWait

	// 更新 lastStats
	c.lastStats = current

	if totalDelta == 0 {
		return &CPUUsage{0, 0}, nil
	}

	return &CPUUsage{
		StealPercent:  float64(stealDelta) / float64(totalDelta) * 100,
		IOWaitPercent: float64(iowaitDelta) / float64(totalDelta) * 100,
	}, nil
}

// BenchmarkResult CPU 基准测试结果
type BenchmarkResult struct {
	DurationMs float64 // 执行耗时（毫秒）
}

// RunBenchmark 执行 CPU 基准测试
// 计算一定数量的素数，返回耗时
func (c *CPUCollector) RunBenchmark() (*BenchmarkResult, error) {
	start := time.Now()

	// 使用埃拉托斯特尼筛法找前 10000 个素数
	const targetCount = 10000
	count := 0
	n := 2

	for count < targetCount {
		if isPrime(n) {
			count++
		}
		n++
	}

	duration := time.Since(start)

	return &BenchmarkResult{
		DurationMs: float64(duration.Microseconds()) / 1000.0,
	}, nil
}

// isPrime 判断是否为素数
func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	if n == 2 {
		return true
	}
	if n%2 == 0 {
		return false
	}
	sqrt := int(math.Sqrt(float64(n)))
	for i := 3; i <= sqrt; i += 2 {
		if n%i == 0 {
			return false
		}
	}
	return true
}
