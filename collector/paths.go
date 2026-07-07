package collector

var (
	procStatPath       = "/proc/stat"
	procMeminfoPath    = "/proc/meminfo"
	procLoadavgPath    = "/proc/loadavg"
	procMountsPath     = "/proc/mounts"
	procDiskstatsPath  = "/proc/diskstats"
	procPressureCPU    = "/proc/pressure/cpu"
	procPressureIO     = "/proc/pressure/io"
	procCPUInfoPath    = "/proc/cpuinfo"
	procInitCgroupPath = "/proc/1/cgroup"
	procSelfCgroupPath = "/proc/self/cgroup"

	sysBlockPath      = "/sys/block"
	sysDMIProductPath = "/sys/class/dmi/id/product_name"
	sysDMIVendorPath  = "/sys/class/dmi/id/sys_vendor"
	sysDMIBoardPath   = "/sys/class/dmi/id/board_vendor"

	cgroupCPUStatPaths = []string{
		"/sys/fs/cgroup/cpu.stat",             // cgroup v2
		"/sys/fs/cgroup/cpu/cpu.stat",         // cgroup v1
		"/sys/fs/cgroup/cpuacct/cpu.stat",     // 部分发行版
		"/sys/fs/cgroup/cpu,cpuacct/cpu.stat", // systemd v1 常见布局
	}
)
