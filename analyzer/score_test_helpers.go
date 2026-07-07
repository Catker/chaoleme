package analyzer

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func newTestStore(t *testing.T) *storage.Storage {
	t.Helper()

	store, err := storage.New(filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("创建测试存储失败: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func saveHostContext(t *testing.T, store *storage.Storage, ts time.Time, hypervisor, container, stealDirect bool) {
	t.Helper()
	virtType := "kvm"
	if !hypervisor {
		virtType = "baremetal-or-unknown"
	}
	saveMetric(t, store, ts, storage.MetricTypeHostContext, boolValue(hypervisor), map[string]interface{}{
		storage.ExtraHypervisorDetected:         hypervisor,
		storage.ExtraContainerDetected:          container,
		storage.ExtraVirtualizationType:         virtType,
		storage.ExtraStealDirectlyInterpretable: stealDirect,
	})
}

func boolValue(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func saveMetric(t *testing.T, store *storage.Storage, ts time.Time, typ storage.MetricType, value float64, extra map[string]interface{}) {
	t.Helper()

	if err := store.Save(&storage.Metric{Timestamp: ts, Type: typ, Value: value, Extra: extra}); err != nil {
		t.Fatalf("保存指标失败: %v", err)
	}
}

func saveBaselineDays(t *testing.T, store *storage.Storage, start time.Time, days int, steal, ioLatency, load float64) {
	t.Helper()

	for day := days; day >= 1; day-- {
		base := start.AddDate(0, 0, -day)
		for _, hour := range []int{0, 6, 12, 18} {
			ts := base.Add(time.Duration(hour) * time.Hour)
			saveMetric(t, store, ts, storage.MetricTypeCPUSteal, steal, nil)
			saveMetric(t, store, ts, storage.MetricTypeIOLatency, ioLatency, nil)
			saveMetric(t, store, ts, storage.MetricTypeCPULoad, load, nil)
		}
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
