package collector

import "testing"

func TestInferVirtualizationTypeFromCPUInfo(t *testing.T) {
	t.Parallel()

	virt, detected := inferVirtualizationType("flags: fpu vme hypervisor\n", "")
	if !detected || virt != "unknown-hypervisor" {
		t.Fatalf("期望识别未知虚拟化，实际 virt=%s detected=%t", virt, detected)
	}
}

func TestInferVirtualizationTypeFromDMI(t *testing.T) {
	t.Parallel()

	virt, detected := inferVirtualizationType("", "Amazon EC2")
	if !detected || virt != "ec2" {
		t.Fatalf("期望识别 EC2，实际 virt=%s detected=%t", virt, detected)
	}
}

func TestInferContainer(t *testing.T) {
	t.Parallel()

	if !inferContainer("0::/kubepods.slice/containerd/foo") {
		t.Fatal("期望识别容器环境")
	}
	if inferContainer("0::/user.slice/user-1000.slice") {
		t.Fatal("普通 cgroup 不应识别为容器")
	}
}
