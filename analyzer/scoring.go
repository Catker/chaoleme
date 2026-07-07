package analyzer

import (
	"fmt"

	"github.com/Catker/chaoleme/collector"
)

func (a *Analyzer) calculateScore(stats *PeriodStats) {
	var totalScore float64

	// 计算超售可信度加成（基于本地负载佐证）
	confidenceBoost := a.calculateOversellConfidenceBoost(stats)

	// 1. CPU Steal 评分 (35%) - 应用佐证因子
	cpuStealScore := scoreIfPresent(stats.CPUStealSamples > 0, a.scoreCPUSteal(stats.CPUStealAvg))
	// 当 confidenceBoost > 1 时，低分会变得更低（更严厉）
	if confidenceBoost > 1.0 && cpuStealScore < 100 {
		cpuStealScore = cpuStealScore / confidenceBoost
	}
	totalScore += cpuStealScore * WeightCPUSteal
	stats.RiskDetails["cpu_steal"] = describeIfPresent(stats.CPUStealSamples > 0, a.describeCPUStealRisk(stats.CPUStealAvg, stats.CPUStealMax))

	// 2. CPU IOWait 评分 (10%) - 应用佐证因子
	cpuIoWaitScore := scoreIfPresent(stats.CPUIoWaitSamples > 0, a.scoreCPUIoWait(stats.CPUIoWaitAvg))
	if confidenceBoost > 1.0 && cpuIoWaitScore < 100 {
		cpuIoWaitScore = cpuIoWaitScore / confidenceBoost
	}
	totalScore += cpuIoWaitScore * WeightCPUIoWait
	stats.RiskDetails["cpu_iowait"] = describeIfPresent(stats.CPUIoWaitSamples > 0, a.describeCPUIoWaitRisk(stats.CPUIoWaitAvg))

	// 3. CPU 稳定性评分 (10%)
	cpuStabilityScore := scoreIfPresent(stats.CPUBenchSamples > 1, a.scoreCPUStability(stats.CPUBenchCV))
	totalScore += cpuStabilityScore * WeightCPUStability
	stats.RiskDetails["cpu_stability"] = describeIfPresent(stats.CPUBenchSamples > 1, a.describeCPUStabilityRisk(stats.CPUBenchCV))

	// 4. I/O 顺序延迟评分 (15%)
	ioScore := scoreIfPresent(stats.IOLatencySamples > 0, a.scoreIOLatency(stats.IOLatencyP95, stats.StorageType))
	totalScore += ioScore * WeightIOLatency
	stats.RiskDetails["io_latency"] = describeIfPresent(stats.IOLatencySamples > 0, a.describeIOLatencyRisk(stats.IOLatencyP95, stats.StorageType))

	// 5. I/O 随机延迟评分 (10%)
	randomIOScore := scoreIfPresent(stats.RandomIOSamples > 0, a.scoreRandomIO(stats.RandomIOP95, stats.StorageType))
	totalScore += randomIOScore * WeightRandomIO
	stats.RiskDetails["random_io"] = describeIfPresent(stats.RandomIOSamples > 0, a.describeRandomIORisk(stats.RandomIOP95, stats.RandomIOReadAvg, stats.StorageType))

	// 6. 磁盘繁忙度评分 (5%)
	diskBusyScore := scoreIfPresent(stats.DiskStatsSamples > 1, a.scoreDiskBusy(stats.DiskBusyPercent))
	totalScore += diskBusyScore * WeightDiskBusy
	stats.RiskDetails["disk_busy"] = describeIfPresent(stats.DiskStatsSamples > 1, a.describeDiskBusyRisk(stats.DiskBusyPercent))

	// 7. 内存评分 (10%)
	memoryScore := scoreIfPresent(stats.MemorySamples > 0, a.scoreMemory(stats.MemoryAvailablePercent))
	totalScore += memoryScore * WeightMemory
	stats.RiskDetails["memory"] = describeIfPresent(stats.MemorySamples > 0, a.describeMemoryRisk(stats.MemoryAvailablePercent))

	// 8. CPU Load - 仅作为参考显示，不参与评分
	stats.RiskDetails["cpu_load"] = describeIfPresent(stats.CPULoadSamples > 0, a.describeCPULoadReference(stats.CPULoadAvg, stats.CPULoadMax))

	// 9. 历史趋势评分 (5%)
	baselineScore := a.scoreBaselineTrend(stats)
	totalScore += baselineScore * WeightBaseline
	stats.RiskDetails["baseline"] = a.describeBaselineStatus(stats)

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

const unknownMetricScore = 50.0

func scoreIfPresent(present bool, score float64) float64 {
	if !present {
		return unknownMetricScore
	}
	return score
}

func describeIfPresent(present bool, desc string) string {
	if !present {
		return "⚪ 数据不足"
	}
	return desc
}

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

func (a *Analyzer) describeRandomIORisk(writeP95, readAvg float64, storageType collector.StorageType) string {
	// 使用写延迟 P95 作为主要指标，和评分逻辑保持一致。
	threshold := 30.0
	if storageType == collector.StorageTypeHDD {
		threshold = 100.0
	}

	switch {
	case writeP95 < threshold:
		return fmt.Sprintf("✅ 低 (写P95:%.1fms 读均值:%.1fms)", writeP95, readAvg)
	case writeP95 < threshold*2.5:
		return fmt.Sprintf("⚠️ 中等 (写P95:%.1fms 读均值:%.1fms)", writeP95, readAvg)
	default:
		return fmt.Sprintf("🔴 严重 (写P95:%.1fms 读均值:%.1fms)", writeP95, readAvg)
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
	case availablePercent >= 50:
		return 100
	case availablePercent >= 30:
		return 80
	case availablePercent >= 15:
		return 60
	case availablePercent >= 5:
		return 30
	default:
		return 0
	}
}

// describeMemoryRisk 描述内存风险

func (a *Analyzer) describeMemoryRisk(availablePercent float64) string {
	switch {
	case availablePercent >= 30:
		return "✅ 正常"
	case availablePercent >= 15:
		return "⚠️ 偏低"
	default:
		return "🔴 不足"
	}
}

// calculateOversellConfidenceBoost 计算超售可信度加成
// 当本地负载低但 steal/iowait 高时，增加超售检测的可信度

func (a *Analyzer) calculateOversellConfidenceBoost(stats *PeriodStats) float64 {
	// 只有存在本机负载样本且负载较低时才应用加成。
	if stats.CPULoadSamples == 0 || stats.CPULoadAvg >= 0.7 {
		return 1.0
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

// scoreBaselineDeviation 历史趋势偏离评分
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

func (a *Analyzer) scoreBaselineTrend(stats *PeriodStats) float64 {
	switch stats.BaselineQuality {
	case BaselineUsable:
		return a.scoreBaselineDeviation(stats.BaselineDeviation)
	case BaselineWeak:
		score := a.scoreBaselineDeviation(stats.BaselineDeviation)
		if score > 70 {
			return 70
		}
		return score
	default:
		return unknownMetricScore
	}
}

// describeBaselineStatus 描述历史趋势状态

func (a *Analyzer) describeBaselineStatus(stats *PeriodStats) string {
	status := stats.BaselineStatus
	if status == "" {
		status = "building"
	}
	minDays := stats.BaselineMinDays
	if minDays == 0 {
		minDays = 7
	}
	quality := stats.BaselineQuality
	if quality == "" {
		quality = BaselineUnavailable
	}
	switch status {
	case "stable":
		return fmt.Sprintf("✅ 稳定 (%s)", quality.Label())
	case "improving":
		return fmt.Sprintf("📈 改善中 (%s)", quality.Label())
	case "degrading":
		if stats.BaselineDeviation > 25 {
			return fmt.Sprintf("🔴 明显下降 (%s)", quality.Label())
		}
		return fmt.Sprintf("⚠️ 轻微下降 (%s)", quality.Label())
	case "contaminated":
		return "⚠️ 历史样本疑似异常，不作为趋势证据"
	case "building":
		return fmt.Sprintf("📊 历史趋势建立中 (%d/%d天)", int(stats.BaselineDeviation), minDays)
	default:
		return fmt.Sprintf("📊 历史趋势不可用 (%s)", quality.Label())
	}
}
