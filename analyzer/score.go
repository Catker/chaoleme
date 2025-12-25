package analyzer

import (
	"math"
	"sort"
	"time"

	"github.com/Catker/chaoleme/collector"
	"github.com/Catker/chaoleme/storage"
)

// 评分权重
const (
	WeightCPUSteal     = 0.40 // CPU Steal 权重 40%
	WeightCPUIoWait    = 0.10 // CPU IOWait 权重 10%
	WeightCPUStability = 0.15 // CPU 稳定性权重 15%
	WeightIOLatency    = 0.25 // I/O 延迟权重 25%
	WeightMemory       = 0.10 // 内存权重 10%
)

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLevelExcellent RiskLevel = "excellent" // 90-100: 优秀
	RiskLevelGood      RiskLevel = "good"      // 70-89: 良好
	RiskLevelMedium    RiskLevel = "medium"    // 50-69: 中等
	RiskLevelSevere    RiskLevel = "severe"    // 0-49: 严重
)

// PeriodStats 周期统计数据
type PeriodStats struct {
	Period    string    // "daily", "weekly", "monthly"
	StartTime time.Time // 统计开始时间
	EndTime   time.Time // 统计结束时间

	// CPU Steal 统计
	CPUStealAvg float64
	CPUStealMax float64
	CPUStealP95 float64

	// CPU IOWait 统计
	CPUIoWaitAvg float64
	CPUIoWaitMax float64
	CPUIoWaitP95 float64

	// CPU 基准测试统计
	CPUBenchAvg float64 // 平均耗时
	CPUBenchCV  float64 // 变异系数 (Coefficient of Variation)

	// I/O 延迟统计
	IOLatencyAvg float64
	IOLatencyP95 float64
	IOLatencyP99 float64

	// 内存统计
	MemoryAvailablePercent float64

	// 存储类型
	StorageType collector.StorageType

	// 综合评分
	TotalScore  float64
	RiskLevel   RiskLevel
	RiskDetails map[string]string
}

// Analyzer 分析器
type Analyzer struct {
	store       *storage.Storage
	storageType collector.StorageType
}

// NewAnalyzer 创建分析器
func NewAnalyzer(store *storage.Storage) *Analyzer {
	// 检测存储类型
	diskCollector := collector.NewDiskCollector(1)
	storageType := diskCollector.DetectStorageType()

	return &Analyzer{
		store:       store,
		storageType: storageType,
	}
}

// AnalyzePeriod 分析指定周期的数据
func (a *Analyzer) AnalyzePeriod(period string, start, end time.Time) (*PeriodStats, error) {
	stats := &PeriodStats{
		Period:      period,
		StartTime:   start,
		EndTime:     end,
		StorageType: a.storageType,
		RiskDetails: make(map[string]string),
	}

	// 查询各类指标
	cpuStealMetrics, _ := a.store.Query(storage.MetricTypeCPUSteal, start, end)
	cpuBenchMetrics, _ := a.store.Query(storage.MetricTypeCPUBench, start, end)
	ioLatencyMetrics, _ := a.store.Query(storage.MetricTypeIOLatency, start, end)
	memoryMetrics, _ := a.store.Query(storage.MetricTypeMemory, start, end)

	// 计算 CPU Steal 统计
	if len(cpuStealMetrics) > 0 {
		values := extractValues(cpuStealMetrics)
		stats.CPUStealAvg = avg(values)
		stats.CPUStealMax = max(values)
		stats.CPUStealP95 = percentile(values, 95)
	}

	// 计算 CPU IOWait 统计
	cpuIoWaitMetrics, _ := a.store.Query(storage.MetricTypeCPUIoWait, start, end)
	if len(cpuIoWaitMetrics) > 0 {
		values := extractValues(cpuIoWaitMetrics)
		stats.CPUIoWaitAvg = avg(values)
		stats.CPUIoWaitMax = max(values)
		stats.CPUIoWaitP95 = percentile(values, 95)
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

	// 计算内存统计（取最新值）
	if len(memoryMetrics) > 0 {
		// 从 extra 字段获取可用率
		latest := memoryMetrics[len(memoryMetrics)-1]
		if latest.Extra != nil {
			if availPct, ok := latest.Extra["available_percent"].(float64); ok {
				stats.MemoryAvailablePercent = availPct
			}
		}
		if stats.MemoryAvailablePercent == 0 {
			stats.MemoryAvailablePercent = 100 - latest.Value // Value 存储使用率
		}
	}

	// 计算综合评分
	a.calculateScore(stats)

	return stats, nil
}

// calculateScore 计算综合评分
func (a *Analyzer) calculateScore(stats *PeriodStats) {
	var totalScore float64

	// 1. CPU Steal 评分 (40%)
	cpuStealScore := a.scoreCPUSteal(stats.CPUStealAvg)
	totalScore += cpuStealScore * WeightCPUSteal
	stats.RiskDetails["cpu_steal"] = a.describeCPUStealRisk(stats.CPUStealAvg, stats.CPUStealMax)

	// 2. CPU IOWait 评分 (10%)
	cpuIoWaitScore := a.scoreCPUIoWait(stats.CPUIoWaitAvg)
	totalScore += cpuIoWaitScore * WeightCPUIoWait
	stats.RiskDetails["cpu_iowait"] = a.describeCPUIoWaitRisk(stats.CPUIoWaitAvg)

	// 3. CPU 稳定性评分 (15%)
	cpuStabilityScore := a.scoreCPUStability(stats.CPUBenchCV)
	totalScore += cpuStabilityScore * WeightCPUStability
	stats.RiskDetails["cpu_stability"] = a.describeCPUStabilityRisk(stats.CPUBenchCV)

	// 4. I/O 延迟评分 (25%)
	ioScore := a.scoreIOLatency(stats.IOLatencyP95, stats.StorageType)
	totalScore += ioScore * WeightIOLatency
	stats.RiskDetails["io_latency"] = a.describeIOLatencyRisk(stats.IOLatencyP95, stats.StorageType)

	// 5. 内存评分 (10%)
	memoryScore := a.scoreMemory(stats.MemoryAvailablePercent)
	totalScore += memoryScore * WeightMemory
	stats.RiskDetails["memory"] = a.describeMemoryRisk(stats.MemoryAvailablePercent)

	stats.TotalScore = totalScore

	// 确定风险等级
	switch {
	case totalScore >= 90:
		stats.RiskLevel = RiskLevelExcellent
	case totalScore >= 70:
		stats.RiskLevel = RiskLevelGood
	case totalScore >= 50:
		stats.RiskLevel = RiskLevelMedium
	default:
		stats.RiskLevel = RiskLevelSevere
	}
}

// scoreCPUSteal CPU Steal 评分
func (a *Analyzer) scoreCPUSteal(avgSteal float64) float64 {
	switch {
	case avgSteal < 3:
		return 100
	case avgSteal < 8:
		return 70
	case avgSteal < 15:
		return 40
	default:
		return 0
	}
}

// describeCPUStealRisk 描述 CPU Steal 风险
func (a *Analyzer) describeCPUStealRisk(avg, max float64) string {
	switch {
	case avg < 3:
		return "✅ 低"
	case avg < 8:
		return "⚠️ 中等"
	default:
		return "🔴 严重"
	}
}

// scoreCPUIoWait CPU IOWait 评分
func (a *Analyzer) scoreCPUIoWait(avgIoWait float64) float64 {
	switch {
	case avgIoWait < 5:
		return 100
	case avgIoWait < 15:
		return 70
	case avgIoWait < 30:
		return 40
	default:
		return 0
	}
}

// describeCPUIoWaitRisk 描述 CPU IOWait 风险
func (a *Analyzer) describeCPUIoWaitRisk(avg float64) string {
	switch {
	case avg < 5:
		return "✅ 低"
	case avg < 15:
		return "⚠️ 中等"
	default:
		return "🔴 严重"
	}
}

// scoreCPUStability CPU 稳定性评分
func (a *Analyzer) scoreCPUStability(cv float64) float64 {
	switch {
	case cv < 0.05:
		return 100
	case cv < 0.15:
		return 70
	default:
		return 30
	}
}

// describeCPUStabilityRisk 描述 CPU 稳定性风险
func (a *Analyzer) describeCPUStabilityRisk(cv float64) string {
	switch {
	case cv < 0.05:
		return "✅ 稳定"
	case cv < 0.15:
		return "⚠️ 轻微波动"
	default:
		return "🔴 波动严重"
	}
}

// scoreIOLatency I/O 延迟评分
func (a *Analyzer) scoreIOLatency(p95 float64, storageType collector.StorageType) float64 {
	// SSD 和 HDD 使用不同阈值
	if storageType == collector.StorageTypeHDD {
		switch {
		case p95 < 50:
			return 100
		case p95 < 100:
			return 70
		case p95 < 200:
			return 40
		default:
			return 0
		}
	}

	// SSD 或未知类型
	switch {
	case p95 < 20:
		return 100
	case p95 < 50:
		return 70
	case p95 < 100:
		return 40
	default:
		return 0
	}
}

// describeIOLatencyRisk 描述 I/O 延迟风险
func (a *Analyzer) describeIOLatencyRisk(p95 float64, storageType collector.StorageType) string {
	threshold := 20.0
	if storageType == collector.StorageTypeHDD {
		threshold = 50.0
	}

	switch {
	case p95 < threshold:
		return "✅ 低"
	case p95 < threshold*2.5:
		return "⚠️ 中等"
	default:
		return "🔴 严重"
	}
}

// scoreMemory 内存评分
func (a *Analyzer) scoreMemory(availablePercent float64) float64 {
	switch {
	case availablePercent > 90:
		return 100
	case availablePercent > 80:
		return 80
	default:
		return 50
	}
}

// describeMemoryRisk 描述内存风险
func (a *Analyzer) describeMemoryRisk(availablePercent float64) string {
	switch {
	case availablePercent > 80:
		return "✅ 正常"
	case availablePercent > 50:
		return "⚠️ 偏低"
	default:
		return "🔴 不足"
	}
}

// 辅助函数

func extractValues(metrics []*storage.Metric) []float64 {
	values := make([]float64, len(metrics))
	for i, m := range metrics {
		values[i] = m.Value
	}
	return values
}

func avg(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	index := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func coefficientOfVariation(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	mean := avg(values)
	if mean == 0 {
		return 0
	}

	// 计算标准差
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	stdDev := math.Sqrt(sumSquares / float64(len(values)))

	return stdDev / mean
}
