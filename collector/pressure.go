package collector

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// PressureResult 表示 Linux PSI 的 some/full 压力数据。
type PressureResult struct {
	SomeAvg10  float64
	SomeAvg60  float64
	SomeAvg300 float64
	SomeTotal  uint64
	FullAvg10  float64
	FullAvg60  float64
	FullAvg300 float64
	FullTotal  uint64
	HasFull    bool
}

func CollectCPUPressure() (*PressureResult, error) {
	return collectPressureFile(procPressureCPU)
}

func CollectIOPressure() (*PressureResult, error) {
	return collectPressureFile(procPressureIO)
}

func collectPressureFile(path string) (*PressureResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("无法打开 %s: %w", path, err)
	}
	defer file.Close()

	result := &PressureResult{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		kind, values, err := parsePressureLine(scanner.Text())
		if err != nil {
			return nil, err
		}
		switch kind {
		case "some":
			result.SomeAvg10 = values.avg10
			result.SomeAvg60 = values.avg60
			result.SomeAvg300 = values.avg300
			result.SomeTotal = values.total
		case "full":
			result.FullAvg10 = values.avg10
			result.FullAvg60 = values.avg60
			result.FullAvg300 = values.avg300
			result.FullTotal = values.total
			result.HasFull = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	return result, nil
}

type pressureValues struct {
	avg10  float64
	avg60  float64
	avg300 float64
	total  uint64
}

func parsePressureLine(line string) (string, pressureValues, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return "", pressureValues{}, fmt.Errorf("pressure 行格式错误: %s", line)
	}

	kind := fields[0]
	values := pressureValues{}
	for _, field := range fields[1:] {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			return "", pressureValues{}, fmt.Errorf("pressure 字段格式错误: %s", field)
		}
		switch parts[0] {
		case "avg10":
			v, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return "", pressureValues{}, fmt.Errorf("解析 avg10 失败: %w", err)
			}
			values.avg10 = v
		case "avg60":
			v, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return "", pressureValues{}, fmt.Errorf("解析 avg60 失败: %w", err)
			}
			values.avg60 = v
		case "avg300":
			v, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				return "", pressureValues{}, fmt.Errorf("解析 avg300 失败: %w", err)
			}
			values.avg300 = v
		case "total":
			v, err := strconv.ParseUint(parts[1], 10, 64)
			if err != nil {
				return "", pressureValues{}, fmt.Errorf("解析 total 失败: %w", err)
			}
			values.total = v
		}
	}

	return kind, values, nil
}
