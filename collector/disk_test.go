package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldSkipDiskDeviceKeepsWholeNVMeAndMMCDevices(t *testing.T) {
	t.Parallel()

	kept := []string{"sda", "vda", "xvda", "nvme0n1", "mmcblk0"}
	for _, name := range kept {
		if shouldSkipDiskDevice(name) {
			t.Fatalf("整盘设备 %s 不应被跳过", name)
		}
	}
}

func TestShouldSkipDiskDeviceSkipsPartitionsAndVirtualDevices(t *testing.T) {
	t.Parallel()

	skipped := []string{"sda1", "vda1", "xvda1", "nvme0n1p1", "mmcblk0p1", "loop0", "ram0", "dm-0"}
	for _, name := range skipped {
		if !shouldSkipDiskDevice(name) {
			t.Fatalf("分区或虚拟设备 %s 应被跳过", name)
		}
	}
}

func TestParseUint64ReturnsErrorOnInvalidInput(t *testing.T) {
	t.Parallel()

	if _, err := parseUint64("not-a-number"); err == nil {
		t.Fatal("非法数字应返回解析错误")
	}
}

func TestRandomBlockOffsetStaysWithinRange(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		offset, err := randomBlockOffset(64, 4096)
		if err != nil {
			t.Fatalf("生成随机偏移失败: %v", err)
		}
		if offset < 0 || offset >= 64*4096 || offset%4096 != 0 {
			t.Fatalf("随机偏移越界或未按块对齐: %d", offset)
		}
	}
}

func TestFindTestDirMountUsesDeepestDecodedMountPoint(t *testing.T) {
	t.Parallel()

	mount, err := findTestDirMount("/var/tmp/test dir/run", []byte("25 1 8:1 / / rw - ext4 /dev/vda1 rw\n26 25 8:2 / /var/tmp/test\\040dir rw - ext4 /dev/vdb1 rw\n"))
	if err != nil {
		t.Fatalf("查找挂载点失败: %v", err)
	}
	if mount.source != "/dev/vdb1" {
		t.Fatalf("应选择最深挂载点，实际=%+v", mount)
	}
}

func TestFindTestDirMountRejectsConflictingDeepestCandidates(t *testing.T) {
	t.Parallel()

	_, err := findTestDirMount("/data/run", []byte("25 1 8:1 / /data rw - ext4 /dev/vda1 rw\n26 1 8:2 / /data rw - ext4 /dev/vdb1 rw\n"))
	if err == nil || !strings.Contains(err.Error(), "不一致候选") {
		t.Fatalf("不一致的最深挂载点应被拒绝: %v", err)
	}
}

func TestDiskDeviceNameRejectsUnsupportedMounts(t *testing.T) {
	t.Parallel()

	diskStats := []byte("8 1 vda1 1 0 2 0 3 0 4 0 0 0 5 6\n")
	for _, mount := range []mountInfoEntry{
		{root: "/subtree", source: "/dev/vda1"},
		{root: "/", source: "overlay"},
		{root: "/", source: "/dev/mapper/data"},
		{root: "/", fsType: "btrfs", source: "/dev/vda1"},
	} {
		if _, err := diskDeviceName(mount, diskStats); err == nil || !strings.Contains(err.Error(), "无法识别") {
			t.Fatalf("不支持挂载应返回明确错误: mount=%+v err=%v", mount, err)
		}
	}
}

func TestCollectDiskStatsUsesTestDirectoryDevice(t *testing.T) {
	testDir := t.TempDir()
	useDiskStatsFixture(t,
		"25 1 8:1 / "+testDir+" rw - ext4 /dev/vda1 rw\n",
		"8 0 vda 100 0 200 0 300 0 400 0 0 500 600\n8 1 vda1 11 0 22 0 33 0 44 0 0 55 66\n",
	)

	stats, err := (&DiskCollector{testDir: testDir}).CollectDiskStats()
	if err != nil {
		t.Fatalf("采集目标分区失败: %v", err)
	}
	if stats.DeviceName != "vda1" || stats.ReadOps != 11 || stats.WriteOps != 33 || stats.IOTimeMs != 55 {
		t.Fatalf("应只读取目标分区而非父盘: %+v", stats)
	}
}

func TestCollectDiskStatsRejectsUnsupportedDeviceMappings(t *testing.T) {
	testDir := t.TempDir()
	diskStats := "8 1 vda1 1 0 2 0 3 0 4 0 0 0 5 6\n253 0 dm-0 1 0 2 0 3 0 4 0 0 0 5 6\n"
	tests := []struct {
		name      string
		mountLine string
	}{
		{name: "subtree", mountLine: "25 1 8:1 /subtree " + testDir + " rw - ext4 /dev/vda1 rw\n"},
		{name: "device mapper", mountLine: "25 1 253:0 / " + testDir + " rw - ext4 /dev/dm-0 rw\n"},
		{name: "btrfs", mountLine: "25 1 8:1 / " + testDir + " rw - btrfs /dev/vda1 rw\n"},
		{name: "unknown", mountLine: "25 1 8:2 / " + testDir + " rw - ext4 /dev/vdb1 rw\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useDiskStatsFixture(t, tt.mountLine, diskStats)
			_, err := (&DiskCollector{testDir: testDir}).CollectDiskStats()
			if err == nil || !strings.Contains(err.Error(), "无法识别") {
				t.Fatalf("应拒绝不可靠的设备映射: %v", err)
			}
		})
	}
}

func useDiskStatsFixture(t *testing.T, mountInfo, diskStats string) {
	t.Helper()
	fixtureDir := t.TempDir()
	mountInfoPath := filepath.Join(fixtureDir, "mountinfo")
	diskStatsPath := filepath.Join(fixtureDir, "diskstats")
	if err := os.WriteFile(mountInfoPath, []byte(mountInfo), 0o600); err != nil {
		t.Fatalf("写入 mountinfo 失败: %v", err)
	}
	if err := os.WriteFile(diskStatsPath, []byte(diskStats), 0o600); err != nil {
		t.Fatalf("写入 diskstats 失败: %v", err)
	}
	oldMountInfoPath, oldDiskStatsPath := procSelfMountinfoPath, procDiskstatsPath
	procSelfMountinfoPath, procDiskstatsPath = mountInfoPath, diskStatsPath
	t.Cleanup(func() {
		procSelfMountinfoPath, procDiskstatsPath = oldMountInfoPath, oldDiskStatsPath
	})
}
