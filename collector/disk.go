package collector

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// DiskCollector 磁盘 I/O 采集器
type DiskCollector struct {
	testDir  string
	testSize int // 测试文件大小（字节）
}

// isTmpfs 检测指定路径是否挂载为 tmpfs（内存盘）
// 注意：在 tmpfs 上进行 I/O 测试会测量内存速度而非磁盘速度
func isTmpfs(path string) bool {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mountPoint := fields[1]
		fsType := fields[2]

		// 检查路径是否以此挂载点开头，且文件系统类型为 tmpfs
		if strings.HasPrefix(path, mountPoint) && fsType == "tmpfs" {
			// 精确匹配或目录前缀匹配
			if path == mountPoint || strings.HasPrefix(path, mountPoint+"/") {
				return true
			}
		}
	}
	return false
}

// selectTestDir 选择合适的测试目录，避免使用 tmpfs
// 优先级：/tmp（非tmpfs） > /var/tmp > 程序当前目录
func selectTestDir() string {
	candidates := []string{"/tmp", "/var/tmp", "."}

	for _, dir := range candidates {
		if dir == "." {
			// 当前目录作为最后手段
			return dir
		}
		// 检查目录是否存在且可写
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		// 检查是否为 tmpfs
		if isTmpfs(dir) {
			continue
		}
		return dir
	}
	return "."
}

// NewDiskCollector 创建磁盘采集器
// 自动检测并选择合适的测试目录，避免在 tmpfs 上测试
func NewDiskCollector(testSizeMB int) *DiskCollector {
	testDir := selectTestDir()
	return &DiskCollector{
		testDir:  testDir,
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
// 注意：在 VPS 环境中此方法可能不可靠，建议使用 DetectStorageTypeByLatency
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

// DetectStorageTypeByLatency 根据随机读延迟推断存储类型
// 这比读取 /sys/block/.../rotational 在 VPS 环境更可靠
// 典型延迟参考:
//   - NVMe SSD: 0.05 - 0.5ms
//   - SATA SSD: 0.1 - 1ms
//   - HDD 7200rpm: 8 - 15ms
//   - HDD 5400rpm: 12 - 20ms
func DetectStorageTypeByLatency(randomReadLatencyMs float64) StorageType {
	if randomReadLatencyMs <= 0 {
		return StorageTypeUnknown
	}
	if randomReadLatencyMs < 2.0 {
		return StorageTypeSSD // < 2ms 基本是 SSD
	} else if randomReadLatencyMs > 5.0 {
		return StorageTypeHDD // > 5ms 大概率是 HDD
	}
	return StorageTypeUnknown // 2-5ms 区间不确定
}

// DiskStats 系统级磁盘统计（从 /proc/diskstats 采集）
type DiskStats struct {
	ReadOps      uint64 // 读操作完成次数
	WriteOps     uint64 // 写操作完成次数
	ReadBytes    uint64 // 读取字节数
	WriteBytes   uint64 // 写入字节数
	IOTimeMs     uint64 // IO 操作耗时（毫秒）
	WeightedIOMs uint64 // 加权 IO 耗时（反映队列深度）
}

// CollectDiskStats 从 /proc/diskstats 采集磁盘统计
// 开销极低：仅读取内核虚拟文件，无实际磁盘 IO
func (d *DiskCollector) CollectDiskStats() (*DiskStats, error) {
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, fmt.Errorf("读取 /proc/diskstats 失败: %w", err)
	}

	stats := &DiskStats{}
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}

		deviceName := fields[2]
		// 跳过分区（如 sda1, vda1）和虚拟设备
		if strings.HasPrefix(deviceName, "loop") ||
			strings.HasPrefix(deviceName, "ram") ||
			strings.HasPrefix(deviceName, "dm-") {
			continue
		}
		// 跳过分区，只统计整盘
		if len(deviceName) > 2 && deviceName[len(deviceName)-1] >= '0' && deviceName[len(deviceName)-1] <= '9' {
			// 检查是否为分区（如 sda1, vda1, nvme0n1p1）
			if strings.Contains(deviceName, "p") || (deviceName[len(deviceName)-2] >= 'a' && deviceName[len(deviceName)-2] <= 'z') {
				continue
			}
		}

		// 解析字段
		// fields[3]: 读完成次数
		// fields[5]: 读扇区数 (每扇区 512 字节)
		// fields[7]: 写完成次数
		// fields[9]: 写扇区数
		// fields[12]: IO 耗时 (毫秒)
		// fields[13]: 加权 IO 耗时

		readOps, _ := parseUint64(fields[3])
		readSectors, _ := parseUint64(fields[5])
		writeOps, _ := parseUint64(fields[7])
		writeSectors, _ := parseUint64(fields[9])
		ioTime, _ := parseUint64(fields[12])
		weightedIO, _ := parseUint64(fields[13])

		stats.ReadOps += readOps
		stats.WriteOps += writeOps
		stats.ReadBytes += readSectors * 512
		stats.WriteBytes += writeSectors * 512
		stats.IOTimeMs += ioTime
		stats.WeightedIOMs += weightedIO
	}

	return stats, nil
}

// parseUint64 解析 uint64，失败返回 0
func parseUint64(s string) (uint64, error) {
	var v uint64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

// RandomIOResult 随机读写测试结果
type RandomIOResult struct {
	RandomWriteLatencyMs float64 // 4KB 随机写延迟
	RandomReadLatencyMs  float64 // 4KB 随机读延迟
}

// alignedBuffer 创建对齐的缓冲区（O_DIRECT 需要内存对齐）
// alignment 通常为 512 或 4096 字节
func alignedBuffer(size, alignment int) []byte {
	// 分配额外空间以确保对齐
	buf := make([]byte, size+alignment)
	// 计算对齐偏移
	offset := alignment - int(uintptr(unsafe.Pointer(&buf[0]))%uintptr(alignment))
	if offset == alignment {
		offset = 0
	}
	return buf[offset : offset+size]
}

// TestRandomIO 执行 4KB 随机读写测试
// 使用 O_DIRECT 绕过页缓存，测量真实磁盘延迟
func (d *DiskCollector) TestRandomIO() (*RandomIOResult, error) {
	const blockSize = 4096 // 4KB，也是常见的磁盘扇区/页大小

	// 创建对齐的写入缓冲区（O_DIRECT 需要）
	writeData := alignedBuffer(blockSize, blockSize)
	if _, err := rand.Read(writeData); err != nil {
		return nil, fmt.Errorf("生成随机数据失败: %w", err)
	}

	// 创建临时文件路径
	tmpFile := filepath.Join(d.testDir, fmt.Sprintf("chaoleme-random-io-%d", time.Now().UnixNano()))
	defer os.Remove(tmpFile)

	// ========== 测试随机写入（使用 O_DIRECT） ==========
	writeStart := time.Now()
	writeFile, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_DIRECT, 0600)
	if err != nil {
		// O_DIRECT 不支持时，回退到普通模式
		writeFile, err = os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return nil, fmt.Errorf("创建测试文件失败: %w", err)
		}
	}

	_, err = writeFile.Write(writeData)
	if err != nil {
		writeFile.Close()
		return nil, fmt.Errorf("写入测试数据失败: %w", err)
	}

	// O_DIRECT 模式下数据直接落盘，但仍调用 Sync 确保元数据同步
	err = writeFile.Sync()
	writeFile.Close()
	if err != nil {
		return nil, fmt.Errorf("fsync 失败: %w", err)
	}
	writeLatency := time.Since(writeStart)

	// ========== 测试随机读取（使用 O_DIRECT 绕过页缓存） ==========
	// 创建对齐的读取缓冲区
	readData := alignedBuffer(blockSize, blockSize)

	readStart := time.Now()
	readFile, err := os.OpenFile(tmpFile, os.O_RDONLY|syscall.O_DIRECT, 0)
	if err != nil {
		// O_DIRECT 不支持时，回退到普通模式（此时读取会命中缓存）
		readFile, err = os.OpenFile(tmpFile, os.O_RDONLY, 0)
		if err != nil {
			return nil, fmt.Errorf("打开测试文件读取失败: %w", err)
		}
	}

	_, err = readFile.Read(readData)
	readLatency := time.Since(readStart)
	readFile.Close()

	if err != nil {
		return nil, fmt.Errorf("读取测试数据失败: %w", err)
	}

	return &RandomIOResult{
		RandomWriteLatencyMs: float64(writeLatency.Microseconds()) / 1000.0,
		RandomReadLatencyMs:  float64(readLatency.Microseconds()) / 1000.0,
	}, nil
}
