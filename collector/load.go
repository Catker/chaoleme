package collector

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LoadResult Load Average 采集结果
type LoadResult struct {
	Load1  float64 // 1 分钟平均负载
	Load5  float64 // 5 分钟平均负载
	Load15 float64 // 15 分钟平均负载
}

// CollectLoadAverage 采集系统 Load Average
// 读取 /proc/loadavg 获取负载信息
func CollectLoadAverage() (*LoadResult, error) {
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		return nil, fmt.Errorf("无法打开 /proc/loadavg: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return nil, fmt.Errorf("读取 /proc/loadavg 失败")
	}

	line := scanner.Text()
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return nil, fmt.Errorf("loadavg 格式错误: %s", line)
	}

	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil, fmt.Errorf("解析 load1 失败: %w", err)
	}

	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return nil, fmt.Errorf("解析 load5 失败: %w", err)
	}

	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return nil, fmt.Errorf("解析 load15 失败: %w", err)
	}

	return &LoadResult{
		Load1:  load1,
		Load5:  load5,
		Load15: load15,
	}, nil
}
