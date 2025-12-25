package collector

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DiskCollector 磁盘 I/O 采集器
type DiskCollector struct {
	testDir  string
	testSize int // 测试文件大小（字节）
}

// NewDiskCollector 创建磁盘采集器
func NewDiskCollector(testSizeMB int) *DiskCollector {
	return &DiskCollector{
		testDir:  "/tmp",
		testSize: testSizeMB * 1024 * 1024,
	}
}

// IOLatencyResult I/O 延迟测试结果
type IOLatencyResult struct {
	WriteLatencyMs float64 // 写入延迟（毫秒）
	SyncLatencyMs  float64 // fsync 延迟（毫秒）
	TotalLatencyMs float64 // 总延迟（毫秒）
}

// TestWriteLatency 测试写入延迟
func (d *DiskCollector) TestWriteLatency() (*IOLatencyResult, error) {
	// 生成随机数据
	data := make([]byte, d.testSize)
	if _, err := rand.Read(data); err != nil {
		return nil, fmt.Errorf("生成随机数据失败: %w", err)
	}

	// 创建临时文件
	tmpFile := filepath.Join(d.testDir, fmt.Sprintf("chaoleme-io-test-%d", time.Now().UnixNano()))

	// 测试写入
	writeStart := time.Now()
	file, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("创建测试文件失败: %w", err)
	}

	_, err = file.Write(data)
	if err != nil {
		file.Close()
		os.Remove(tmpFile)
		return nil, fmt.Errorf("写入测试数据失败: %w", err)
	}
	writeLatency := time.Since(writeStart)

	// 测试 fsync
	syncStart := time.Now()
	err = file.Sync()
	syncLatency := time.Since(syncStart)

	file.Close()
	os.Remove(tmpFile)

	if err != nil {
		return nil, fmt.Errorf("fsync 失败: %w", err)
	}

	return &IOLatencyResult{
		WriteLatencyMs: float64(writeLatency.Microseconds()) / 1000.0,
		SyncLatencyMs:  float64(syncLatency.Microseconds()) / 1000.0,
		TotalLatencyMs: float64((writeLatency + syncLatency).Microseconds()) / 1000.0,
	}, nil
}

// StorageType 存储类型
type StorageType string

const (
	StorageTypeSSD     StorageType = "SSD"
	StorageTypeHDD     StorageType = "HDD"
	StorageTypeUnknown StorageType = "Unknown"
)

// DetectStorageType 检测存储类型（SSD 或 HDD）
func (d *DiskCollector) DetectStorageType() StorageType {
	// 读取 /sys/block/*/queue/rotational
	// 0 = SSD, 1 = HDD
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return StorageTypeUnknown
	}

	for _, entry := range entries {
		name := entry.Name()
		// 跳过 loop、ram 等虚拟设备
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "dm-") {
			continue
		}

		rotationalPath := fmt.Sprintf("/sys/block/%s/queue/rotational", name)
		data, err := os.ReadFile(rotationalPath)
		if err != nil {
			continue
		}

		value := strings.TrimSpace(string(data))
		if value == "0" {
			return StorageTypeSSD
		} else if value == "1" {
			return StorageTypeHDD
		}
	}

	return StorageTypeUnknown
}
