package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Catker/chaoleme/analyzer"
	"github.com/Catker/chaoleme/collector"
	"github.com/Catker/chaoleme/config"
	"github.com/Catker/chaoleme/reporter"
	"github.com/Catker/chaoleme/storage"
)

var (
	configPath        = flag.String("config", "/opt/chaoleme/config/config.yaml", "配置文件路径")
	validateOnly      = flag.Bool("validate", false, "仅验证配置文件")
	testTelegram      = flag.Bool("test-telegram", false, "测试 Telegram 连接")
	collectOnce       = flag.Bool("collect-once", false, "仅采集一次数据")
	collectFor        = flag.String("collect-for", "", "连续采样时长，如 24h")
	collectInterval   = flag.String("collect-interval", "", "连续采样间隔，如 5m；默认使用配置中的 cpu_steal_interval")
	collectIOInterval = flag.String("collect-io-interval", "", "连续采样中的重 I/O 采样间隔，如 15m；默认使用配置中的 io_test_interval")
	reportType        = flag.String("report", "", "立即生成并发送报告 (daily/weekly/monthly)")
	reportPreview     = flag.String("report-preview", "", "本地预览报告，不发送通知 (daily/weekly/monthly)")
	reportJSON        = flag.String("report-json", "", "输出机器可读 JSON 报告 (daily/weekly/monthly)")
	reportCheck       = flag.String("report-check", "", "检查报告证据并返回退出码 (daily/weekly/monthly)")
	verifyEvidence    = flag.String("verify-evidence", "", "执行环境诊断和报告证据验证 (daily/weekly/monthly)")
	version           = flag.Bool("version", false, "显示版本信息")
	diagnose          = flag.Bool("diagnose", false, "诊断当前环境是否支持超售证据采集")
)

var Version = "1.3.0"

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("chaoleme v%s\n", Version)
		return
	}

	if *diagnose {
		diagnostics := collector.CollectDiagnostics()
		fmt.Print(formatDiagnostics(diagnostics))
		if !diagnostics.ReadyForOversellDetection() {
			os.Exit(2)
		}
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
	aiAnalyzer := analyzer.NewAIAnalyzer(&cfg.AI, *configPath)

	// 仅采集一次
	if *collectOnce {
		collectAll(cpuCollector, diskCollector, memoryCollector, store)
		fmt.Println("✅ 数据采集完成")
		return
	}

	if *collectFor != "" {
		totalDuration, sampleInterval, ioInterval, err := parseCollectForOptions(
			*collectFor,
			*collectInterval,
			*collectIOInterval,
			cfg.GetCPUStealInterval(),
			cfg.GetIOTestInterval(),
		)
		if err != nil {
			log.Fatal(err)
		}
		collectForDuration(cpuCollector, diskCollector, memoryCollector, store, totalDuration, sampleInterval, ioInterval)
		return
	}

	// 本地预览报告，不调用 AI，不发送 Telegram。
	if *reportPreview != "" {
		previewReport(*reportPreview, scoreAnalyzer, telegramReporter)
		return
	}

	// 输出机器可读 JSON 报告，便于实机验证脚本读取。
	if *reportJSON != "" {
		printReportJSON(*reportJSON, scoreAnalyzer)
		return
	}

	// 检查报告证据并返回适合脚本使用的退出码。
	if *reportCheck != "" {
		os.Exit(checkReport(*reportCheck, scoreAnalyzer))
	}

	// 执行完整证据验证，适合部署后在 VPS 上直接运行。
	if *verifyEvidence != "" {
		os.Exit(verifyOversellEvidence(*verifyEvidence, scoreAnalyzer))
	}

	// 立即生成并发送报告
	if *reportType != "" {
		generateReport(*reportType, scoreAnalyzer, aiAnalyzer, telegramReporter)
		return
	}

	// 守护进程模式
	log.Println("超了么 (chaoleme) 启动...")
	runDaemon(cfg, cpuCollector, diskCollector, memoryCollector, store, scoreAnalyzer, aiAnalyzer, telegramReporter)
}
