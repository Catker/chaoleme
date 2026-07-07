package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQueryReturnsErrorOnBrokenExtraJSON(t *testing.T) {
	t.Parallel()

	store, err := New(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	_, err = store.db.Exec(
		"INSERT INTO metrics (timestamp, metric_type, value, extra) VALUES (?, ?, ?, ?)",
		time.Now().Unix(),
		string(MetricTypeHostContext),
		1.0,
		"{broken-json",
	)
	if err != nil {
		t.Fatalf("写入损坏测试数据失败: %v", err)
	}

	_, err = store.Query(MetricTypeHostContext, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err == nil || !strings.Contains(err.Error(), "解析 extra JSON 失败") {
		t.Fatalf("损坏 extra JSON 应返回明确错误，实际=%v", err)
	}
}

func TestNewConfiguresSQLitePragmas(t *testing.T) {
	t.Parallel()

	store, err := New(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var journalMode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("读取 journal_mode 失败: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode 应为 wal，实际=%s", journalMode)
	}

	var busyTimeout int
	if err := store.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("读取 busy_timeout 失败: %v", err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("busy_timeout 应至少 5000ms，实际=%d", busyTimeout)
	}
}

func TestSaveQueryLatestAndCleanup(t *testing.T) {
	t.Parallel()

	store, err := New(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	old := time.Now().AddDate(0, 0, -10)
	now := time.Now()
	if err := store.Save(&Metric{
		Timestamp: old,
		Type:      MetricTypeCPUSteal,
		Value:     1,
		Extra:     map[string]interface{}{ExtraLoad1: 0.1},
	}); err != nil {
		t.Fatalf("保存旧指标失败: %v", err)
	}
	if err := store.Save(&Metric{
		Timestamp: now,
		Type:      MetricTypeCPUSteal,
		Value:     2,
		Extra:     map[string]interface{}{ExtraLoad1: 0.2},
	}); err != nil {
		t.Fatalf("保存新指标失败: %v", err)
	}

	metrics, err := store.Query(MetricTypeCPUSteal, old.Add(-time.Second), now.Add(time.Second))
	if err != nil {
		t.Fatalf("查询指标失败: %v", err)
	}
	if len(metrics) != 2 {
		t.Fatalf("期望 2 条指标，实际=%d", len(metrics))
	}
	if metrics[1].Extra[ExtraLoad1].(float64) != 0.2 {
		t.Fatalf("extra 未正确反序列化: %+v", metrics[1].Extra)
	}

	latest, err := store.GetLatestMetric(MetricTypeCPUSteal)
	if err != nil {
		t.Fatalf("获取最新指标失败: %v", err)
	}
	if latest == nil || latest.Value != 2 {
		t.Fatalf("最新指标不符合预期: %+v", latest)
	}

	deleted, err := store.Cleanup(1)
	if err != nil {
		t.Fatalf("清理旧指标失败: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("期望清理 1 条旧指标，实际=%d", deleted)
	}
}

func TestGetLatestMetricReturnsNilWhenMissing(t *testing.T) {
	t.Parallel()

	store, err := New(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	latest, err := store.GetLatestMetric(MetricTypeMemory)
	if err != nil {
		t.Fatalf("获取不存在指标不应失败: %v", err)
	}
	if latest != nil {
		t.Fatalf("不存在指标应返回 nil，实际=%+v", latest)
	}
}
