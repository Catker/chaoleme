package analyzer

import (
	"testing"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func TestAnalyzePeriodMarksLikelyOversoldWithStrongStealAndLowLocalLoad(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)

	saveHostContext(t, store, start, true, false, true)
	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 10, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUPressure, 12, nil)
	}
	for i, v := range []float64{100, 108, 96} {
		saveMetric(t, store, start.Add(time.Duration(i)*20*time.Minute), storage.MetricTypeCPUBench, v, nil)
	}
	saveMetric(t, store, start.Add(5*time.Minute), storage.MetricTypeIOLatency, 5, nil)
	saveMetric(t, store, start.Add(5*time.Minute), storage.MetricTypeRandomIO, 2, map[string]interface{}{
		storage.ExtraWriteLatencyMS: 2.0,
		storage.ExtraReadLatencyMS:  1.0,
		storage.ExtraDirectIOWrite:  true,
		storage.ExtraDirectIORead:   true,
	})
	saveMetric(t, store, start.Add(5*time.Minute), storage.MetricTypeMemory, 20, map[string]interface{}{
		storage.ExtraAvailablePercent: 80.0,
	})

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellLikely {
		t.Fatalf("强 Steal 且本机低负载应判为高度可能超售，实际=%s", stats.OversellVerdict)
	}
	if stats.EvidenceLevel != EvidenceHigh {
		t.Fatalf("证据等级应为高，实际=%s", stats.EvidenceLevel)
	}
}

func TestAnalyzePeriodKeepsStrongStealLikelyEvenWithHighLocalLoad(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)

	saveHostContext(t, store, start, true, false, true)
	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 12, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 1.8, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellLikely {
		t.Fatalf("强 Steal 不应被高本机负载降级，实际=%s", stats.OversellVerdict)
	}
}

func TestAnalyzePeriodDowngradesStrongStealWhenContextIsContainer(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)
	saveHostContext(t, store, start, true, true, false)

	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 10, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellPossible {
		t.Fatalf("容器环境下强 Steal 应降为可能，实际=%s", stats.OversellVerdict)
	}
}

func TestAnalyzePeriodIgnoresStaleHostContext(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)
	saveHostContext(t, store, start.Add(-8*24*time.Hour), true, false, true)

	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 10, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.HostContextSamples != 0 {
		t.Fatalf("过旧 host_context 不应被使用，实际样本数=%d", stats.HostContextSamples)
	}
	if stats.OversellVerdict != OversellPossible {
		t.Fatalf("过旧 host_context 下强 Steal 应降级为可能，实际=%s", stats.OversellVerdict)
	}
	if stats.EvidenceLevel != EvidenceMedium {
		t.Fatalf("过旧 host_context 下证据等级应降为中，实际=%s", stats.EvidenceLevel)
	}
}

func TestAnalyzePeriodDoesNotUseCachedRandomIOAsStrongIOEvidence(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)

	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 0.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
	}
	saveMetric(t, store, start.Add(10*time.Minute), storage.MetricTypeRandomIO, 500, map[string]interface{}{
		storage.ExtraWriteLatencyMS: 500.0,
		storage.ExtraReadLatencyMS:  1.0,
		storage.ExtraDirectIOWrite:  false,
		storage.ExtraDirectIORead:   false,
	})

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellUnlikely {
		t.Fatalf("无 O_DIRECT 的随机 I/O 不应单独触发强 I/O 证据，实际=%s", stats.OversellVerdict)
	}
	if !containsString(stats.MissingMetrics, "random_io_direct") {
		t.Fatalf("缺少 O_DIRECT 时应标记 random_io_direct，实际=%v", stats.MissingMetrics)
	}
}

func TestAnalyzePeriodTreatsSingleIOLatencySpikeAsLowEvidence(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)

	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 0.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.2, nil)
	}
	saveMetric(t, store, start.Add(10*time.Minute), storage.MetricTypeIOLatency, 200, nil)

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellPossible {
		t.Fatalf("单个 I/O 异常可作为弱资源争抢迹象，实际=%s", stats.OversellVerdict)
	}
	if stats.EvidenceLevel != EvidenceLow {
		t.Fatalf("单个 I/O 异常不应升级为中/高证据，实际=%s", stats.EvidenceLevel)
	}
}

func TestAnalyzePeriodSeparatesLocalLoadFromOversell(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)

	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 0.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 1.5, nil)
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellLocalLoad {
		t.Fatalf("高本机负载且 Steal 正常时应判为本机负载，实际=%s", stats.OversellVerdict)
	}
}

func TestAnalyzePeriodSeparatesCgroupThrottleFromOversell(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	analyzer := NewAnalyzer(store)
	start := time.Now().Add(-time.Hour)

	for i := 0; i < 12; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Minute)
		saveMetric(t, store, ts, storage.MetricTypeCPUSteal, 0.2, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUIoWait, 1, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPULoad, 0.4, nil)
		saveMetric(t, store, ts, storage.MetricTypeCPUThrottle, 0, map[string]interface{}{
			storage.ExtraPeriods:          float64(100 + i*100),
			storage.ExtraThrottledPeriods: float64(10 + i*60),
		})
	}

	stats, err := analyzer.AnalyzePeriod("daily", start, start.Add(time.Hour))
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if stats.OversellVerdict != OversellLocalLoad {
		t.Fatalf("CPU 限额节流明显且 Steal 正常时应判为本机限额，实际=%s", stats.OversellVerdict)
	}
}
