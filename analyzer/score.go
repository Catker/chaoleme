package analyzer

import (
	"fmt"
	"sort"
	"time"

	"github.com/Catker/chaoleme/collector"
	"github.com/Catker/chaoleme/storage"
)

// 评分权重
const (
	WeightCPUSteal     = 0.35 // CPU Steal 权重 35%
	WeightCPUIoWait    = 0.10 // CPU IOWait 权重 10%
	WeightCPUStability = 0.10 // CPU 稳定性权重 10%
	WeightIOLatency    = 0.15 // I/O 顺序延迟权重 15%
	WeightRandomIO     = 0.10 // I/O 随机延迟权重 10%
	WeightDiskBusy     = 0.05 // 磁盘繁忙度权重 5%
	WeightMemory       = 0.10 // 内存权重 10%
	WeightBaseline     = 0.05 // 历史趋势权重 5%
	// 注意：CPU Load 不再参与独立评分，改为佐证因子
)

const maxHostContextAge = 7 * 24 * time.Hour

type HourlyStats struct {
	Hour         int     // 0-23 小时
	SampleCount  int     // 样本数量
	CPUStealAvg  float64 // CPU Steal 平均值
	CPUStealMax  float64 // CPU Steal 峰值
	CPUIoWaitAvg float64 // IOWait 平均值
	CPUIoWaitMax float64 // IOWait 峰值
}

// PeriodStats 周期统计数据
type PeriodStats struct {
	Period    string    // "daily", "weekly", "monthly"
	StartTime time.Time // 统计开始时间
	EndTime   time.Time // 统计结束时间

	// 样本数量，用于判断证据是否充分
	CPUStealSamples    int
	CPUIoWaitSamples   int
	CPUBenchSamples    int
	IOLatencySamples   int
	RandomIOSamples    int
	DiskStatsSamples   int
	MemorySamples      int
	CPULoadSamples     int
	CPUPressureSamples int
	IOPressureSamples  int
	CPUThrottleSamples int
	HostContextSamples int

	// 核心样本覆盖率。用于防止短时间密集采样误代表整个报告周期。
	CoreSampleSpanHours float64
	CoreCoveragePercent float64

	// CPU Steal 统计
	CPUStealAvg     float64
	CPUStealMax     float64
	CPUStealP95     float64
	CPUStealMaxTime time.Time // 峰值发生时间

	// 按时间加权的 Steal 统计，包含 burst 样本，用于识别被全周期均值冲淡的高峰时段争抢。
	CPUStealHighTimePercent float64   // Steal ≥10% 的时长占覆盖时间的比例
	CPUStealHighTimeHours   float64   // Steal ≥10% 的累计时长
	CPUStealWorstHourAvg    float64   // 最差 1 小时窗口的时间加权均值
	CPUStealWorstHourStart  time.Time // 最差 1 小时窗口起点

	// CPU IOWait 统计
	CPUIoWaitAvg     float64
	CPUIoWaitMax     float64
	CPUIoWaitP95     float64
	CPUIoWaitMaxTime time.Time // 峰值发生时间

	// 时段分布（用于周报/月报分析）
	HourlyBreakdown []HourlyStats

	// CPU 基准测试统计
	CPUBenchAvg float64 // 平均耗时
	CPUBenchCV  float64 // 变异系数 (Coefficient of Variation)

	// I/O 顺序延迟统计
	IOLatencyAvg float64
	IOLatencyP95 float64
	IOLatencyP99 float64

	// I/O 随机延迟统计
	RandomIOWriteAvg float64
	RandomIOReadAvg  float64
	RandomIOP95      float64
	// 可信样本：预写文件方法 + O_DIRECT，且未被 diskstats 证明读请求没有到达块设备。
	RandomIODirectIOSamples int

	// 磁盘繁忙度统计
	DiskBusyPercent       float64 // IO 时间占比（平均）
	DiskBusyP95           float64 // IO 时间占比（P95）
	DiskBusyAvailable     bool
	DiskStatsDeviceName   string
	DiskStatsDeviceStatus string

	// 资源争抢事件只展示原始采样，不影响 Verdict。
	ContentionEvents []ContentionEvent

	// 内存统计
	MemoryAvailablePercent float64

	// CPU Load 统计
	CPULoadAvg float64 // 归一化后的 load1 平均值
	CPULoadMax float64 // 归一化后的 load1 最大值

	// Linux PSI 压力统计，表示任务因资源不足产生等待的比例。
	CPUPressureSomeAvg float64
	CPUPressureSomeP95 float64
	IOPressureSomeAvg  float64
	IOPressureSomeP95  float64
	CPUThrottleAvg     float64
	CPUThrottleP95     float64

	// 历史趋势。旧字段保留，便于兼容现有 JSON 和报告。
	BaselineDeviation float64 // 历史趋势偏离度；building 时表示已有天数
	BaselineStatus    string  // "stable" / "degrading" / "improving" / "building" / "contaminated"
	BaselineMinDays   int     // 根据报告类型动态设置的最小天数要求
	BaselineQuality   BaselineQuality
	BaselineReason    string
	BaselineMetrics   []BaselineMetricTrend

	// 运行环境上下文
	HypervisorDetected         bool
	ContainerDetected          bool
	VirtualizationType         string
	StealDirectlyInterpretable bool

	// 存储类型
	StorageType collector.StorageType

	// 综合评分
	TotalScore  float64
	RiskLevel   RiskLevel
	RiskDetails map[string]string

	// 基于证据的超售结论。综合评分仅用于健康度参考。
	EvidenceLevel   EvidenceLevel
	OversellVerdict OversellVerdict
	EvidenceSummary []string
	MissingMetrics  []string
	QueryErrors     []string
}

// Analyzer 分析器
type Analyzer struct {
	store MetricStore
}

// MetricStore 抽象分析器需要的数据读取能力，便于替换数据源和单元测试。
type MetricStore interface {
	Query(metricType storage.MetricType, start, end time.Time) ([]*storage.Metric, error)
	GetLatestMetric(metricType storage.MetricType) (*storage.Metric, error)
}

// NewAnalyzer 创建分析器
// 存储类型将在 AnalyzePeriod 时根据实测的随机读延迟动态推断
func NewAnalyzer(store MetricStore) *Analyzer {
	return &Analyzer{
		store: store,
	}
}

// AnalyzePeriod 分析指定周期的数据
func (a *Analyzer) AnalyzePeriod(period string, start, end time.Time) (*PeriodStats, error) {
	stats := &PeriodStats{
		Period:      period,
		StartTime:   start,
		EndTime:     end,
		StorageType: collector.StorageTypeUnknown, // 初始为未知，后续根据延迟推断
		RiskDetails: make(map[string]string),
	}

	// 查询各类指标。查询失败会降低证据等级，避免误判为优秀。
	cpuStealAllMetrics := a.queryMetrics(stats, storage.MetricTypeCPUSteal, start, end)
	cpuStealMetrics := filterRegularMetrics(cpuStealAllMetrics)
	cpuBenchMetrics := a.queryRegularMetrics(stats, storage.MetricTypeCPUBench, start, end)
	ioLatencyMetrics := a.queryRegularMetrics(stats, storage.MetricTypeIOLatency, start, end)
	memoryMetrics := a.queryRegularMetrics(stats, storage.MetricTypeMemory, start, end)
	stats.CPUStealSamples = len(cpuStealMetrics)
	stats.CPUBenchSamples = len(cpuBenchMetrics)
	stats.IOLatencySamples = len(ioLatencyMetrics)
	stats.MemorySamples = len(memoryMetrics)

	// 读取常规采样的运行环境上下文。burst 上下文只用于事件同步说明。
	if hostContext := a.getLatestRegularMetric(stats, storage.MetricTypeHostContext, end); hostContext != nil {
		if isHostContextFresh(hostContext.Timestamp, end) {
			stats.HostContextSamples = 1
			stats.HypervisorDetected, _ = extraBool(hostContext, storage.ExtraHypervisorDetected)
			stats.ContainerDetected, _ = extraBool(hostContext, storage.ExtraContainerDetected)
			stats.StealDirectlyInterpretable, _ = extraBool(hostContext, storage.ExtraStealDirectlyInterpretable)
			if virtType, ok := extraString(hostContext, storage.ExtraVirtualizationType); ok {
				stats.VirtualizationType = virtType
			}
		}
	}

	// 计算 CPU Steal 统计
	if len(cpuStealMetrics) > 0 {
		values := extractValues(cpuStealMetrics)
		stats.CPUStealAvg = avg(values)
		stats.CPUStealMax = max(values)
		stats.CPUStealP95 = percentile(values, 95)
		// 记录峰值发生时间
		_, stats.CPUStealMaxTime = findMaxWithTime(cpuStealMetrics)
	}
	stealTimeline := analyzeStealTimeline(buildStealTimeline(cpuStealAllMetrics, cpuStealMetrics))
	stats.CPUStealHighTimePercent = stealTimeline.HighTimePercent
	stats.CPUStealHighTimeHours = stealTimeline.HighTimeHours
	if !stealTimeline.WorstWindowEnd.IsZero() {
		stats.CPUStealWorstHourAvg = stealTimeline.WorstWindowAvg
		stats.CPUStealWorstHourStart = stealTimeline.WorstWindowEnd.Add(-stealWindow)
	}

	// 计算 CPU IOWait 统计
	cpuIoWaitAllMetrics := a.queryMetrics(stats, storage.MetricTypeCPUIoWait, start, end)
	cpuIoWaitMetrics := filterRegularMetrics(cpuIoWaitAllMetrics)
	stats.CPUIoWaitSamples = len(cpuIoWaitMetrics)
	if len(cpuIoWaitMetrics) > 0 {
		values := extractValues(cpuIoWaitMetrics)
		stats.CPUIoWaitAvg = avg(values)
		stats.CPUIoWaitMax = max(values)
		stats.CPUIoWaitP95 = percentile(values, 95)
		// 记录峰值发生时间
		_, stats.CPUIoWaitMaxTime = findMaxWithTime(cpuIoWaitMetrics)
	}
	stats.CoreSampleSpanHours, stats.CoreCoveragePercent = calculateCoreSampleCoverage(cpuStealMetrics, cpuIoWaitMetrics, start, end)

	// 计算时段分布（用于周报/月报分析）
	if len(cpuStealMetrics) > 0 || len(cpuIoWaitMetrics) > 0 {
		stats.HourlyBreakdown = calculateHourlyBreakdown(cpuStealMetrics, cpuIoWaitMetrics)
	}

	// 计算 CPU 基准测试统计
	if len(cpuBenchMetrics) > 0 {
		values := extractValues(cpuBenchMetrics)
		stats.CPUBenchAvg = avg(values)
		stats.CPUBenchCV = coefficientOfVariation(values)
	}

	// 计算 I/O 延迟统计
	if len(ioLatencyMetrics) > 0 {
		values := extractValues(ioLatencyMetrics)
		stats.IOLatencyAvg = avg(values)
		stats.IOLatencyP95 = percentile(values, 95)
		stats.IOLatencyP99 = percentile(values, 99)
	}

	// 计算内存统计（使用平均可用率，而非单点值）
	if len(memoryMetrics) > 0 {
		var availPercents []float64
		for _, m := range memoryMetrics {
			if m.Extra != nil {
				if availPct, ok := m.Extra[storage.ExtraAvailablePercent].(float64); ok {
					availPercents = append(availPercents, availPct)
				}
			}
		}
		if len(availPercents) > 0 {
			stats.MemoryAvailablePercent = avg(availPercents)
		} else {
			// 降级：从 Value（使用率）计算可用率
			values := extractValues(memoryMetrics)
			stats.MemoryAvailablePercent = 100 - avg(values)
		}
	}

	// 计算 CPU Load 统计
	cpuLoadAllMetrics := a.queryMetrics(stats, storage.MetricTypeCPULoad, start, end)
	cpuLoadMetrics := filterRegularMetrics(cpuLoadAllMetrics)
	stats.CPULoadSamples = len(cpuLoadMetrics)
	if len(cpuLoadMetrics) > 0 {
		values := extractValues(cpuLoadMetrics)
		stats.CPULoadAvg = avg(values)
		stats.CPULoadMax = percentile(values, 99) // 使用 P99 作为实用峰值
	}

	// 计算 Linux PSI 压力统计
	cpuPressureAllMetrics := a.queryMetrics(stats, storage.MetricTypeCPUPressure, start, end)
	cpuPressureMetrics := filterRegularMetrics(cpuPressureAllMetrics)
	stats.CPUPressureSamples = len(cpuPressureMetrics)
	if len(cpuPressureMetrics) > 0 {
		values := calculatePressurePercents(cpuPressureMetrics)
		stats.CPUPressureSomeAvg = avg(values)
		stats.CPUPressureSomeP95 = percentile(values, 95)
	}

	cpuThrottleAllMetrics := a.queryMetrics(stats, storage.MetricTypeCPUThrottle, start, end)
	cpuThrottleMetrics := filterRegularMetrics(cpuThrottleAllMetrics)
	stats.CPUThrottleSamples = len(cpuThrottleMetrics)
	if len(cpuThrottleMetrics) > 0 {
		values := calculateCPUThrottlePercents(cpuThrottleMetrics)
		stats.CPUThrottleAvg = avg(values)
		stats.CPUThrottleP95 = percentile(values, 95)
	}

	ioPressureAllMetrics := a.queryMetrics(stats, storage.MetricTypeIOPressure, start, end)
	ioPressureMetrics := filterRegularMetrics(ioPressureAllMetrics)
	stats.IOPressureSamples = len(ioPressureMetrics)
	if len(ioPressureMetrics) > 0 {
		values := calculatePressurePercents(ioPressureMetrics)
		stats.IOPressureSomeAvg = avg(values)
		stats.IOPressureSomeP95 = percentile(values, 95)
	}

	// 计算随机 IO 统计。有可信样本时只用可信样本，避免旧方法或未落盘的样本拉低延迟。
	randomIOMetrics := a.queryRegularMetrics(stats, storage.MetricTypeRandomIO, start, end)
	stats.RandomIOSamples = len(randomIOMetrics)
	if len(randomIOMetrics) > 0 {
		var writeLatencies, readLatencies, trustedWriteLatencies, trustedReadLatencies []float64
		for _, m := range randomIOMetrics {
			wl, hasWrite := extraFloat(m, storage.ExtraWriteLatencyMS)
			rl, hasRead := extraFloat(m, storage.ExtraReadLatencyMS)
			if hasWrite {
				writeLatencies = append(writeLatencies, wl)
			}
			if hasRead {
				readLatencies = append(readLatencies, rl)
			}
			if !isTrustedRandomIOSample(m) {
				continue
			}
			stats.RandomIODirectIOSamples++
			if hasWrite {
				trustedWriteLatencies = append(trustedWriteLatencies, wl)
			}
			if hasRead {
				trustedReadLatencies = append(trustedReadLatencies, rl)
			}
		}
		if stats.RandomIODirectIOSamples > 0 {
			writeLatencies, readLatencies = trustedWriteLatencies, trustedReadLatencies
		}
		if len(writeLatencies) > 0 {
			stats.RandomIOWriteAvg = avg(writeLatencies)
			stats.RandomIOP95 = percentile(writeLatencies, 95)
		}
		if len(readLatencies) > 0 {
			stats.RandomIOReadAvg = avg(readLatencies)
		}

		// 根据平均随机读延迟推断存储类型（比读取 /sys/block 更可靠）
		if stats.RandomIOReadAvg > 0 {
			stats.StorageType = collector.DetectStorageTypeByLatency(stats.RandomIOReadAvg)
		}
	}

	// 计算磁盘繁忙度（从 disk_stats 采集的增量数据）
	diskStatsMetrics := a.queryRegularMetrics(stats, storage.MetricTypeDiskStats, start, end)
	stats.DiskStatsSamples = len(diskStatsMetrics)
	stats.DiskStatsDeviceName, stats.DiskStatsDeviceStatus = diskStatsDevice(diskStatsMetrics)
	if len(diskStatsMetrics) >= 2 && stats.DiskStatsDeviceStatus == "single" {
		busyPercents := calculateDiskBusyPercents(diskStatsMetrics)
		if len(busyPercents) > 0 {
			stats.DiskBusyPercent = avg(busyPercents)
			stats.DiskBusyP95 = percentile(busyPercents, 95)
			stats.DiskBusyAvailable = true
		}
	}

	// 异常事件保留所有采样，用于展示 burst 的现场证据；不影响常规统计和 Verdict。
	hostContextAllMetrics := a.queryMetrics(stats, storage.MetricTypeHostContext, start, end)
	stats.ContentionEvents = append(stats.ContentionEvents,
		detectContentionEvents(string(storage.MetricTypeCPUSteal), contentionStealThreshold, cpuStealAllMetrics)...)
	stats.ContentionEvents = append(stats.ContentionEvents,
		detectContentionEvents(string(storage.MetricTypeCPUIoWait), contentionIOWaitThreshold, cpuIoWaitAllMetrics)...)
	stats.ContentionEvents = correlateContentionEvents(
		stats.ContentionEvents,
		cpuStealAllMetrics,
		cpuIoWaitAllMetrics,
		cpuLoadAllMetrics,
		cpuPressureAllMetrics,
		ioPressureAllMetrics,
		cpuThrottleAllMetrics,
		hostContextAllMetrics,
	)
	sort.Slice(stats.ContentionEvents, func(i, j int) bool {
		return stats.ContentionEvents[i].StartTime.Before(stats.ContentionEvents[j].StartTime)
	})

	// 计算历史趋势。它只作为辅助趋势证据，不直接替代绝对阈值。
	a.calculateBaselineTrend(stats, period, cpuStealMetrics, ioLatencyMetrics, cpuLoadMetrics)

	stats.MissingMetrics = findMissingMetrics(stats)

	// 计算综合评分和证据判定
	a.calculateScore(stats)
	a.calculateOversellVerdict(stats)
	applyEvidenceRiskLevel(stats)

	return stats, nil
}

func (a *Analyzer) queryMetrics(stats *PeriodStats, metricType storage.MetricType, start, end time.Time) []*storage.Metric {
	metrics, err := a.store.Query(metricType, start, end)
	if err != nil {
		stats.QueryErrors = append(stats.QueryErrors, fmt.Sprintf("%s: %v", metricType, err))
		return nil
	}
	return metrics
}

func (a *Analyzer) queryRegularMetrics(stats *PeriodStats, metricType storage.MetricType, start, end time.Time) []*storage.Metric {
	return filterRegularMetrics(a.queryMetrics(stats, metricType, start, end))
}

func (a *Analyzer) getLatestRegularMetric(stats *PeriodStats, metricType storage.MetricType, reference time.Time) *storage.Metric {
	metrics := a.queryRegularMetrics(stats, metricType, reference.Add(-maxHostContextAge), reference.Add(maxHostContextAge))
	if len(metrics) == 0 {
		return nil
	}
	latest := metrics[0]
	for _, metric := range metrics[1:] {
		if metric.Timestamp.After(latest.Timestamp) {
			latest = metric
		}
	}
	return latest
}

func isHostContextFresh(metricTime, referenceTime time.Time) bool {
	if metricTime.IsZero() || referenceTime.IsZero() {
		return false
	}
	if metricTime.After(referenceTime) {
		return true
	}
	return referenceTime.Sub(metricTime) <= maxHostContextAge
}

// calculateScore 计算综合评分
func (v OversellVerdict) Label() string {
	switch v {
	case OversellLikely:
		return "🔴 高度可能超售"
	case OversellPossible:
		return "⚠️ 可能存在超售或资源争抢"
	case OversellUnlikely:
		return "✅ 暂无明显超售证据"
	case OversellLocalLoad:
		return "📊 更像本机负载导致"
	default:
		return "⚪ 数据不足，无法判定"
	}
}

func (e EvidenceLevel) Label() string {
	switch e {
	case EvidenceHigh:
		return "高"
	case EvidenceMedium:
		return "中"
	case EvidenceLow:
		return "低"
	default:
		return "不足"
	}
}

// calculateOversellVerdict 生成基于证据的超售判定。
// CPU Steal 是强证据；I/O 与历史趋势只作辅助证据，避免把本机压力误判为超售。
func (q BaselineQuality) Label() string {
	switch q {
	case BaselineUsable:
		return "可参考"
	case BaselineWeak:
		return "弱参考"
	case BaselineContaminated:
		return "疑似污染"
	default:
		return "不可用"
	}
}
