package verdict

import (
	"path/filepath"
	"strings"
)

const (
	metricSecPerOp = "sec/op"
)

func displayLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return label
	}

	base := filepath.Base(label)
	if base == "." || base == string(filepath.Separator) {
		return label
	}

	return base
}

func displayLabelWithFallback(label, fallback string) string {
	if label != "" {
		return displayLabel(label)
	}

	return fallback
}

func classify(metric string, delta float64, significant bool) Direction {
	if !significant || delta == 0 {
		// Exact zero is a final guard after significance and min-delta checks.
		return Same
	}

	if lowerIsBetter(metric) == (delta < 0) {
		return Improved
	}

	return Worsened
}

func lowerIsBetter(metric string) bool {
	metric = strings.ToLower(metric)

	return !strings.HasSuffix(metric, "/s") &&
		metric != "speed" &&
		metric != "throughput" &&
		metric != "rate"
}
