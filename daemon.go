package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Catker/chaoleme/analyzer"
	"github.com/Catker/chaoleme/collector"
	"github.com/Catker/chaoleme/config"
	"github.com/Catker/chaoleme/reporter"
	"github.com/Catker/chaoleme/storage"
)

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
	dailyTime, err := time.Parse("15:04", cfg.Report.DailyTime)
	if err != nil {
		log.Fatalf("daily_time 格式无效，应为 HH:MM: %s", cfg.Report.DailyTime)
	}

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
			collectCoreMetrics(cpu, store, time.Now())

		case <-cpuBenchTicker.C:
			log.Println("[定时任务] 开始 CPU 基准测试...")
			collectCPUBenchmark(cpu, store, time.Now())

		case <-ioTestTicker.C:
			log.Println("[定时任务] 开始 I/O 测试...")
			collectIOMemoryDiskMetrics(disk, mem, store, time.Now())

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

			if shouldSendScheduledReport("daily", cfg, dailyTime, now, lastDailyReport) {
				if sendScheduledReport("daily", scoreAnalyzer, aiAnalyzer, telegramReporter) {
					lastDailyReport = now
				}
			}

			if shouldSendScheduledReport("weekly", cfg, dailyTime, now, lastWeeklyReport) {
				if sendScheduledReport("weekly", scoreAnalyzer, aiAnalyzer, telegramReporter) {
					lastWeeklyReport = now
				}
			}

			if shouldSendScheduledReport("monthly", cfg, dailyTime, now, lastMonthlyReport) {
				if sendScheduledReport("monthly", scoreAnalyzer, aiAnalyzer, telegramReporter) {
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

func shouldSendScheduledReport(reportType string, cfg *config.Config, scheduledTime, now, lastSent time.Time) bool {
	if now.Hour() != scheduledTime.Hour() || now.Minute() != scheduledTime.Minute() {
		return false
	}

	switch reportType {
	case "daily":
		return cfg.Report.Daily && !sameDate(lastSent, now)
	case "weekly":
		return cfg.Report.Weekly && int(now.Weekday()) == cfg.Report.WeeklyDay && !sameDate(lastSent, now)
	case "monthly":
		return cfg.Report.Monthly && now.Day() == cfg.Report.MonthlyDay && !sameMonth(lastSent, now)
	default:
		return false
	}
}

func sameDate(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func sameMonth(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month()
}

// sendScheduledReport 发送定时报告

func sendScheduledReport(reportType string, scoreAnalyzer *analyzer.Analyzer, aiAnalyzer *analyzer.AIAnalyzer, telegramReporter *reporter.TelegramReporter) bool {
	start, end, err := reportRange(reportType, time.Now())
	if err != nil {
		log.Print(err)
		return false
	}

	stats, err := scoreAnalyzer.AnalyzePeriod(reportType, start, end)
	if err != nil {
		log.Printf("分析 %s 数据失败: %v", reportType, err)
		return false
	}

	aiAnalysis, _ := aiAnalyzer.Analyze(stats, reportType)

	if err := telegramReporter.SendReport(stats, aiAnalysis); err != nil {
		log.Printf("发送 %s 报告失败: %v", reportType, err)
		return false
	} else {
		log.Printf("%s 报告已发送", reportType)
	}
	return true
}
