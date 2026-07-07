package collector

import "testing"

func TestParseLoadAverageLine(t *testing.T) {
	t.Parallel()

	result, err := parseLoadAverageLine("0.12 0.34 0.56 1/100 12345")
	if err != nil {
		t.Fatalf("解析 loadavg 失败: %v", err)
	}
	if result.Load1 != 0.12 || result.Load5 != 0.34 || result.Load15 != 0.56 {
		t.Fatalf("loadavg 解析结果不符合预期: %+v", result)
	}
}

func TestParseLoadAverageLineRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	cases := []string{
		"0.12 0.34",
		"bad 0.34 0.56",
		"0.12 bad 0.56",
		"0.12 0.34 bad",
	}
	for _, line := range cases {
		if _, err := parseLoadAverageLine(line); err == nil {
			t.Fatalf("非法 loadavg 行应返回错误: %q", line)
		}
	}
}
