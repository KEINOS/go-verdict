// Package benchparser defines neutral parsed benchmark data shared by input decoders.
package benchparser

import "strings"

const (
	// MetricSecPerOp is the normalized time-per-operation metric.
	MetricSecPerOp = "sec/op"
	// MetricNanosecondsPerOp is the raw Go benchmark time-per-operation metric.
	MetricNanosecondsPerOp = "ns/op"
	// MetricBytesPerOp is the normalized bytes-per-operation metric.
	MetricBytesPerOp = "B/op"
	// MetricAllocsPerOp is the normalized allocations-per-operation metric.
	MetricAllocsPerOp = "allocs/op"
)

// Comparison is one decoded benchstat metric comparison.
type Comparison struct {
	Benchmark      string
	Metric         string
	BaselineLabel  string
	CandidateLabel string
	DeltaPct       float64
	PValue         float64
	ApproxEqual    bool
}

// NormalizeMetric normalizes equivalent benchmark metric names.
func NormalizeMetric(metric string) string {
	metric = strings.TrimSpace(metric)

	switch strings.ToLower(metric) {
	case "time/op", MetricNanosecondsPerOp:
		return MetricSecPerOp
	case "bytes/op", "b/op":
		return MetricBytesPerOp
	default:
		return metric
	}
}

// NormalizeRawMetric returns the supported normalized raw benchmark metric.
func NormalizeRawMetric(metric string) (string, bool) {
	switch metric {
	case MetricNanosecondsPerOp:
		return MetricSecPerOp, true
	case MetricBytesPerOp:
		return MetricBytesPerOp, true
	case MetricAllocsPerOp:
		return MetricAllocsPerOp, true
	default:
		return "", false
	}
}
