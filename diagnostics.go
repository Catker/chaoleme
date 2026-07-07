package main

import (
	"fmt"
	"strings"

	"github.com/Catker/chaoleme/collector"
)

func formatDiagnostics(diagnostics collector.Diagnostics) string {
	var result strings.Builder
	result.WriteString("🔎 超售检测环境诊断\n")
	result.WriteString("━━━━━━━━━━━━━━━━━━\n")
	for _, check := range diagnostics.Checks {
		result.WriteString(fmt.Sprintf("%s %s: %s\n", diagnosticStatusIcon(check.Status), check.Name, check.Detail))
	}
	result.WriteString("━━━━━━━━━━━━━━━━━━\n")
	if diagnostics.ReadyForOversellDetection() {
		result.WriteString("✅ 关键采集项可用\n")
	} else {
		result.WriteString("🔴 关键采集项不可用\n")
	}
	return result.String()
}

func diagnosticStatusIcon(status collector.DiagnosticStatus) string {
	switch status {
	case collector.DiagnosticOK:
		return "✅"
	case collector.DiagnosticWarn:
		return "⚠️"
	default:
		return "🔴"
	}
}
