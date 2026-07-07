package collector

import "testing"

func TestParsePressureLine(t *testing.T) {
	t.Parallel()

	kind, values, err := parsePressureLine("some avg10=1.23 avg60=0.50 avg300=0.10 total=123456")
	if err != nil {
		t.Fatalf("解析 PSI 行失败: %v", err)
	}
	if kind != "some" {
		t.Fatalf("kind 不符合预期: %s", kind)
	}
	if values.avg10 != 1.23 || values.avg60 != 0.50 || values.avg300 != 0.10 || values.total != 123456 {
		t.Fatalf("解析结果不符合预期: %+v", values)
	}
}

func TestParsePressureLineRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	cases := []string{
		"some avg10=1.0",
		"some avg10=bad avg60=0.50 avg300=0.10 total=123456",
		"some avg10=1.0 avg60=0.50 avg300=0.10 total=bad",
		"some avg10",
	}
	for _, line := range cases {
		if _, _, err := parsePressureLine(line); err == nil {
			t.Fatalf("非法 PSI 行应返回错误: %q", line)
		}
	}
}
