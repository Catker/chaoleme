package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectCPUThrottleFileCgroupV2(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cpu.stat")
	content := []byte("usage_usec 10\nuser_usec 6\nsystem_usec 4\nnr_periods 100\nnr_throttled 25\nthrottled_usec 3000\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("写入测试 cpu.stat 失败: %v", err)
	}

	result, err := collectCPUThrottleFile(path)
	if err != nil {
		t.Fatalf("解析 cpu.stat 失败: %v", err)
	}
	if result.ThrottledPercent() != 25 {
		t.Fatalf("期望节流比例 25%%，实际 %.1f", result.ThrottledPercent())
	}
	if result.ThrottledUsec != 3000 {
		t.Fatalf("期望 throttled_usec=3000，实际=%d", result.ThrottledUsec)
	}
}

func TestCollectCPUThrottleFileCgroupV2ZeroValues(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cpu.stat")
	content := []byte("usage_usec 10\nuser_usec 6\nsystem_usec 4\nnr_periods 0\nnr_throttled 0\nthrottled_usec 0\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("写入测试 cpu.stat 失败: %v", err)
	}

	result, err := collectCPUThrottleFile(path)
	if err != nil {
		t.Fatalf("零值节流字段应视为有效采集结果: %v", err)
	}
	if result.ThrottledPercent() != 0 {
		t.Fatalf("期望节流比例 0%%，实际 %.1f", result.ThrottledPercent())
	}
	if result.ThrottledUsec != 0 {
		t.Fatalf("期望 throttled_usec=0，实际=%d", result.ThrottledUsec)
	}
}

func TestCollectCPUThrottleFileCgroupV1ThrottledTime(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cpu.stat")
	content := []byte("nr_periods 200\nnr_throttled 20\nthrottled_time 3000000\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("写入测试 cpu.stat 失败: %v", err)
	}

	result, err := collectCPUThrottleFile(path)
	if err != nil {
		t.Fatalf("解析 cpu.stat 失败: %v", err)
	}
	if result.ThrottledPercent() != 10 {
		t.Fatalf("期望节流比例 10%%，实际 %.1f", result.ThrottledPercent())
	}
	if result.ThrottledUsec != 3000 {
		t.Fatalf("期望 throttled_time 转为 3000 usec，实际=%d", result.ThrottledUsec)
	}
}

func TestCollectCPUThrottleFileRejectsMissingFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cpu.stat")
	if err := os.WriteFile(path, []byte("usage_usec 1\n"), 0o600); err != nil {
		t.Fatalf("写入测试 cpu.stat 失败: %v", err)
	}
	if _, err := collectCPUThrottleFile(path); err == nil {
		t.Fatal("缺少节流字段时应返回错误")
	}
}
