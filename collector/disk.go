package collector

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"
)

const (
	randomIOBlockSize = 4096 // 4KB，也是常见的磁盘扇区/页大小
	randomIOFileName  = "chaoleme-random-io.dat"
	prefillChunkSize  = 1024 * 1024

	DefaultRandomIOFileMB = 64
	DefaultRandomIOReads  = 128
	DefaultRandomIOWrites = 32

	// RandomIOMethodPrefilled 标识使用预写文件的随机 I/O 测试方法。
	RandomIOMethodPrefilled = "prefilled"
	// RandomIODeviceVerified 设备读次数增量覆盖了测试读次数，读请求确实下发到块设备。
	RandomIODeviceVerified = "verified"
	// RandomIODeviceNotReached 设备读次数增量明显不足，读请求被页缓存或空洞吸收。
	RandomIODeviceNotReached = "not_reached"
	// RandomIODeviceUnknown 无法把测试目录映射到块设备，不能验证。
	RandomIODeviceUnknown = "unknown"
)

// DiskCollector 磁盘 I/O 采集器
type DiskCollector struct {
	testDir  string
	testSize int // 测试文件大小（字节）

	// 随机 I/O 使用常驻的预写文件，避免每次重建和稀疏空洞。
	randomIODir      string
	randomIOFileSize int64
	randomIOReads    int
	randomIOWrites   int
}

// RandomIOOptions 随机 I/O 测试参数
type RandomIOOptions struct {
	Dir    string // 预写文件所在目录，应位于持久化的真实磁盘上
	FileMB int
	Reads  int
	Writes int
}

// isTmpfs 检测指定路径是否挂载为 tmpfs（内存盘）
// 注意：在 tmpfs 上进行 I/O 测试会测量内存速度而非磁盘速度
func isTmpfs(path string) bool {
	data, err := os.ReadFile(procMountsPath)
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
		testDir:          testDir,
		testSize:         testSizeMB * 1024 * 1024,
		randomIODir:      testDir,
		randomIOFileSize: DefaultRandomIOFileMB * 1024 * 1024,
		randomIOReads:    DefaultRandomIOReads,
		randomIOWrites:   DefaultRandomIOWrites,
	}
}

// NewDiskCollectorWithRandomIO 创建磁盘采集器，并指定随机 I/O 测试参数。
// 非正数参数使用默认值；Dir 为空时沿用顺序写测试目录。
func NewDiskCollectorWithRandomIO(testSizeMB int, opts RandomIOOptions) *DiskCollector {
	d := NewDiskCollector(testSizeMB)
	if opts.Dir != "" {
		d.randomIODir = opts.Dir
	}
	if opts.FileMB > 0 {
		d.randomIOFileSize = int64(opts.FileMB) * 1024 * 1024
	}
	if opts.Reads > 0 {
		d.randomIOReads = opts.Reads
	}
	if opts.Writes > 0 {
		d.randomIOWrites = opts.Writes
	}
	return d
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
	entries, err := os.ReadDir(sysBlockPath)
	if err != nil {
		return StorageTypeUnknown
	}

	for _, entry := range entries {
		name := entry.Name()
		// 跳过 loop、ram 等虚拟设备
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "dm-") {
			continue
		}

		rotationalPath := filepath.Join(sysBlockPath, name, "queue", "rotational")
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
	DeviceName   string // 测试目录所在挂载点对应的块设备名称
	ReadOps      uint64 // 读操作完成次数
	WriteOps     uint64 // 写操作完成次数
	ReadBytes    uint64 // 读取字节数
	WriteBytes   uint64 // 写入字节数
	IOTimeMs     uint64 // IO 操作耗时（毫秒）
	WeightedIOMs uint64 // 加权 IO 耗时（反映队列深度）
}

// CollectDiskStats 从测试目录所在设备的 /proc/diskstats 条目采集统计。
// 无法将测试目录可靠映射到单个块设备时，返回错误而不是汇总无关设备。
func (d *DiskCollector) CollectDiskStats() (*DiskStats, error) {
	return collectDiskStatsForDir(d.testDir)
}

// collectDiskStatsForDir 采集指定目录所在块设备的 /proc/diskstats 条目。
func collectDiskStatsForDir(dir string) (*DiskStats, error) {
	testDir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return nil, fmt.Errorf("解析测试目录失败: %w", err)
	}

	mountInfo, err := os.ReadFile(procSelfMountinfoPath)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", procSelfMountinfoPath, err)
	}
	diskStatsData, err := os.ReadFile(procDiskstatsPath)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", procDiskstatsPath, err)
	}

	mount, err := findTestDirMount(testDir, mountInfo)
	if err != nil {
		return nil, err
	}
	deviceName, err := diskDeviceName(mount, diskStatsData)
	if err != nil {
		return nil, err
	}
	return parseDiskStatsForDevice(deviceName, diskStatsData)
}

type mountInfoEntry struct {
	root       string
	mountPoint string
	fsType     string
	source     string
}

func findTestDirMount(testDir string, data []byte) (mountInfoEntry, error) {
	var selected mountInfoEntry
	found := false

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		separator := -1
		for i, field := range fields {
			if field == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || separator+2 >= len(fields) {
			continue
		}

		entry := mountInfoEntry{
			root:       decodeMountinfoPath(fields[3]),
			mountPoint: filepath.Clean(decodeMountinfoPath(fields[4])),
			fsType:     decodeMountinfoPath(fields[separator+1]),
			source:     decodeMountinfoPath(fields[separator+2]),
		}
		if !pathUsesMount(testDir, entry.mountPoint) {
			continue
		}
		if !found || len(entry.mountPoint) > len(selected.mountPoint) {
			selected = entry
			found = true
			continue
		}
		if len(entry.mountPoint) == len(selected.mountPoint) && !sameMountCandidate(entry, selected) {
			return mountInfoEntry{}, fmt.Errorf("无法识别测试目录所在块设备: 最深挂载点存在不一致候选")
		}
	}

	if !found {
		return mountInfoEntry{}, fmt.Errorf("无法识别测试目录所在块设备: 未找到 %s 的挂载点", testDir)
	}
	return selected, nil
}

func sameMountCandidate(a, b mountInfoEntry) bool {
	return a.root == b.root && a.mountPoint == b.mountPoint && a.fsType == b.fsType && a.source == b.source
}

func pathUsesMount(path, mountPoint string) bool {
	if mountPoint == "/" {
		return strings.HasPrefix(path, "/")
	}
	return path == mountPoint || strings.HasPrefix(path, mountPoint+"/")
}

func decodeMountinfoPath(value string) string {
	var decoded strings.Builder
	decoded.Grow(len(value))

	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+3 >= len(value) {
			decoded.WriteByte(value[i])
			continue
		}
		first, second, third := value[i+1], value[i+2], value[i+3]
		if first < '0' || first > '7' || second < '0' || second > '7' || third < '0' || third > '7' {
			decoded.WriteByte(value[i])
			continue
		}
		decoded.WriteByte((first-'0')*64 + (second-'0')*8 + (third - '0'))
		i += 3
	}
	return decoded.String()
}

func diskDeviceName(mount mountInfoEntry, diskStatsData []byte) (string, error) {
	if mount.root != "/" {
		return "", fmt.Errorf("无法识别测试目录所在块设备: 挂载根目录 %q 不是 /", mount.root)
	}
	if mount.fsType == "btrfs" {
		return "", fmt.Errorf("无法识别测试目录所在块设备: 文件系统 %q 不能证明单设备", mount.fsType)
	}
	if strings.HasPrefix(mount.source, "/dev/mapper/") {
		return "", fmt.Errorf("无法识别测试目录所在块设备: 不支持 device-mapper 来源 %q", mount.source)
	}
	if !strings.HasPrefix(mount.source, "/dev/") {
		return "", fmt.Errorf("无法识别测试目录所在块设备: 挂载来源 %q 不是直接块设备", mount.source)
	}

	deviceName := strings.TrimPrefix(mount.source, "/dev/")
	if deviceName == "" || strings.Contains(deviceName, "/") {
		return "", fmt.Errorf("无法识别测试目录所在块设备: 挂载来源 %q 无效", mount.source)
	}
	if strings.HasPrefix(deviceName, "dm-") {
		return "", fmt.Errorf("无法识别测试目录所在块设备: 不支持 device-mapper 设备 %q", deviceName)
	}
	if !diskStatsHasDevice(diskStatsData, deviceName) {
		return "", fmt.Errorf("无法识别测试目录所在块设备: %q 未出现在 %s", deviceName, procDiskstatsPath)
	}
	return deviceName, nil
}

func diskStatsHasDevice(data []byte, deviceName string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == deviceName {
			return true
		}
	}
	return false
}

func parseDiskStatsForDevice(deviceName string, data []byte) (*DiskStats, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[2] != deviceName {
			continue
		}
		if len(fields) < 14 {
			return nil, fmt.Errorf("解析 %s 失败: 字段不足", deviceName)
		}

		// fields[3]: 读完成次数
		// fields[5]: 读扇区数 (每扇区 512 字节)
		// fields[7]: 写完成次数
		// fields[9]: 写扇区数
		// fields[12]: IO 耗时 (毫秒)
		// fields[13]: 加权 IO 耗时

		readOps, err := parseUint64(fields[3])
		if err != nil {
			return nil, fmt.Errorf("解析 %s 读操作数失败: %w", deviceName, err)
		}
		readSectors, err := parseUint64(fields[5])
		if err != nil {
			return nil, fmt.Errorf("解析 %s 读扇区数失败: %w", deviceName, err)
		}
		writeOps, err := parseUint64(fields[7])
		if err != nil {
			return nil, fmt.Errorf("解析 %s 写操作数失败: %w", deviceName, err)
		}
		writeSectors, err := parseUint64(fields[9])
		if err != nil {
			return nil, fmt.Errorf("解析 %s 写扇区数失败: %w", deviceName, err)
		}
		ioTime, err := parseUint64(fields[12])
		if err != nil {
			return nil, fmt.Errorf("解析 %s IO 耗时失败: %w", deviceName, err)
		}
		weightedIO, err := parseUint64(fields[13])
		if err != nil {
			return nil, fmt.Errorf("解析 %s 加权 IO 耗时失败: %w", deviceName, err)
		}

		return &DiskStats{
			DeviceName:   deviceName,
			ReadOps:      readOps,
			WriteOps:     writeOps,
			ReadBytes:    readSectors * 512,
			WriteBytes:   writeSectors * 512,
			IOTimeMs:     ioTime,
			WeightedIOMs: weightedIO,
		}, nil
	}

	return nil, fmt.Errorf("解析 %s 失败: 未找到设备", deviceName)
}

func shouldSkipDiskDevice(name string) bool {
	if name == "" {
		return true
	}
	if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "dm-") {
		return true
	}

	// NVMe 与 MMC 整盘名称通常以数字结尾，如 nvme0n1、mmcblk0；
	// 其分区会带 p，如 nvme0n1p1、mmcblk0p1。
	if strings.HasPrefix(name, "nvme") || strings.HasPrefix(name, "mmcblk") {
		return strings.Contains(name, "p")
	}

	// sda1、vda1、xvda1 这类以数字结尾的是分区。
	last := name[len(name)-1]
	return last >= '0' && last <= '9'
}

// parseUint64 解析 uint64，失败返回 0
func parseUint64(s string) (uint64, error) {
	var v uint64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

// RandomIOResult 随机读写测试结果
type RandomIOResult struct {
	RandomWriteLatencyMs float64 // 4KB 随机写平均延迟
	RandomReadLatencyMs  float64 // 4KB 随机读平均延迟
	WriteP50Ms           float64
	WriteP99Ms           float64
	ReadP50Ms            float64
	ReadP99Ms            float64
	Writes               int
	Reads                int
	DirectIOWrite        bool   // 写测试是否成功使用 O_DIRECT
	DirectIORead         bool   // 读测试是否成功使用 O_DIRECT
	DeviceCheck          string // 读请求是否确实下发到块设备
	DeviceReadOps        uint64 // 读测试期间块设备读完成次数增量
}

// alignedBuffer 创建满足 O_DIRECT 地址要求的缓冲区。
// alignment 通常为 512 或 4096 字节
func alignedBuffer(size, alignment int) []byte {
	// 分配额外空间以满足地址边界要求。
	buf := make([]byte, size+alignment)
	// 计算地址偏移。
	offset := alignment - int(uintptr(unsafe.Pointer(&buf[0]))%uintptr(alignment))
	if offset == alignment {
		offset = 0
	}
	return buf[offset : offset+size]
}

// TestRandomIO 执行 4KB 随机读写测试。
// 使用常驻且预先写满随机数据的文件：读请求不会落在稀疏空洞上（空洞读由内核直接填零，不产生磁盘 I/O），
// 随机数据也避免被宿主机按零块压缩或去重。O_DIRECT 绕过客户机页缓存，
// 并用 /proc/diskstats 读次数增量验证读请求确实下发到了块设备。
// 注意：宿主机侧缓存无法从虚拟机内绕过，预写文件越大，命中宿主机缓存的概率越低。
func (d *DiskCollector) TestRandomIO() (*RandomIOResult, error) {
	path := filepath.Join(d.randomIODir, randomIOFileName)
	if err := ensureRandomIOFile(path, d.randomIOFileSize); err != nil {
		return nil, err
	}
	blocks := int(d.randomIOFileSize / randomIOBlockSize)

	// ========== 随机写（覆盖写已分配块，不触发块分配） ==========
	writeData := alignedBuffer(randomIOBlockSize, randomIOBlockSize)
	if _, err := rand.Read(writeData); err != nil {
		return nil, fmt.Errorf("生成随机数据失败: %w", err)
	}
	writeFile, directIOWrite, err := openWithDirectIO(path, os.O_WRONLY)
	if err != nil {
		return nil, fmt.Errorf("打开随机 IO 测试文件失败: %w", err)
	}
	writeLatencies, err := timeRandomBlockOps(d.randomIOWrites, blocks, func(offset int64) error {
		_, err := writeFile.WriteAt(writeData, offset)
		return err
	})
	if err != nil {
		writeFile.Close()
		return nil, fmt.Errorf("随机写入测试数据失败: %w", err)
	}
	// O_DIRECT 模式绕过页缓存，但仍调用 Sync 确保数据与元数据落盘。
	err = writeFile.Sync()
	writeFile.Close()
	if err != nil {
		return nil, fmt.Errorf("fsync 失败: %w", err)
	}

	// ========== 随机读（O_DIRECT，并用 diskstats 验证落盘） ==========
	readData := alignedBuffer(randomIOBlockSize, randomIOBlockSize)
	readFile, directIORead, err := openWithDirectIO(path, os.O_RDONLY)
	if err != nil {
		return nil, fmt.Errorf("打开随机 IO 测试文件读取失败: %w", err)
	}
	before, beforeErr := collectDiskStatsForDir(d.randomIODir)
	readLatencies, err := timeRandomBlockOps(d.randomIOReads, blocks, func(offset int64) error {
		_, err := readFile.ReadAt(readData, offset)
		return err
	})
	readFile.Close()
	if err != nil {
		return nil, fmt.Errorf("随机读取测试数据失败: %w", err)
	}
	after, afterErr := collectDiskStatsForDir(d.randomIODir)
	deviceCheck, deviceReadOps := verifyDeviceReads(before, after, beforeErr, afterErr, d.randomIOReads)

	return &RandomIOResult{
		RandomWriteLatencyMs: meanMs(writeLatencies),
		RandomReadLatencyMs:  meanMs(readLatencies),
		WriteP50Ms:           percentileMs(writeLatencies, 50),
		WriteP99Ms:           percentileMs(writeLatencies, 99),
		ReadP50Ms:            percentileMs(readLatencies, 50),
		ReadP99Ms:            percentileMs(readLatencies, 99),
		Writes:               len(writeLatencies),
		Reads:                len(readLatencies),
		DirectIOWrite:        directIOWrite,
		DirectIORead:         directIORead,
		DeviceCheck:          deviceCheck,
		DeviceReadOps:        deviceReadOps,
	}, nil
}

// ensureRandomIOFile 确保预写文件存在且大小正确；否则用随机数据重新写满。
// 先写临时文件再重命名，避免中断后留下未写满（含空洞）的测试文件。
func ensureRandomIOFile(path string, size int64) error {
	if size < randomIOBlockSize {
		return fmt.Errorf("随机 IO 测试文件过小: %d 字节", size)
	}
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() == size {
		// 例如以 root 手动运行后留下 0600 的文件，服务用户无法读写时删除重建（目录属于服务用户即可删除）。
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err == nil {
			return file.Close()
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("随机 IO 测试文件不可读写且无法删除: %w", err)
		}
	}

	tmpPath := path + ".tmp"
	file, _, err := openWithDirectIO(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("创建随机 IO 测试文件失败: %w", err)
	}
	// O_DIRECT 写入不会把 64MB 数据留在页缓存里。
	chunk := alignedBuffer(prefillChunkSize, randomIOBlockSize)
	for written := int64(0); written < size; {
		n := int64(len(chunk))
		if remaining := size - written; remaining < n {
			n = remaining
		}
		if _, err := rand.Read(chunk[:n]); err != nil {
			file.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("生成随机数据失败: %w", err)
		}
		if _, err := file.WriteAt(chunk[:n], written); err != nil {
			file.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("预写随机 IO 测试文件失败: %w", err)
		}
		written += n
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("fsync 随机 IO 测试文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("关闭随机 IO 测试文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("重命名随机 IO 测试文件失败: %w", err)
	}
	return nil
}

// openWithDirectIO 优先以 O_DIRECT 打开文件，不支持时回退普通模式。
func openWithDirectIO(path string, flag int) (*os.File, bool, error) {
	if directIOFlag != 0 {
		if file, err := os.OpenFile(path, flag|directIOFlag, 0600); err == nil {
			return file, true, nil
		}
	}
	file, err := os.OpenFile(path, flag, 0600)
	return file, false, err
}

// timeRandomBlockOps 对随机块偏移逐个执行操作并记录每次耗时（QD1）。
func timeRandomBlockOps(count, blocks int, op func(offset int64) error) ([]time.Duration, error) {
	latencies := make([]time.Duration, 0, count)
	for i := 0; i < count; i++ {
		offset, err := randomBlockOffset(blocks, randomIOBlockSize)
		if err != nil {
			return nil, err
		}
		start := time.Now()
		if err := op(offset); err != nil {
			return nil, err
		}
		latencies = append(latencies, time.Since(start))
	}
	return latencies, nil
}

// verifyDeviceReads 用读测试前后的设备读完成次数判断读请求是否下发到块设备。
// 其他进程的读只会让增量偏大，所以增量明显不足可以证明测试读被缓存或空洞吸收；
// 增量充足只能说明客户机侧没有吸收，宿主机缓存仍可能命中。
func verifyDeviceReads(before, after *DiskStats, beforeErr, afterErr error, reads int) (string, uint64) {
	if beforeErr != nil || afterErr != nil || before == nil || after == nil || before.DeviceName != after.DeviceName || after.ReadOps < before.ReadOps {
		return RandomIODeviceUnknown, 0
	}
	delta := after.ReadOps - before.ReadOps
	// 留 10% 余量，兼容少量请求合并。
	if float64(delta) < float64(reads)*0.9 {
		return RandomIODeviceNotReached, delta
	}
	return RandomIODeviceVerified, delta
}

func meanMs(latencies []time.Duration) float64 {
	if len(latencies) == 0 {
		return 0
	}
	var total time.Duration
	for _, latency := range latencies {
		total += latency
	}
	return durationMs(total) / float64(len(latencies))
}

// percentileMs 使用最近秩法计算分位数，样本较少时 P99 即最大值。
func percentileMs(latencies []time.Duration, p float64) float64 {
	if len(latencies) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), latencies...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return durationMs(sorted[rank-1])
}

func durationMs(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

func randomBlockOffset(blocks, blockSize int) (int64, error) {
	if blocks <= 0 || blockSize <= 0 {
		return 0, fmt.Errorf("随机 IO 参数无效: blocks=%d blockSize=%d", blocks, blockSize)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(blocks)))
	if err != nil {
		return 0, fmt.Errorf("生成随机块偏移失败: %w", err)
	}
	return n.Int64() * int64(blockSize), nil
}
