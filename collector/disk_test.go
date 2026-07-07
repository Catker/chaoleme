package collector

import "testing"

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
