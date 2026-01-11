package analyzer

import (
	"fmt"
	"math"
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
	WeightBaseline     = 0.05 // 基线偏离权重 5%
	// 注意：CPU Load 不再参与独立评分，改为佐证因子
)

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLevelExcellent RiskLevel = "excellent" // 90-100: 优秀
	RiskLevelGood      RiskLevel = "good"      // 70-89: 良好
	RiskLevelMedium    RiskLevel = "medium"    // 50-69: 中等
	RiskLevelSevere    RiskLevel = "severe"    // 0-49: 严重
)

// HourlyStats 小时级统计（用于时段分析）
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

	// CPU Steal 统计
	CPUStealAvg     float64
	CPUStealMax     float64
	CPUStealP95     float64
	CPUStealMaxTime time.Time // 峰值发生时间

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

	// 磁盘繁忙度统计
	DiskBusyPercent float64 // IO 时间占比（平均）
	DiskBusyP95     float64 // IO 时间占比（P95）

	// 内存统计
	MemoryAvailablePercent float64

	// CPU Load 统计
	CPULoadAvg float64 // 归一化后的 load1 平均值
	CPULoadMax float64 // 归一化后的 load1 最大值

	// 基线对比
	BaselineDeviation float64 // 基线偏离度 (0-100，0 表示无偏离)
	BaselineStatus    string  // "stable" / "degrading" / "improving"

	// 存储类型
	StorageType collector.StorageType

	// 综合评分
	TotalScore  float64
	RiskLevel   RiskLevel
	RiskDetails map[string]string
}

// Analyzer 分析器
type Analyzer struct {
	store *storage.Storage
}

// NewAnalyzer 创建分析器
// 存储类型将在 AnalyzePeriod 时根据实测的随机读延迟动态推断
func NewAnalyzer(store *storage.Storage) *Analyzer {
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

	// 查询各类指标
	cpuStealMetrics, _ := a.store.Query(storage.MetricTypeCPUSteal, start, end)
	cpuBenchMetrics, _ := a.store.Query(storage.MetricTypeCPUBench, start, end)
	ioLatencyMetrics, _ := a.store.Query(storage.MetricTypeIOLatency, start, end)
	memoryMetrics, _ := a.store.Query(storage.MetricTypeMemory, start, end)

	// 计算 CPU Steal 统计
	if len(cpuStealMetrics) > 0 {
		values := extractValues(cpuStealMetrics)
		stats.CPUStealAvg = avg(values)
		stats.CPUStealMax = percentile(values, 99) // 使用 P99 作为实用峰值，避免极端异常干扰
		stats.CPUStealP95 = percentile(values, 95)
		// 记录峰值发生时间
		_, stats.CPUStealMaxTime = findMaxWithTime(cpuStealMetrics)
	}

	// 计算 CPU IOWait 统计
	cpuIoWaitMetrics, _ := a.store.Query(storage.MetricTypeCPUIoWait, start, end)
	if len(cpuIoWaitMetrics) > 0 {
		values := extractValues(cpuIoWaitMetrics)
		stats.CPUIoWaitAvg = avg(values)
		stats.CPUIoWaitMax = percentile(values, 99) // 使用 P99 作为实用峰值
		stats.CPUIoWaitP95 = percentile(values, 95)
		// 记录峰值发生时间
		_, stats.CPUIoWaitMaxTime = findMaxWithTime(cpuIoWaitMetrics)
	}

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
				if availPct, ok := m.Extra["available_percent"].(float64); ok {
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
	cpuLoadMetrics, _ := a.store.Query(storage.MetricTypeCPULoad, start, end)
	if len(cpuLoadMetrics) > 0 {
		values := extractValues(cpuLoadMetrics)
		stats.CPULoadAvg = avg(values)
		stats.CPULoadMax = percentile(values, 99) // 使用 P99 作为实用峰值
	}

	// 计算随机 IO 统计
	randomIOMetrics, _ := a.store.Query(storage.MetricTypeRandomIO, start, end)
	if len(randomIOMetrics) > 0 {
		var writeLatencies, readLatencies []float64
		for _, m := range randomIOMetrics {
			if m.Extra != nil {
				if wl, ok := m.Extra["write_latency_ms"].(float64); ok {
					writeLatencies = append(writeLatencies, wl)
				}
				if rl, ok := m.Extra["read_latency_ms"].(float64); ok {
					readLatencies = append(readLatencies, rl)
				}
			}
		}
		if len(writeLatencies) > 0 {
			stats.RandomIOWriteAvg = avg(writeLatencies)
		}
		if len(readLatencies) > 0 {
			stats.RandomIOReadAvg = avg(readLatencies)
		}
		// P95 使用写延迟（通常更能反映问题）
		if len(writeLatencies) > 0 {
			stats.RandomIOP95 = percentile(writeLatencies, 95)
		}

		// 根据平均随机读延迟推断存储类型（比读取 /sys/block 更可靠）
		if stats.RandomIOReadAvg > 0 {
			stats.StorageType = collector.DetectStorageTypeByLatency(stats.RandomIOReadAvg)
		}
	}

	// 计算磁盘繁忙度（从 disk_stats 采集的增量数据）
	diskStatsMetrics, _ := a.store.Query(storage.MetricTypeDiskStats, start, end)
	if len(diskStatsMetrics) >= 2 {
		// 计算时间段内的平均繁忙度
		var busyPercents []float64
		for _, m := range diskStatsMetrics {
			if m.Extra != nil {
				if bp, ok := m.Extra["busy_percent"].(float64); ok {
					busyPercents = append(busyPercents, bp)
				}
			}
		}
		if len(busyPercents) > 0 {
			stats.DiskBusyPercent = avg(busyPercents)
			stats.DiskBusyP95 = percentile(busyPercents, 95) // 添加 P95 感知 IO 抖动
		}
	}

	// 计算基线偏离
	stats.BaselineDeviation, stats.BaselineStatus = a.calculateBaselineDeviation(stats, period)

	// 计算综合评分
	a.calculateScore(stats)

	return stats, nil
}

// calculateScore 计算综合评分
func (a *Analyzer) calculateScore(stats *PeriodStats) {
	var totalScore float64

	// 计算超售可信度加成（基于本地负载佐证）
	confidenceBoost := a.calculateOversellConfidenceBoost(stats)

	// 1. CPU Steal 评分 (35%) - 应用佐证因子
	cpuStealScore := a.scoreCPUSteal(stats.CPUStealAvg)
	// 当 confidenceBoost > 1 时，低分会变得更低（更严厉）
	if confidenceBoost > 1.0 && cpuStealScore < 100 {
		cpuStealScore = cpuStealScore / confidenceBoost
	}
	totalScore += cpuStealScore * WeightCPUSteal
	stats.RiskDetails["cpu_steal"] = a.describeCPUStealRisk(stats.CPUStealAvg, stats.CPUStealMax)

	// 2. CPU IOWait 评分 (10%) - 应用佐证因子
	cpuIoWaitScore := a.scoreCPUIoWait(stats.CPUIoWaitAvg)
	if confidenceBoost > 1.0 && cpuIoWaitScore < 100 {
		cpuIoWaitScore = cpuIoWaitScore / confidenceBoost
	}
	totalScore += cpuIoWaitScore * WeightCPUIoWait
	stats.RiskDetails["cpu_iowait"] = a.describeCPUIoWaitRisk(stats.CPUIoWaitAvg)

	// 3. CPU 稳定性评分 (10%)
	cpuStabilityScore := a.scoreCPUStability(stats.CPUBenchCV)
	totalScore += cpuStabilityScore * WeightCPUStability
	stats.RiskDetails["cpu_stability"] = a.describeCPUStabilityRisk(stats.CPUBenchCV)

	// 4. I/O 顺序延迟评分 (15%)
	ioScore := a.scoreIOLatency(stats.IOLatencyP95, stats.StorageType)
	totalScore += ioScore * WeightIOLatency
	stats.RiskDetails["io_latency"] = a.describeIOLatencyRisk(stats.IOLatencyP95, stats.StorageType)

	// 5. I/O 随机延迟评分 (10%)
	randomIOScore := a.scoreRandomIO(stats.RandomIOP95, stats.StorageType)
	totalScore += randomIOScore * WeightRandomIO
	stats.RiskDetails["random_io"] = a.describeRandomIORisk(stats.RandomIOWriteAvg, stats.RandomIOReadAvg, stats.StorageType)

	// 6. 磁盘繁忙度评分 (5%)
	diskBusyScore := a.scoreDiskBusy(stats.DiskBusyPercent)
	totalScore += diskBusyScore * WeightDiskBusy
	stats.RiskDetails["disk_busy"] = a.describeDiskBusyRisk(stats.DiskBusyPercent)

	// 7. 内存评分 (10%)
	memoryScore := a.scoreMemory(stats.MemoryAvailablePercent)
	totalScore += memoryScore * WeightMemory
	stats.RiskDetails["memory"] = a.describeMemoryRisk(stats.MemoryAvailablePercent)

	// 8. CPU Load - 仅作为参考显示，不参与评分
	stats.RiskDetails["cpu_load"] = a.describeCPULoadReference(stats.CPULoadAvg, stats.CPULoadMax)

	// 9. 基线偏离评分 (5%)
	baselineScore := a.scoreBaselineDeviation(stats.BaselineDeviation)
	totalScore += baselineScore * WeightBaseline
	stats.RiskDetails["baseline"] = a.describeBaselineStatus(stats.BaselineDeviation, stats.BaselineStatus)

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

// scoreRandomIO 随机 IO 延迟评分
func (a *Analyzer) scoreRandomIO(p95 float64, storageType collector.StorageType) float64 {
	// 随机 IO 通常比顺序 IO 慢，阈值放宽
	if storageType == collector.StorageTypeHDD {
		switch {
		case p95 < 100:
			return 100
		case p95 < 200:
			return 70
		case p95 < 500:
			return 40
		default:
			return 0
		}
	}

	// SSD 或未知类型
	switch {
	case p95 < 30:
		return 100
	case p95 < 80:
		return 70
	case p95 < 150:
		return 40
	default:
		return 0
	}
}

// describeRandomIORisk 描述随机 IO 风险
func (a *Analyzer) describeRandomIORisk(writeAvg, readAvg float64, storageType collector.StorageType) string {
	// 使用写延迟作为主要指标
	threshold := 30.0
	if storageType == collector.StorageTypeHDD {
		threshold = 100.0
	}

	switch {
	case writeAvg < threshold:
		return fmt.Sprintf("✅ 低 (写:%.1fms 读:%.1fms)", writeAvg, readAvg)
	case writeAvg < threshold*2.5:
		return fmt.Sprintf("⚠️ 中等 (写:%.1fms 读:%.1fms)", writeAvg, readAvg)
	default:
		return fmt.Sprintf("🔴 严重 (写:%.1fms 读:%.1fms)", writeAvg, readAvg)
	}
}

// scoreDiskBusy 磁盘繁忙度评分
func (a *Analyzer) scoreDiskBusy(busyPercent float64) float64 {
	switch {
	case busyPercent < 30:
		return 100
	case busyPercent < 60:
		return 70
	case busyPercent < 85:
		return 40
	default:
		return 0
	}
}

// describeDiskBusyRisk 描述磁盘繁忙度风险
func (a *Analyzer) describeDiskBusyRisk(busyPercent float64) string {
	switch {
	case busyPercent < 30:
		return fmt.Sprintf("✅ 低 (%.1f%%)", busyPercent)
	case busyPercent < 60:
		return fmt.Sprintf("⚠️ 中等 (%.1f%%)", busyPercent)
	default:
		return fmt.Sprintf("🔴 高 (%.1f%%)", busyPercent)
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

// calculateOversellConfidenceBoost 计算超售可信度加成
// 当本地负载低但 steal/iowait 高时，增加超售检测的可信度
func (a *Analyzer) calculateOversellConfidenceBoost(stats *PeriodStats) float64 {
	// 只有当本地负载较低时才应用加成
	if stats.CPULoadAvg >= 0.7 {
		return 1.0 // 本地负载高，不加成
	}

	// 本地负载低，检查是否有超售迹象
	hasStealIssue := stats.CPUStealAvg > 3.0
	hasIoWaitIssue := stats.CPUIoWaitAvg > 5.0

	if hasStealIssue || hasIoWaitIssue {
		// 负载越低，可信度加成越高（最高 1.2）
		boost := 1.0 + (0.7-stats.CPULoadAvg)*0.3
		if boost > 1.2 {
			boost = 1.2
		}
		return boost
	}

	return 1.0
}

// describeCPULoadReference 描述 CPU Load 参考值（不参与评分）
func (a *Analyzer) describeCPULoadReference(avg, max float64) string {
	var status string
	switch {
	case avg < 0.7:
		status = "空闲"
	case avg < 1.0:
		status = "正常"
	case avg < 2.0:
		status = "较高"
	default:
		status = "过载"
	}
	return fmt.Sprintf("📊 %.2f (%s) [参考值]", avg, status)
}

// scoreBaselineDeviation 基线偏离评分
// deviation: 0-100，0 表示无偏离
func (a *Analyzer) scoreBaselineDeviation(deviation float64) float64 {
	switch {
	case deviation < 10:
		return 100
	case deviation < 25:
		return 70
	case deviation < 50:
		return 40
	default:
		return 20
	}
}

// describeBaselineStatus 描述基线状态
func (a *Analyzer) describeBaselineStatus(deviation float64, status string) string {
	if status == "" {
		status = "stable"
	}
	switch status {
	case "stable":
		return "✅ 稳定"
	case "improving":
		return "📈 改善中"
	case "degrading":
		if deviation > 25 {
			return "🔴 明显下降"
		}
		return "⚠️ 轻微下降"
	case "building":
		// deviation 此时表示已有天数
		return fmt.Sprintf("📊 基线建立中 (%d/7天)", int(deviation))
	default:
		return "✅ 稳定"
	}
}

// calculateBaselineDeviation 计算与历史基线的偏离度
// period: daily/weekly/monthly，用于动态调整基线期长度
func (a *Analyzer) calculateBaselineDeviation(stats *PeriodStats, period string) (float64, string) {
	// 根据报告类型确定基线期长度和最小数据要求
	baselineDays := 14
	minDaysRequired := 3
	switch period {
	case "weekly":
		baselineDays = 28
		minDaysRequired = 7
	case "monthly":
		baselineDays = 60
		minDaysRequired = 14
	}

	baselineEnd := stats.StartTime
	baselineStart := baselineEnd.AddDate(0, 0, -baselineDays)

	// 获取基线期间的各项指标
	baselineSteal, _ := a.store.Query(storage.MetricTypeCPUSteal, baselineStart, baselineEnd)
	baselineIO, _ := a.store.Query(storage.MetricTypeIOLatency, baselineStart, baselineEnd)
	baselineLoad, _ := a.store.Query(storage.MetricTypeCPULoad, baselineStart, baselineEnd)

	// 计算历史数据覆盖的天数（通过检查数据点的时间跨度）
	daysWithData := a.countDaysWithData(baselineSteal, baselineIO)

	if daysWithData < minDaysRequired {
		// 返回已有天数，用于显示进度
		return float64(daysWithData), "building"
	}

	// 对于优质 VPS（指标极小），使用绝对偏差而非百分比
	// 阈值：当基线值低于此值时，使用绝对偏差
	const (
		absStealThreshold = 1.0  // Steal < 1% 时用绝对偏差
		absIOThreshold    = 15.0 // I/O < 15ms 时用绝对偏差
		absLoadThreshold  = 0.3  // Load < 0.3 时用绝对偏差
	)

	var deviations []float64

	// 计算 CPU Steal 偏离
	if len(baselineSteal) > 0 {
		baselineStealAvg := avg(extractValues(baselineSteal))
		if baselineStealAvg < absStealThreshold && stats.CPUStealAvg < absStealThreshold {
			// 两者都很小，使用绝对偏差（放大 10 倍使其有意义）
			deviations = append(deviations, (stats.CPUStealAvg-baselineStealAvg)*10)
		} else if baselineStealAvg > 0.01 {
			// 使用百分比偏差
			deviations = append(deviations, (stats.CPUStealAvg-baselineStealAvg)/baselineStealAvg*100)
		}
	}

	// 计算 I/O 延迟偏离
	if len(baselineIO) > 0 {
		baselineIOAvg := avg(extractValues(baselineIO))
		if baselineIOAvg < absIOThreshold && stats.IOLatencyAvg < absIOThreshold {
			// 两者都很小，使用绝对偏差（放大使其有意义）
			deviations = append(deviations, (stats.IOLatencyAvg-baselineIOAvg)/absIOThreshold*100)
		} else if baselineIOAvg > 0.1 {
			// 使用百分比偏差
			deviations = append(deviations, (stats.IOLatencyAvg-baselineIOAvg)/baselineIOAvg*100)
		}
	}

	// 计算 CPU Load 偏离
	if len(baselineLoad) > 0 {
		baselineLoadAvg := avg(extractValues(baselineLoad))
		if baselineLoadAvg < absLoadThreshold && stats.CPULoadAvg < absLoadThreshold {
			// 两者都很小，使用绝对偏差
			deviations = append(deviations, (stats.CPULoadAvg-baselineLoadAvg)/absLoadThreshold*100)
		} else if baselineLoadAvg > 0.01 {
			// 使用百分比偏差
			deviations = append(deviations, (stats.CPULoadAvg-baselineLoadAvg)/baselineLoadAvg*100)
		}
	}

	// 计算平均偏离度
	var totalDeviation float64
	if len(deviations) > 0 {
		for _, d := range deviations {
			totalDeviation += d
		}
		totalDeviation /= float64(len(deviations))
	}

	// 确定状态（阈值从 10 调整为 15，减少误报）
	var status string
	if totalDeviation > 15 {
		status = "degrading"
	} else if totalDeviation < -15 {
		status = "improving"
	} else {
		status = "stable"
	}

	// 返回偏离度的绝对值
	if totalDeviation < 0 {
		totalDeviation = -totalDeviation
	}

	return totalDeviation, status
}

// countDaysWithData 统计有数据的天数
func (a *Analyzer) countDaysWithData(stealMetrics, ioMetrics []*storage.Metric) int {
	daysSet := make(map[string]bool)

	for _, m := range stealMetrics {
		dayKey := m.Timestamp.Format("2006-01-02")
		daysSet[dayKey] = true
	}
	for _, m := range ioMetrics {
		dayKey := m.Timestamp.Format("2006-01-02")
		daysSet[dayKey] = true
	}

	return len(daysSet)
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

// findMaxWithTime 找到最大值及其发生时间
func findMaxWithTime(metrics []*storage.Metric) (float64, time.Time) {
	if len(metrics) == 0 {
		return 0, time.Time{}
	}

	maxVal := metrics[0].Value
	maxTime := metrics[0].Timestamp

	for _, m := range metrics[1:] {
		if m.Value > maxVal {
			maxVal = m.Value
			maxTime = m.Timestamp
		}
	}

	return maxVal, maxTime
}

// calculateHourlyBreakdown 按小时聚合 CPU Steal 和 IOWait 统计
func calculateHourlyBreakdown(stealMetrics, iowaitMetrics []*storage.Metric) []HourlyStats {
	// 按小时分组数据
	type hourData struct {
		stealValues  []float64
		iowaitValues []float64
	}

	hourlyData := make(map[int]*hourData)

	// 收集 CPU Steal 数据
	for _, m := range stealMetrics {
		hour := m.Timestamp.Hour()
		if hourlyData[hour] == nil {
			hourlyData[hour] = &hourData{}
		}
		hourlyData[hour].stealValues = append(hourlyData[hour].stealValues, m.Value)
	}

	// 收集 IOWait 数据
	for _, m := range iowaitMetrics {
		hour := m.Timestamp.Hour()
		if hourlyData[hour] == nil {
			hourlyData[hour] = &hourData{}
		}
		hourlyData[hour].iowaitValues = append(hourlyData[hour].iowaitValues, m.Value)
	}

	// 生成按小时的统计结果
	var result []HourlyStats
	for hour := 0; hour < 24; hour++ {
		data := hourlyData[hour]
		if data == nil {
			continue
		}

		hs := HourlyStats{Hour: hour}

		if len(data.stealValues) > 0 {
			hs.SampleCount = len(data.stealValues)
			hs.CPUStealAvg = avg(data.stealValues)
			hs.CPUStealMax = max(data.stealValues)
		}

		if len(data.iowaitValues) > 0 {
			if len(data.iowaitValues) > hs.SampleCount {
				hs.SampleCount = len(data.iowaitValues)
			}
			hs.CPUIoWaitAvg = avg(data.iowaitValues)
			hs.CPUIoWaitMax = max(data.iowaitValues)
		}

		result = append(result, hs)
	}

	return result
}
