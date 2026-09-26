package analyzer

import "time"

type RiskLevel string

const (
	RiskLevelUnknown   RiskLevel = "unknown"   // 数据不足，不能给出健康等级
	RiskLevelExcellent RiskLevel = "excellent" // 90-100: 优秀
	RiskLevelGood      RiskLevel = "good"      // 70-89: 良好
	RiskLevelMedium    RiskLevel = "medium"    // 50-69: 中等
	RiskLevelSevere    RiskLevel = "severe"    // 0-49: 严重
)

// BaselineQuality 表示历史趋势证据的可信程度。

type BaselineQuality string

const (
	BaselineUnavailable  BaselineQuality = "unavailable"
	BaselineWeak         BaselineQuality = "weak"
	BaselineUsable       BaselineQuality = "usable"
	BaselineContaminated BaselineQuality = "contaminated"
)

// BaselineMetricTrend 是单项指标的历史趋势对比结果。

type BaselineMetricTrend struct {
	Name              string          `json:"name"`
	Status            string          `json:"status"`
	Quality           BaselineQuality `json:"quality"`
	Samples           int             `json:"samples"`
	Days              int             `json:"days"`
	Current           float64         `json:"current"`
	BaselineMedian    float64         `json:"baseline_median"`
	BaselineP75       float64         `json:"baseline_p75"`
	BaselineP95       float64         `json:"baseline_p95"`
	DeviationPercent  float64         `json:"deviation_percent"`
	RobustZ           float64         `json:"robust_z"`
	EvidenceCandidate bool            `json:"evidence_candidate"`
}

// EvidenceLevel 表示本次报告的数据证据强度。

type EvidenceLevel string

const (
	EvidenceInsufficient EvidenceLevel = "insufficient"
	EvidenceLow          EvidenceLevel = "low"
	EvidenceMedium       EvidenceLevel = "medium"
	EvidenceHigh         EvidenceLevel = "high"
)

// OversellVerdict 是基于证据的超售判定。

type OversellVerdict string

const (
	OversellInsufficient OversellVerdict = "insufficient_data"
	OversellUnlikely     OversellVerdict = "unlikely"
	OversellPossible     OversellVerdict = "possible"
	OversellLikely       OversellVerdict = "likely"
	OversellLocalLoad    OversellVerdict = "local_load"
)

// ContentionEvent 表示报告周期内连续出现的原始资源争抢样本。
// 它仅用于展示，不参与超售结论。
type ContentionEvent struct {
	Type                string    `json:"type"`
	StartTime           time.Time `json:"start_time"`
	EndTime             time.Time `json:"end_time"`
	SampleCount         int       `json:"sample_count"`
	PeakPercent         float64   `json:"peak_percent"`
	SynchronizedSamples int       `json:"synchronized_samples"`
	Correlation         string    `json:"correlation"`
}
