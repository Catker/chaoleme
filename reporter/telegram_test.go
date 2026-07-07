package reporter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Catker/chaoleme/analyzer"
	"github.com/Catker/chaoleme/config"
)

func TestFormatReportShowsVerdictAndHealthSeparately(t *testing.T) {
	t.Parallel()

	reporter := NewTelegramReporter(&config.TelegramConfig{}, "test-host")
	stats := &analyzer.PeriodStats{
		Period:                     "daily",
		EndTime:                    time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		RiskDetails:                map[string]string{},
		OversellVerdict:            analyzer.OversellPossible,
		EvidenceLevel:              analyzer.EvidenceMedium,
		EvidenceSummary:            []string{"CPU Steal 出现持续异常"},
		CoreCoveragePercent:        75,
		CoreSampleSpanHours:        18,
		TotalScore:                 72,
		RiskLevel:                  analyzer.RiskLevelGood,
		HostContextSamples:         1,
		VirtualizationType:         "kvm",
		HypervisorDetected:         true,
		StealDirectlyInterpretable: true,
	}

	message := reporter.FormatReport(stats, "")
	for _, want := range []string{"🧭 超售判定", "🔎 证据等级", "🧱 运行环境", "📈 健康评分", "📋 健康等级"} {
		if !strings.Contains(message, want) {
			t.Fatalf("报告缺少 %q，内容:\n%s", want, message)
		}
	}
	if strings.Contains(message, "严重超售") || strings.Contains(message, "无超售迹象") {
		t.Fatalf("健康等级不应直接写成超售结论，内容:\n%s", message)
	}
}

func TestSplitTelegramMessageRespectsSafeLimit(t *testing.T) {
	t.Parallel()

	message := strings.Repeat("长", telegramSafeMessageLimit+50)
	chunks := splitTelegramMessage(message, telegramSafeMessageLimit)
	if len(chunks) != 2 {
		t.Fatalf("长消息应拆为 2 段，实际=%d", len(chunks))
	}
	for i, chunk := range chunks {
		if got := len([]rune(chunk)); got > telegramSafeMessageLimit {
			t.Fatalf("第 %d 段超过安全长度: %d", i+1, got)
		}
	}
}

func TestSendReportSplitsLongMessageAndEscapesHTML(t *testing.T) {
	t.Parallel()

	var messages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("解析请求失败: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		messages = append(messages, payload["text"].(string))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	reporter := NewTelegramReporter(&config.TelegramConfig{BotToken: "token", ChatID: "chat"}, "test-host")
	targetURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("解析测试服务 URL 失败: %v", err)
	}
	reporter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			cloned := req.Clone(req.Context())
			cloned.URL = targetURL
			cloned.Host = targetURL.Host
			return http.DefaultTransport.RoundTrip(cloned)
		}),
	}

	stats := &analyzer.PeriodStats{
		Period:          "daily",
		EndTime:         time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		RiskDetails:     map[string]string{},
		OversellVerdict: analyzer.OversellUnlikely,
		EvidenceLevel:   analyzer.EvidenceHigh,
		RiskLevel:       analyzer.RiskLevelGood,
	}
	if err := reporter.SendReport(stats, strings.Repeat("<tag>&", 900)); err != nil {
		t.Fatalf("发送长报告失败: %v", err)
	}
	if len(messages) < 2 {
		t.Fatalf("长报告应拆分发送，实际请求数=%d", len(messages))
	}
	for i, message := range messages {
		if got := len([]rune(message)); got > telegramSafeMessageLimit {
			t.Fatalf("第 %d 段超过安全长度: %d", i+1, got)
		}
	}
	joined := strings.Join(messages, "")
	if !strings.Contains(joined, "&lt;tag&gt;&amp;") {
		t.Fatalf("消息未进行 HTML 转义: %s", joined)
	}
}

func TestTelegramErrorSanitizesBotToken(t *testing.T) {
	t.Parallel()

	reporter := NewTelegramReporter(&config.TelegramConfig{BotToken: "secret-token", ChatID: "chat"}, "test-host")
	got := reporter.sanitizeErrorText("request failed for secret-token")
	if strings.Contains(got, "secret-token") {
		t.Fatalf("错误信息不应包含 bot token: %s", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
