package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/Catker/chaoleme/analyzer"
	"github.com/Catker/chaoleme/collector"
	"github.com/Catker/chaoleme/config"
	"github.com/Catker/chaoleme/reporter"
	"github.com/Catker/chaoleme/storage"
)

var (
	configPath   = flag.String("config", "/opt/chaoleme/config/config.yaml", "配置文件路径")
	validateOnly = flag.Bool("validate", false, "仅验证配置文件")
	testTelegram = flag.Bool("test-telegram", false, "测试 Telegram 连接")
	collectOnce  = flag.Bool("collect-once", false, "仅采集一次数据")
	reportType   = flag.String("report", "", "立即生成报告 (daily/weekly/monthly)")
	version      = flag.Bool("version", false, "显示版本信息")
)

var Version = "1.1.0"

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("chaoleme v%s\n", Version)
		return
	}

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if *validateOnly {
		fmt.Println("✅ 配置文件验证通过")
		return
	}

	// 初始化存储
	store, err := storage.New(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("初始化存储失败: %v", err)
	}
	defer store.Close()

	// 初始化 Telegram 报告器
	telegramReporter := reporter.NewTelegramReporter(&cfg.Telegram, cfg.Hostname)

	if *testTelegram {
		if err := telegramReporter.TestConnection(); err != nil {
			log.Fatalf("Telegram 连接测试失败: %v", err)
		}
		fmt.Println("✅ Telegram 连接测试成功")
		return
	}

	// 初始化采集器
	cpuCollector := collector.NewCPUCollector()
	diskCollector := collector.NewDiskCollector(cfg.Collect.IOTestSizeMB)
	memoryCollector := collector.NewMemoryCollector()

	// 初始化分析器
	scoreAnalyzer := analyzer.NewAnalyzer(store)
	aiAnalyzer := analyzer.NewAIAnalyzer(&cfg.AI)

	// 仅采集一次
	if *collectOnce {
		collectAll(cpuCollector, diskCollector, memoryCollector, store)
		fmt.Println("✅ 数据采集完成")
		return
	}

	// 立即生成报告
	if *reportType != "" {
		generateReport(*reportType, scoreAnalyzer, aiAnalyzer, telegramReporter)
		return
	}

	// 守护进程模式
	log.Println("超了么 (chaoleme) 启动...")
	runDaemon(cfg, cpuCollector, diskCollector, memoryCollector, store, scoreAnalyzer, aiAnalyzer, telegramReporter)
}

// collectAll 执行一次完整的数据采集
func collectAll(cpu *collector.CPUCollector, disk *collector.DiskCollector, mem *collector.MemoryCollector, store *storage.Storage) {
	now := time.Now()

	// CPU Usage (Steal & IOWait)
	if cpuUsage, err := cpu.Collect(); err == nil {
		store.Save(&storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUSteal,
			Value:     cpuUsage.StealPercent,
		})
		log.Printf("CPU Steal: %.2f%%", cpuUsage.StealPercent)

		store.Save(&storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUIoWait,
			Value:     cpuUsage.IOWaitPercent,
		})
		log.Printf("CPU IOWait: %.2f%%", cpuUsage.IOWaitPercent)
	} else {
		log.Printf("CPU 数据采集失败: %v", err)
	}

	// CPU 基准测试
	if result, err := cpu.RunBenchmark(); err == nil {
		store.Save(&storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPUBench,
			Value:     result.DurationMs,
		})
		log.Printf("CPU Bench: %.2fms", result.DurationMs)
	} else {
		log.Printf("CPU 基准测试失败: %v", err)
	}

	// I/O 顺序延迟
	if result, err := disk.TestWriteLatency(); err == nil {
		store.Save(&storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeIOLatency,
			Value:     result.TotalLatencyMs,
			Extra: map[string]interface{}{
				"write_latency_ms": result.WriteLatencyMs,
				"sync_latency_ms":  result.SyncLatencyMs,
			},
		})
		log.Printf("I/O Latency: %.2fms", result.TotalLatencyMs)
	} else {
		log.Printf("I/O 延迟测试失败: %v", err)
	}

	// I/O 随机读写
	if result, err := disk.TestRandomIO(); err == nil {
		store.Save(&storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeRandomIO,
			Value:     result.RandomWriteLatencyMs, // 主值使用写延迟
			Extra: map[string]interface{}{
				"write_latency_ms": result.RandomWriteLatencyMs,
				"read_latency_ms":  result.RandomReadLatencyMs,
			},
		})
		log.Printf("Random I/O: Write=%.2fms, Read=%.2fms", result.RandomWriteLatencyMs, result.RandomReadLatencyMs)
	} else {
		log.Printf("随机 I/O 测试失败: %v", err)
	}

	// 内存
	if stats, err := mem.Collect(); err == nil {
		store.Save(&storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeMemory,
			Value:     stats.UsagePercent(),
			Extra: map[string]interface{}{
				"total_kb":          stats.MemTotal,
				"available_kb":      stats.MemAvailable,
				"available_percent": stats.AvailablePercent(),
				"swap_usage":        stats.SwapUsagePercent(),
			},
		})
		log.Printf("Memory Usage: %.1f%%, Available: %.1f%%", stats.UsagePercent(), stats.AvailablePercent())
	} else {
		log.Printf("内存采集失败: %v", err)
	}

	// DiskStats 磁盘统计（从 /proc/diskstats 采集，开销极低）
	if diskStats, err := disk.CollectDiskStats(); err == nil {
		store.Save(&storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeDiskStats,
			Value:     float64(diskStats.IOTimeMs), // 主值使用累计 IO 耗时
			Extra: map[string]interface{}{
				"read_ops":       diskStats.ReadOps,
				"write_ops":      diskStats.WriteOps,
				"read_bytes":     diskStats.ReadBytes,
				"write_bytes":    diskStats.WriteBytes,
				"io_time_ms":     diskStats.IOTimeMs,
				"weighted_io_ms": diskStats.WeightedIOMs,
			},
		})
		log.Printf("Disk Stats: ReadOps=%d, WriteOps=%d, IOTime=%dms", diskStats.ReadOps, diskStats.WriteOps, diskStats.IOTimeMs)
	} else {
		log.Printf("磁盘统计采集失败: %v", err)
	}

	// Load Average
	if loadResult, err := collector.CollectLoadAverage(); err == nil {
		numCPU := float64(runtime.NumCPU())
		normalizedLoad := loadResult.Load1 / numCPU
		store.Save(&storage.Metric{
			Timestamp: now,
			Type:      storage.MetricTypeCPULoad,
			Value:     normalizedLoad,
			Extra: map[string]interface{}{
				"load1":   loadResult.Load1,
				"load5":   loadResult.Load5,
				"load15":  loadResult.Load15,
				"num_cpu": numCPU,
			},
		})
		log.Printf("CPU Load: %.2f (normalized: %.2f)", loadResult.Load1, normalizedLoad)
	} else {
		log.Printf("Load Average 采集失败: %v", err)
	}
}

// generateReport 生成并发送报告
func generateReport(reportType string, scoreAnalyzer *analyzer.Analyzer, aiAnalyzer *analyzer.AIAnalyzer, telegramReporter *reporter.TelegramReporter) {
	var start, end time.Time
	end = time.Now()

	switch reportType {
	case "daily":
		start = end.AddDate(0, 0, -1)
	case "weekly":
		start = end.AddDate(0, 0, -7)
	case "monthly":
		start = end.AddDate(0, -1, 0)
	default:
		log.Fatalf("无效的报告类型: %s", reportType)
	}

	stats, err := scoreAnalyzer.AnalyzePeriod(reportType, start, end)
	if err != nil {
		log.Fatalf("分析数据失败: %v", err)
	}

	// AI 分析
	aiAnalysis, err := aiAnalyzer.Analyze(stats, reportType)
	if err != nil {
		log.Printf("AI 分析失败 (降级为规则评分): %v", err)
	}

	// 发送报告
	if err := telegramReporter.SendReport(stats, aiAnalysis); err != nil {
		log.Fatalf("发送报告失败: %v", err)
	}

	fmt.Printf("✅ %s 报告已发送\n", reportType)
}

// runDaemon 守护进程模式
func runDaemon(cfg *config.Config, cpu *collector.CPUCollector, disk *collector.DiskCollector, mem *collector.MemoryCollector, store *storage.Storage, scoreAnalyzer *analyzer.Analyzer, aiAnalyzer *analyzer.AIAnalyzer, telegramReporter *reporter.TelegramReporter) {
	// 获取并打印采集间隔配置
	cpuStealInterval := cfg.GetCPUStealInterval()
	cpuBenchInterval := cfg.GetCPUBenchInterval()
	ioTestInterval := cfg.GetIOTestInterval()
	log.Printf("采集间隔配置: CPU Steal=%v, CPU Bench=%v, I/O Test=%v", cpuStealInterval, cpuBenchInterval, ioTestInterval)

	// 创建定时器
	cpuStealTicker := time.NewTicker(cpuStealInterval)
	cpuBenchTicker := time.NewTicker(cpuBenchInterval)
	ioTestTicker := time.NewTicker(ioTestInterval)
	cleanupTicker := time.NewTicker(24 * time.Hour)
	reportCheckTicker := time.NewTicker(1 * time.Minute) // 报告检查定时器

	// 解析日报时间
	dailyTime, _ := time.Parse("15:04", cfg.Report.DailyTime)

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 启动时先采集一次
	collectAll(cpu, disk, mem, store)

	// 上次发送报告的日期
	var lastDailyReport, lastWeeklyReport, lastMonthlyReport time.Time

	for {
		select {
		case <-cpuStealTicker.C:
			log.Println("[定时任务] 开始采集 CPU Steal/IOWait...")
			if cpuUsage, err := cpu.Collect(); err == nil {
				now := time.Now()
				// 保存 Steal
				store.Save(&storage.Metric{
					Timestamp: now,
					Type:      storage.MetricTypeCPUSteal,
					Value:     cpuUsage.StealPercent,
				})
				// 保存 IOWait
				store.Save(&storage.Metric{
					Timestamp: now,
					Type:      storage.MetricTypeCPUIoWait,
					Value:     cpuUsage.IOWaitPercent,
				})
				log.Printf("CPU Steal: %.2f%%, IOWait: %.2f%%", cpuUsage.StealPercent, cpuUsage.IOWaitPercent)
			} else {
				log.Printf("[定时任务] CPU 采集失败: %v", err)
			}

			// Load Average 采集
			if loadResult, err := collector.CollectLoadAverage(); err == nil {
				numCPU := float64(runtime.NumCPU())
				store.Save(&storage.Metric{
					Timestamp: time.Now(),
					Type:      storage.MetricTypeCPULoad,
					Value:     loadResult.Load1 / numCPU,
				})
			} else {
				log.Printf("[定时任务] Load Average 采集失败: %v", err)
			}

		case <-cpuBenchTicker.C:
			log.Println("[定时任务] 开始 CPU 基准测试...")
			if result, err := cpu.RunBenchmark(); err == nil {
				store.Save(&storage.Metric{
					Timestamp: time.Now(),
					Type:      storage.MetricTypeCPUBench,
					Value:     result.DurationMs,
				})
				log.Printf("CPU Bench: %.2fms", result.DurationMs)
			} else {
				log.Printf("[定时任务] CPU 基准测试失败: %v", err)
			}

		case <-ioTestTicker.C:
			log.Println("[定时任务] 开始 I/O 测试...")
			if result, err := disk.TestWriteLatency(); err == nil {
				store.Save(&storage.Metric{
					Timestamp: time.Now(),
					Type:      storage.MetricTypeIOLatency,
					Value:     result.TotalLatencyMs,
				})
				log.Printf("I/O Latency: %.2fms", result.TotalLatencyMs)
			} else {
				log.Printf("[定时任务] I/O 延迟测试失败: %v", err)
			}
			// 同时采集内存
			if stats, err := mem.Collect(); err == nil {
				store.Save(&storage.Metric{
					Timestamp: time.Now(),
					Type:      storage.MetricTypeMemory,
					Value:     stats.UsagePercent(),
					Extra: map[string]interface{}{
						"available_percent": stats.AvailablePercent(),
					},
				})
			} else {
				log.Printf("[定时任务] 内存采集失败: %v", err)
			}
			// 磁盘统计（从 /proc/diskstats 采集，开销极低）
			if diskStats, err := disk.CollectDiskStats(); err == nil {
				store.Save(&storage.Metric{
					Timestamp: time.Now(),
					Type:      storage.MetricTypeDiskStats,
					Value:     float64(diskStats.IOTimeMs),
					Extra: map[string]interface{}{
						"read_ops":       diskStats.ReadOps,
						"write_ops":      diskStats.WriteOps,
						"read_bytes":     diskStats.ReadBytes,
						"write_bytes":    diskStats.WriteBytes,
						"io_time_ms":     diskStats.IOTimeMs,
						"weighted_io_ms": diskStats.WeightedIOMs,
					},
				})
				log.Printf("Disk Stats: ReadOps=%d, WriteOps=%d", diskStats.ReadOps, diskStats.WriteOps)
			} else {
				log.Printf("[定时任务] 磁盘统计采集失败: %v", err)
			}

		case <-cleanupTicker.C:
			deleted, err := store.Cleanup(cfg.Storage.RetentionDays)
			if err != nil {
				log.Printf("清理过期数据失败: %v", err)
			} else if deleted > 0 {
				log.Printf("已清理 %d 条过期数据", deleted)
			}

		case <-reportCheckTicker.C:
			// 检查是否需要发送报告
			now := time.Now()

			// 日报
			if cfg.Report.Daily && now.Hour() == dailyTime.Hour() && now.Minute() == dailyTime.Minute() {
				if lastDailyReport.Day() != now.Day() {
					go sendScheduledReport("daily", scoreAnalyzer, aiAnalyzer, telegramReporter)
					lastDailyReport = now
				}
			}

			// 周报 (指定星期)
			if cfg.Report.Weekly && int(now.Weekday()) == cfg.Report.WeeklyDay && now.Hour() == dailyTime.Hour() {
				if lastWeeklyReport.YearDay() != now.YearDay() {
					go sendScheduledReport("weekly", scoreAnalyzer, aiAnalyzer, telegramReporter)
					lastWeeklyReport = now
				}
			}

			// 月报 (指定日期)
			if cfg.Report.Monthly && now.Day() == cfg.Report.MonthlyDay && now.Hour() == dailyTime.Hour() {
				if lastMonthlyReport.Month() != now.Month() {
					go sendScheduledReport("monthly", scoreAnalyzer, aiAnalyzer, telegramReporter)
					lastMonthlyReport = now
				}
			}

		case sig := <-sigCh:
			log.Printf("收到信号 %v，正在退出...", sig)
			cpuStealTicker.Stop()
			cpuBenchTicker.Stop()
			ioTestTicker.Stop()
			cleanupTicker.Stop()
			reportCheckTicker.Stop()
			return
		}
	}
}

// sendScheduledReport 发送定时报告
func sendScheduledReport(reportType string, scoreAnalyzer *analyzer.Analyzer, aiAnalyzer *analyzer.AIAnalyzer, telegramReporter *reporter.TelegramReporter) {
	var start, end time.Time
	end = time.Now()

	switch reportType {
	case "daily":
		start = end.AddDate(0, 0, -1)
	case "weekly":
		start = end.AddDate(0, 0, -7)
	case "monthly":
		start = end.AddDate(0, -1, 0)
	}

	stats, err := scoreAnalyzer.AnalyzePeriod(reportType, start, end)
	if err != nil {
		log.Printf("分析 %s 数据失败: %v", reportType, err)
		return
	}

	aiAnalysis, _ := aiAnalyzer.Analyze(stats, reportType)

	if err := telegramReporter.SendReport(stats, aiAnalysis); err != nil {
		log.Printf("发送 %s 报告失败: %v", reportType, err)
	} else {
		log.Printf("%s 报告已发送", reportType)
	}
}
