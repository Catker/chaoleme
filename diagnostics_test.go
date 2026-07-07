package main

import (
	"strings"
	"testing"

	"github.com/Catker/chaoleme/collector"
)

func TestFormatDiagnostics(t *testing.T) {
	t.Parallel()

	message := formatDiagnostics(collector.Diagnostics{Checks: []collector.DiagnosticCheck{
		{Name: "core", Status: collector.DiagnosticOK, Detail: "ready", Critical: true},
		{Name: "optional", Status: collector.DiagnosticWarn, Detail: "missing"},
	}})

	for _, want := range []string{"超售检测环境诊断", "✅ core", "⚠️ optional", "关键采集项可用"} {
		if !strings.Contains(message, want) {
			t.Fatalf("诊断输出缺少 %q，内容:\n%s", want, message)
		}
	}
}
