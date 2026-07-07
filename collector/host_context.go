package collector

import (
	"os"
	"strings"
)

// HostContextResult 描述当前运行环境。
type HostContextResult struct {
	HypervisorDetected         bool
	ContainerDetected          bool
	VirtualizationType         string
	StealDirectlyInterpretable bool
}

func CollectHostContext() *HostContextResult {
	cpuInfo := readTextFile(procCPUInfoPath)
	dmi := strings.Join([]string{
		readTextFile(sysDMIProductPath),
		readTextFile(sysDMIVendorPath),
		readTextFile(sysDMIBoardPath),
	}, "\n")
	cgroup := strings.Join([]string{
		readTextFile(procInitCgroupPath),
		readTextFile(procSelfCgroupPath),
	}, "\n")

	virtType, hypervisorDetected := inferVirtualizationType(cpuInfo, dmi)
	containerDetected := inferContainer(cgroup)
	if fileExists("/.dockerenv") || fileExists("/run/.containerenv") {
		containerDetected = true
	}

	return &HostContextResult{
		HypervisorDetected:         hypervisorDetected,
		ContainerDetected:          containerDetected,
		VirtualizationType:         virtType,
		StealDirectlyInterpretable: hypervisorDetected && !containerDetected,
	}
}

func inferVirtualizationType(cpuInfo, dmi string) (string, bool) {
	combined := strings.ToLower(cpuInfo + "\n" + dmi)
	known := []struct {
		needle string
		name   string
	}{
		{"kvm", "kvm"},
		{"qemu", "qemu"},
		{"xen", "xen"},
		{"vmware", "vmware"},
		{"virtualbox", "virtualbox"},
		{"microsoft corporation", "hyper-v"},
		{"hyper-v", "hyper-v"},
		{"amazon ec2", "ec2"},
		{"google compute", "gce"},
		{"digitalocean", "digitalocean"},
		{"vultr", "vultr"},
		{"openstack", "openstack"},
		{"bochs", "bochs"},
	}
	for _, item := range known {
		if strings.Contains(combined, item.needle) {
			return item.name, true
		}
	}
	if strings.Contains(combined, " hypervisor ") || strings.Contains(combined, "flags") && strings.Contains(combined, "hypervisor") {
		return "unknown-hypervisor", true
	}
	return "baremetal-or-unknown", false
}

func inferContainer(cgroup string) bool {
	lower := strings.ToLower(cgroup)
	markers := []string{"docker", "kubepods", "containerd", "lxc", "libpod", "podman"}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func readTextFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
