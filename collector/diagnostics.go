package collector

import "fmt"

type DiagnosticStatus string

const (
	DiagnosticOK   DiagnosticStatus = "ok"
	DiagnosticWarn DiagnosticStatus = "warn"
	DiagnosticFail DiagnosticStatus = "fail"
)

type DiagnosticCheck struct {
	Name     string
	Status   DiagnosticStatus
	Detail   string
	Critical bool
}

type Diagnostics struct {
	Checks []DiagnosticCheck
}

func (d Diagnostics) ReadyForOversellDetection() bool {
	for _, check := range d.Checks {
		if check.Critical && check.Status == DiagnosticFail {
			return false
		}
	}
	return true
}

func CollectDiagnostics() Diagnostics {
	checks := []DiagnosticCheck{
		diagnoseCPUStats(),
		diagnoseHostContext(),
		diagnoseLoadAverage(),
		diagnoseCPUPressure(),
		diagnoseIOPressure(),
		diagnoseCPUThrottle(),
		diagnoseDiskStats(),
	}
	return Diagnostics{Checks: checks}
}

func diagnoseCPUStats() DiagnosticCheck {
	stats, err := readCPUStats()
	if err != nil {
		return DiagnosticCheck{Name: "/proc/stat CPU", Status: DiagnosticFail, Detail: err.Error(), Critical: true}
	}
	return DiagnosticCheck{
		Name:     "/proc/stat CPU",
		Status:   DiagnosticOK,
		Detail:   fmt.Sprintf("total=%d steal=%d iowait=%d", stats.Total(), stats.Steal, stats.IOWait),
		Critical: true,
	}
}

func diagnoseHostContext() DiagnosticCheck {
	ctx := CollectHostContext()
	status := DiagnosticOK
	if ctx.ContainerDetected || !ctx.StealDirectlyInterpretable {
		status = DiagnosticFail
	}
	return DiagnosticCheck{
		Name:     "运行环境上下文",
		Status:   status,
		Critical: true,
		Detail: fmt.Sprintf("virt=%s hypervisor=%t container=%t steal_direct=%t",
			ctx.VirtualizationType, ctx.HypervisorDetected, ctx.ContainerDetected, ctx.StealDirectlyInterpretable),
	}
}

func diagnoseLoadAverage() DiagnosticCheck {
	load, err := CollectLoadAverage()
	if err != nil {
		return DiagnosticCheck{Name: "/proc/loadavg", Status: DiagnosticWarn, Detail: err.Error()}
	}
	return DiagnosticCheck{Name: "/proc/loadavg", Status: DiagnosticOK, Detail: fmt.Sprintf("load1=%.2f load5=%.2f load15=%.2f", load.Load1, load.Load5, load.Load15)}
}

func diagnoseCPUPressure() DiagnosticCheck {
	pressure, err := CollectCPUPressure()
	if err != nil {
		return DiagnosticCheck{Name: "CPU PSI", Status: DiagnosticWarn, Detail: err.Error()}
	}
	return DiagnosticCheck{Name: "CPU PSI", Status: DiagnosticOK, Detail: fmt.Sprintf("some avg10=%.2f avg60=%.2f", pressure.SomeAvg10, pressure.SomeAvg60)}
}

func diagnoseIOPressure() DiagnosticCheck {
	pressure, err := CollectIOPressure()
	if err != nil {
		return DiagnosticCheck{Name: "IO PSI", Status: DiagnosticWarn, Detail: err.Error()}
	}
	return DiagnosticCheck{Name: "IO PSI", Status: DiagnosticOK, Detail: fmt.Sprintf("some avg10=%.2f avg60=%.2f", pressure.SomeAvg10, pressure.SomeAvg60)}
}

func diagnoseCPUThrottle() DiagnosticCheck {
	throttle, err := CollectCPUThrottle()
	if err != nil {
		return DiagnosticCheck{Name: "cgroup CPU 节流", Status: DiagnosticWarn, Detail: err.Error()}
	}
	return DiagnosticCheck{Name: "cgroup CPU 节流", Status: DiagnosticOK, Detail: fmt.Sprintf("periods=%d throttled=%d percent=%.2f", throttle.Periods, throttle.ThrottledPeriods, throttle.ThrottledPercent())}
}

func diagnoseDiskStats() DiagnosticCheck {
	disk := NewDiskCollector(1)
	stats, err := disk.CollectDiskStats()
	if err != nil {
		return DiagnosticCheck{Name: "/proc/diskstats", Status: DiagnosticWarn, Detail: err.Error()}
	}
	return DiagnosticCheck{Name: "/proc/diskstats", Status: DiagnosticOK, Detail: fmt.Sprintf("read_ops=%d write_ops=%d io_time_ms=%d", stats.ReadOps, stats.WriteOps, stats.IOTimeMs)}
}
