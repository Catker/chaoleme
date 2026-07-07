package analyzer

import (
	"fmt"
	"time"

	"github.com/Catker/chaoleme/storage"
)

func minCoreCoveragePercentForPeriod(period string) float64 {
	switch period {
	case "weekly":
		return 60
	case "monthly":
		return 70
	default:
		return 50
	}
}

func applyEvidenceRiskLevel(stats *PeriodStats) {
	if stats.EvidenceLevel == EvidenceInsufficient {
		stats.RiskLevel = RiskLevelUnknown
	}
}

func calculateCoreSampleCoverage(stealMetrics, iowaitMetrics []*storage.Metric, start, end time.Time) (float64, float64) {
	if len(stealMetrics) == 0 || len(iowaitMetrics) == 0 || !end.After(start) {
		return 0, 0
	}

	stealStart := stealMetrics[0].Timestamp
	stealEnd := stealMetrics[len(stealMetrics)-1].Timestamp
	iowaitStart := iowaitMetrics[0].Timestamp
	iowaitEnd := iowaitMetrics[len(iowaitMetrics)-1].Timestamp

	coverageStart := maxTime(stealStart, iowaitStart)
	coverageEnd := minTime(stealEnd, iowaitEnd)
	if !coverageEnd.After(coverageStart) {
		return 0, 0
	}

	spanHours := coverageEnd.Sub(coverageStart).Hours()
	coveragePercent := spanHours / end.Sub(start).Hours() * 100
	if coveragePercent > 100 {
		coveragePercent = 100
	}
	return spanHours, coveragePercent
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minCoreSamplesForPeriod(period string) int {
	switch period {
	case "weekly":
		return 36
	case "monthly":
		return 72
	default:
		return 12
	}
}

func findMissingMetrics(stats *PeriodStats) []string {
	var missing []string
	if stats.CPUStealSamples == 0 {
		missing = append(missing, "cpu_steal")
	}
	if stats.CPUIoWaitSamples == 0 {
		missing = append(missing, "cpu_iowait")
	}
	if stats.CPUBenchSamples < 2 {
		missing = append(missing, "cpu_bench")
	}
	if stats.IOLatencySamples == 0 {
		missing = append(missing, "io_latency")
	}
	if stats.RandomIOSamples == 0 {
		missing = append(missing, "random_io")
	} else if stats.RandomIODirectIOSamples == 0 {
		missing = append(missing, "random_io_direct")
	}
	if stats.DiskStatsSamples < 2 {
		missing = append(missing, "disk_stats")
	}
	if stats.MemorySamples == 0 {
		missing = append(missing, "memory")
	}
	if stats.CPULoadSamples == 0 {
		missing = append(missing, "cpu_load")
	}
	return missing
}

func (a *Analyzer) calculateOversellVerdict(stats *PeriodStats) {
	stats.EvidenceSummary = nil

	minCoreSamples := minCoreSamplesForPeriod(stats.Period)
	minCoreCoverage := minCoreCoveragePercentForPeriod(stats.Period)
	hasCoreSamples := stats.CPUStealSamples >= minCoreSamples && stats.CPUIoWaitSamples >= minCoreSamples
	hasCoreCoverage := stats.CoreCoveragePercent >= minCoreCoverage
	hasCoreCPU := hasCoreSamples && hasCoreCoverage
	hasLoad := stats.CPULoadSamples > 0
	stealContextWeak := stats.HostContextSamples == 0 || stats.ContainerDetected || !stats.StealDirectlyInterpretable
	lowLocalLoad := hasLoad && stats.CPULoadAvg < 0.70
	highLocalLoad := hasLoad && stats.CPULoadAvg >= 1.20

	strongSteal := stats.CPUStealAvg >= 8 || stats.CPUStealP95 >= 15
	mediumSteal := stats.CPUStealAvg >= 3 || stats.CPUStealP95 >= 8
	strongIOWait := stats.CPUIoWaitAvg >= 15 || stats.CPUIoWaitP95 >= 30
	strongCPUPressure := stats.CPUPressureSamples > 0 && (stats.CPUPressureSomeAvg >= 10 || stats.CPUPressureSomeP95 >= 20)
	strongCPUThrottle := stats.CPUThrottleSamples > 0 && (stats.CPUThrottleAvg >= 20 || stats.CPUThrottleP95 >= 50)
	strongIOPressure := stats.IOPressureSamples > 0 && (stats.IOPressureSomeAvg >= 10 || stats.IOPressureSomeP95 >= 20)
	strongRandomIO := stats.RandomIODirectIOSamples > 0 && stats.RandomIOP95 >= 150
	strongIOLatency := stats.IOLatencySamples > 0 && stats.IOLatencyP95 >= 100
	strongDiskBusy := stats.DiskStatsSamples > 1 && stats.DiskBusyP95 >= 85
	strongIO := strongIOWait || strongIOPressure || strongIOLatency || strongRandomIO || strongDiskBusy
	ioOnlyStrong := !strongIOWait && !mediumSteal && (strongIOLatency || strongRandomIO || strongDiskBusy)
	baselineDown := stats.BaselineQuality == BaselineUsable && stats.BaselineStatus == "degrading" && stats.BaselineDeviation >= 15
	benchUnstable := stats.CPUBenchSamples > 1 && stats.CPUBenchCV >= 0.15

	if len(stats.QueryErrors) > 0 {
		stats.EvidenceLevel = EvidenceInsufficient
		stats.OversellVerdict = OversellInsufficient
		stats.EvidenceSummary = append(stats.EvidenceSummary, "指标查询失败，无法形成可靠判定")
		return
	}

	if !hasCoreCPU {
		stats.EvidenceLevel = EvidenceInsufficient
		stats.OversellVerdict = OversellInsufficient
		if !hasCoreSamples {
			stats.EvidenceSummary = append(stats.EvidenceSummary, fmt.Sprintf("CPU Steal/IOWait 样本不足，至少需要 %d 个核心样本", minCoreSamples))
		}
		if !hasCoreCoverage {
			stats.EvidenceSummary = append(stats.EvidenceSummary, fmt.Sprintf("核心样本覆盖率不足，当前 %.1f%%，至少需要 %.0f%%", stats.CoreCoveragePercent, minCoreCoverage))
		}
		return
	}

	stats.EvidenceLevel = EvidenceMedium
	if hasLoad && (stats.CPUBenchSamples > 1 || stats.IOLatencySamples > 0 || stats.RandomIOSamples > 0 || stats.CPUPressureSamples > 0 || stats.IOPressureSamples > 0 || stats.CPUThrottleSamples > 0) {
		stats.EvidenceLevel = EvidenceHigh
	}

	if strongSteal {
		if stealContextWeak {
			stats.OversellVerdict = OversellPossible
			stats.EvidenceLevel = EvidenceMedium
		} else {
			stats.OversellVerdict = OversellLikely
		}
		stats.EvidenceSummary = append(stats.EvidenceSummary, "CPU Steal 已达到强证据阈值")
		if stealContextWeak {
			stats.EvidenceSummary = append(stats.EvidenceSummary, "运行环境使 CPU Steal 解释强度下降")
		}
		if lowLocalLoad {
			stats.EvidenceSummary = append(stats.EvidenceSummary, "本机负载较低，外部资源争抢可信度更高")
		}
		if highLocalLoad {
			stats.EvidenceSummary = append(stats.EvidenceSummary, "本机负载较高，但 CPU Steal 仍属于宿主机争抢证据")
		}
		if strongCPUPressure {
			stats.EvidenceSummary = append(stats.EvidenceSummary, "CPU PSI 显示存在等待压力")
		}
		if strongCPUThrottle {
			stats.EvidenceSummary = append(stats.EvidenceSummary, "同时存在 cgroup CPU 限额节流，需区分本机限额影响")
		}
		if benchUnstable {
			stats.EvidenceSummary = append(stats.EvidenceSummary, "CPU 基准测试波动明显")
		}
		return
	}

	if mediumSteal && (lowLocalLoad || strongCPUPressure || benchUnstable || baselineDown) {
		stats.OversellVerdict = OversellPossible
		stats.EvidenceSummary = append(stats.EvidenceSummary, "CPU Steal 出现持续异常")
		return
	}

	if strongCPUThrottle && !mediumSteal {
		stats.OversellVerdict = OversellLocalLoad
		stats.EvidenceSummary = append(stats.EvidenceSummary, "cgroup CPU 限额节流明显，当前异常更像本机限额导致")
		return
	}

	if highLocalLoad && !mediumSteal {
		stats.OversellVerdict = OversellLocalLoad
		stats.EvidenceSummary = append(stats.EvidenceSummary, "本机负载较高，当前异常更像自身任务导致")
		return
	}

	if strongIO && (strongIOPressure || baselineDown || (lowLocalLoad && !ioOnlyStrong)) {
		stats.OversellVerdict = OversellPossible
		stats.EvidenceLevel = EvidenceMedium
		if strongIOPressure {
			stats.EvidenceSummary = append(stats.EvidenceSummary, "IO PSI 显示存在等待压力")
		} else {
			stats.EvidenceSummary = append(stats.EvidenceSummary, "I/O 或 IOWait 异常，需要结合后续样本确认")
		}
		return
	}

	if mediumSteal || strongIO {
		stats.OversellVerdict = OversellPossible
		stats.EvidenceLevel = EvidenceLow
		stats.EvidenceSummary = append(stats.EvidenceSummary, "存在资源争抢迹象，但证据链不完整")
		return
	}

	stats.OversellVerdict = OversellUnlikely
	if stealContextWeak {
		stats.EvidenceLevel = EvidenceLow
		stats.EvidenceSummary = append(stats.EvidenceSummary, "运行环境未知或处于容器中，未发现明显证据但不能强证明无超售")
		return
	}
	stats.EvidenceSummary = append(stats.EvidenceSummary, "核心指标未达到超售证据阈值")
}
