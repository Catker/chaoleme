package main

import (
	"testing"
	"time"

	"github.com/Catker/chaoleme/config"
)

func TestShouldSendScheduledReportChecksMinuteAndLastSent(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Report.Daily = true
	cfg.Report.Weekly = true
	cfg.Report.WeeklyDay = 2
	cfg.Report.Monthly = true
	cfg.Report.MonthlyDay = 7

	scheduled := time.Date(0, 1, 1, 9, 30, 0, 0, time.UTC)
	now := time.Date(2026, 7, 7, 9, 30, 0, 0, time.UTC)

	if !shouldSendScheduledReport("daily", cfg, scheduled, now, time.Time{}) {
		t.Fatal("日报在精确分钟内应触发")
	}
	if shouldSendScheduledReport("daily", cfg, scheduled, now.Add(time.Minute), time.Time{}) {
		t.Fatal("日报分钟不匹配时不应触发")
	}
	if shouldSendScheduledReport("daily", cfg, scheduled, now, now.Add(-time.Hour)) {
		t.Fatal("日报同一天已发送后不应重复触发")
	}
	if !shouldSendScheduledReport("weekly", cfg, scheduled, now, time.Time{}) {
		t.Fatal("周报在指定星期和精确分钟内应触发")
	}
	if shouldSendScheduledReport("weekly", cfg, scheduled, now.AddDate(0, 0, 1), time.Time{}) {
		t.Fatal("周报星期不匹配时不应触发")
	}
	if !shouldSendScheduledReport("monthly", cfg, scheduled, now, time.Time{}) {
		t.Fatal("月报在指定日期和精确分钟内应触发")
	}
	if shouldSendScheduledReport("monthly", cfg, scheduled, now.AddDate(0, 0, 1), time.Time{}) {
		t.Fatal("月报日期不匹配时不应触发")
	}
}
