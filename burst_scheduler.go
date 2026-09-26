package main

import "time"

const (
	burstStealThreshold  = 8.0
	burstIOWaitThreshold = 30.0
)

// burstScheduler 只管理异常密集采样的状态，不依赖定时器或采集实现。
type burstScheduler struct {
	duration time.Duration
	until    time.Time
}

func newBurstScheduler(duration time.Duration) *burstScheduler {
	return &burstScheduler{duration: duration}
}

// Observe 根据一次成功的 CPU 采样启动或延长密集采样。
// 返回值表示本次是否触发了新的持续时间。
func (s *burstScheduler) Observe(now time.Time, observation cpuObservation) bool {
	if observation.StealPercent < burstStealThreshold && observation.IOWaitPercent < burstIOWaitThreshold {
		return false
	}
	s.until = now.Add(s.duration)
	return true
}

func (s *burstScheduler) Active(now time.Time) bool {
	return !s.until.IsZero() && now.Before(s.until)
}

func (s *burstScheduler) Remaining(now time.Time) time.Duration {
	if !s.Active(now) {
		return 0
	}
	return s.until.Sub(now)
}

// resetTimer 停止并排空过期信号后重设定时器，避免旧信号提前结束密集采样。
func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}
