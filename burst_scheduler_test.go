package main

import (
	"testing"
	"time"
)

func TestBurstSchedulerTriggersAndExtendsWithoutFailureSignal(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	scheduler := newBurstScheduler(10 * time.Minute)
	if scheduler.Observe(now, cpuObservation{StealPercent: 1, IOWaitPercent: 1}) {
		t.Fatal("正常样本不应触发密集采样")
	}
	if !scheduler.Observe(now, cpuObservation{StealPercent: 8}) || !scheduler.Active(now.Add(9*time.Minute)) {
		t.Fatal("Steal 达到阈值应启动密集采样")
	}
	if !scheduler.Observe(now.Add(5*time.Minute), cpuObservation{IOWaitPercent: 30}) {
		t.Fatal("异常样本应延长密集采样")
	}
	if !scheduler.Active(now.Add(14*time.Minute)) || scheduler.Active(now.Add(15*time.Minute)) {
		t.Fatal("延长后的持续时间不符合预期")
	}
}

func TestBurstSchedulerStaysActiveWhenSampleFails(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	scheduler := newBurstScheduler(10 * time.Minute)
	if !scheduler.Observe(now, cpuObservation{StealPercent: burstStealThreshold}) {
		t.Fatal("异常样本应启动密集采样")
	}
	// CPU 采集失败时调用方不会调用 Observe；状态机没有提前结束的失败路径。
	if !scheduler.Active(now.Add(9 * time.Minute)) {
		t.Fatal("采集失败不能提前结束已经启动的密集采样")
	}
}
