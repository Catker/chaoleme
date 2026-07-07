package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyOversellEvidenceScriptCoversRequiredChecks(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join("scripts", "verify_oversell_evidence.sh"))
	if err != nil {
		t.Fatalf("读取实机验证脚本失败: %v", err)
	}
	script := string(content)

	for _, want := range []string{
		"CHAOLEME_BIN",
		"CHAOLEME_CONFIG",
		"--verify-evidence",
		"daily|weekly|monthly",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("实机验证脚本缺少 %q", want)
		}
	}
}

func TestDeploymentFilesCoverReleaseAndHardeningContracts(t *testing.T) {
	t.Parallel()

	release, err := os.ReadFile(filepath.Join(".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("读取 release workflow 失败: %v", err)
	}
	releaseText := string(release)
	for _, want := range []string{
		"go-version-file: go.mod",
		"update.sh",
		"uninstall.sh",
		"sha256sum",
		"release/*.sha256",
		"arch: arm",
	} {
		if !strings.Contains(releaseText, want) {
			t.Fatalf("release workflow 缺少 %q", want)
		}
	}

	ci, err := os.ReadFile(filepath.Join(".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("读取 CI workflow 失败: %v", err)
	}
	ciText := string(ci)
	for _, want := range []string{"go test ./... -timeout 60s -cover", "go vet ./...", "bash -n install.sh"} {
		if !strings.Contains(ciText, want) {
			t.Fatalf("CI workflow 缺少 %q", want)
		}
	}

	service, err := os.ReadFile("chaoleme.service")
	if err != nil {
		t.Fatalf("读取 systemd service 失败: %v", err)
	}
	serviceText := string(service)
	for _, want := range []string{"User=chaoleme", "NoNewPrivileges=true", "ProtectSystem=full"} {
		if !strings.Contains(serviceText, want) {
			t.Fatalf("systemd service 缺少 %q", want)
		}
	}
}

func TestInstallUpdateUninstallScriptsCoverSafetyContracts(t *testing.T) {
	t.Parallel()

	installData, err := os.ReadFile("install.sh")
	if err != nil {
		t.Fatalf("读取 install.sh 失败: %v", err)
	}
	installText := string(installData)
	for _, want := range []string{"Catker/chaoleme", "armv7l", "i686|i386", "useradd --system", "NoNewPrivileges=true"} {
		if !strings.Contains(installText, want) {
			t.Fatalf("install.sh 缺少 %q", want)
		}
	}

	updateData, err := os.ReadFile("update.sh")
	if err != nil {
		t.Fatalf("读取 update.sh 失败: %v", err)
	}
	updateText := string(updateData)
	for _, want := range []string{"checksum_filename", "verify_checksum", "sha256sum", "INSTALL_PATH_FILE"} {
		if !strings.Contains(updateText, want) {
			t.Fatalf("update.sh 缺少 %q", want)
		}
	}

	uninstallData, err := os.ReadFile("uninstall.sh")
	if err != nil {
		t.Fatalf("读取 uninstall.sh 失败: %v", err)
	}
	uninstallText := string(uninstallData)
	for _, want := range []string{"validate_install_dir", "/opt/chaoleme", "拒绝删除"} {
		if !strings.Contains(uninstallText, want) {
			t.Fatalf("uninstall.sh 缺少 %q", want)
		}
	}
}
