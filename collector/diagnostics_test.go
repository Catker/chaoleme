package collector

import "testing"

func TestDiagnosticsReadyForOversellDetection(t *testing.T) {
	t.Parallel()

	ready := Diagnostics{Checks: []DiagnosticCheck{
		{Name: "core", Status: DiagnosticOK, Critical: true},
		{Name: "optional", Status: DiagnosticWarn},
	}}
	if !ready.ReadyForOversellDetection() {
		t.Fatal("关键检查通过时应可用于检测")
	}

	blocked := Diagnostics{Checks: []DiagnosticCheck{
		{Name: "core", Status: DiagnosticFail, Critical: true},
	}}
	if blocked.ReadyForOversellDetection() {
		t.Fatal("关键检查失败时不应标记为可检测")
	}

	weakHostContext := Diagnostics{Checks: []DiagnosticCheck{
		{Name: "运行环境上下文", Status: DiagnosticFail, Critical: true},
	}}
	if weakHostContext.ReadyForOversellDetection() {
		t.Fatal("Steal 无法直接解释时不应标记为可可靠检测")
	}
}
