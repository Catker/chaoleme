package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func TestParseCollectForOptions(t *testing.T) {
	t.Parallel()

	total, interval, ioInterval, err := parseCollectForOptions("24h", "5m", "15m", time.Minute, 15*time.Minute)
	if err != nil {
		t.Fatalf("解析连续采样参数失败: %v", err)
	}
	if total != 24*time.Hour || interval != 5*time.Minute || ioInterval != 15*time.Minute {
		t.Fatalf("解析结果不符合预期: total=%s interval=%s io=%s", total, interval, ioInterval)
	}

	_, defaultInterval, defaultIOInterval, err := parseCollectForOptions("1h", "", "", 2*time.Minute, 20*time.Minute)
	if err != nil {
		t.Fatalf("默认间隔解析失败: %v", err)
	}
	if defaultInterval != 2*time.Minute || defaultIOInterval != 20*time.Minute {
		t.Fatalf("默认间隔不符合预期: core=%s io=%s", defaultInterval, defaultIOInterval)
	}

	if _, _, _, err := parseCollectForOptions("0s", "5m", "15m", time.Minute, 15*time.Minute); err == nil {
		t.Fatal("collect-for 为 0 应返回错误")
	}
	if _, _, _, err := parseCollectForOptions("1h", "bad", "15m", time.Minute, 15*time.Minute); err == nil {
		t.Fatal("collect-interval 格式错误应返回错误")
	}
	if _, _, _, err := parseCollectForOptions("1h", "5m", "bad", time.Minute, 15*time.Minute); err == nil {
		t.Fatal("collect-io-interval 格式错误应返回错误")
	}
}

func TestEstimateCollectForSamples(t *testing.T) {
	t.Parallel()

	if got := estimateCollectForSamples(24*time.Hour, 5*time.Minute); got != 289 {
		t.Fatalf("24h/5m 预计样本数不符合预期: got=%d want=289", got)
	}
	if got := estimateCollectForSamples(0, 5*time.Minute); got != 0 {
		t.Fatalf("无效时长应返回 0，实际=%d", got)
	}
	if got := estimateCollectForSamples(time.Hour, 0); got != 0 {
		t.Fatalf("无效间隔应返回 0，实际=%d", got)
	}
}

func TestShouldTakeFinalSample(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	interval := 5 * time.Minute

	if !shouldTakeFinalSample(now.Add(-5*time.Minute), now, interval) {
		t.Fatal("距离上次采样达到完整间隔时应补采")
	}
	if shouldTakeFinalSample(now.Add(-3*time.Minute), now, interval) {
		t.Fatal("距离上次采样不足完整间隔时不应补采")
	}
	if shouldTakeFinalSample(now.Add(-time.Hour), now, 0) {
		t.Fatal("无效采样间隔不应补采")
	}
}

func TestSaveMetricOrLogReturnsFalseOnStorageError(t *testing.T) {
	t.Parallel()

	store, err := storage.New(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("关闭测试存储失败: %v", err)
	}

	ok := saveMetricOrLog(store, &storage.Metric{
		Timestamp: time.Now(),
		Type:      storage.MetricTypeCPUSteal,
		Value:     1,
	})
	if ok {
		t.Fatal("存储关闭后保存应返回 false")
	}
}
