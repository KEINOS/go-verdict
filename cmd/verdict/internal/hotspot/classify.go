package hotspot

// This file selects hotspot candidates from parsed profile rows.

import (
	"sort"
	"strings"
)

const (
	allocCumThreshold  = 10.0
	allocFlatThreshold = 5.0
	cpuCumThreshold    = 10.0
	cpuFlatThreshold   = 5.0

	rankMixed = 0
	rankCPU   = 1
	rankAlloc = 2
	rankOther = 3

	classAllocHotspot   = "alloc-hotspot"
	classCPUHotspot     = "cpu-hotspot"
	classMixedHotspot   = "mixed-hotspot"
	classNoBenchmark    = "no-benchmark"
	classNoClearHotspot = "no-clear-hotspot"
)

func choiceScore(choice Choice) float64 {
	return max(
		metricScore(choice.CPU, cpuFlatThreshold, cpuCumThreshold),
		metricScore(choice.Alloc, allocFlatThreshold, allocCumThreshold),
	)
}

func classificationRank(classification string) int {
	switch classification {
	case classMixedHotspot:
		return rankMixed
	case classCPUHotspot:
		return rankCPU
	case classAllocHotspot:
		return rankAlloc
	default:
		return rankOther
	}
}

func classify(base Result, profiles profileSet) Result {
	cpuCandidates := profileCandidates(profiles.CPU, profileCPU)
	allocCandidates := profileCandidates(profiles.Alloc, profileAlloc)
	choices := make([]Choice, 0)

	for function, cpu := range cpuCandidates {
		if alloc, ok := allocCandidates[function]; ok {
			choices = append(choices, makeChoice(classMixedHotspot, classMixedHotspot, function, cpu, alloc))
		}
	}

	if len(choices) == 0 {
		for function, cpu := range cpuCandidates {
			choices = append(choices, makeChoice(classCPUHotspot, classCPUHotspot, function, cpu, zeroPprofRow()))
		}

		for function, alloc := range allocCandidates {
			choices = append(choices, makeChoice(classAllocHotspot, classAllocHotspot, function, zeroPprofRow(), alloc))
		}
	}

	if len(choices) == 0 {
		base.Classification = classNoClearHotspot

		base.Reason = classNoClearHotspot
		if base.Caveat == "" {
			base.Caveat = "No clear user-code hotspot found for this benchmark workload."
		}

		return base
	}

	sort.SliceStable(choices, func(left int, right int) bool {
		return compareChoice(choices[left], choices[right]) < 0
	})

	primary := choices[0]
	base.Classification = primary.Classification
	base.Reason = primary.Reason
	base.Function = primary.Function
	base.CPU = primary.CPU
	base.Alloc = primary.Alloc

	if len(choices) > 1 {
		secondary := choices[1]
		base.Secondary = &secondary
	}

	return base
}

func compareChoice(left Choice, right Choice) int {
	leftRank := classificationRank(left.Classification)
	rightRank := classificationRank(right.Classification)

	if leftRank != rightRank {
		return leftRank - rightRank
	}

	leftScore := choiceScore(left)

	rightScore := choiceScore(right)
	if leftScore > rightScore {
		return -1
	}

	if leftScore < rightScore {
		return 1
	}

	if left.Function < right.Function {
		return -1
	}

	if left.Function > right.Function {
		return 1
	}

	return 0
}

func isBenchmarkFunction(function string) bool {
	lastDot := strings.LastIndex(function, ".")
	if lastDot < 0 || lastDot == len(function)-1 {
		return false
	}

	return strings.HasPrefix(function[lastDot+1:], "Benchmark")
}

func isProfileNoise(function string) bool {
	return strings.HasPrefix(function, "runtime.") ||
		strings.HasPrefix(function, "runtime/") ||
		strings.HasPrefix(function, "testing.") ||
		strings.HasPrefix(function, "testing/") ||
		strings.HasPrefix(function, "sync/atomic.") ||
		isBenchmarkFunction(function)
}

func isUserFunction(function string, prefixes []string) bool {
	if isProfileNoise(function) {
		return false
	}

	for _, prefix := range prefixes {
		if function == prefix || strings.HasPrefix(function, prefix+".") || strings.HasPrefix(function, prefix+"/") {
			return true
		}
	}

	return false
}

func makeChoice(classification string, reason string, function string, cpu pprofRow, alloc pprofRow) Choice {
	return Choice{
		Classification: classification,
		Reason:         reason,
		Function:       function,
		CPU:            cpuMetric(cpu),
		Alloc:          allocMetric(alloc),
	}
}

func meetsThreshold(row pprofRow, kind profileKind) bool {
	switch kind {
	case profileCPU:
		return row.FlatPct >= cpuFlatThreshold || row.CumPct >= cpuCumThreshold
	case profileAlloc:
		return row.FlatPct >= allocFlatThreshold || row.CumPct >= allocCumThreshold
	case profileAllocObjects, profileInuse:
		return false
	default:
		return false
	}
}

func metricScore(metric Metric, flatThreshold float64, cumThreshold float64) float64 {
	return max(metric.FlatPct/flatThreshold, metric.CumPct/cumThreshold)
}

func profileCandidates(rows map[string]pprofRow, kind profileKind) map[string]pprofRow {
	result := make(map[string]pprofRow)

	for function, row := range rows {
		if meetsThreshold(row, kind) {
			result[function] = row
		}
	}

	return result
}

func userRows(rows map[string]pprofRow, prefixes []string) map[string]pprofRow {
	result := make(map[string]pprofRow)

	for _, row := range rows {
		if !isUserFunction(row.Function, prefixes) {
			continue
		}

		result[row.Function] = row
	}

	return result
}
