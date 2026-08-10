package hotspot

// This file selects hotspot candidates from parsed profile rows.

import (
	"cmp"
	"slices"
	"sort"
	"strings"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/complexity"
)

const (
	allocCumThreshold  = 10.0
	allocFlatThreshold = 5.0
	cpuCumThreshold    = 10.0
	cpuFlatThreshold   = 5.0

	// Complexity thresholds mark code worth reading, not code that is slow.
	cognitiveThreshold  = 15.0
	cyclomaticThreshold = 10.0

	rankMixed = 0
	rankCPU   = 1
	rankAlloc = 2
	rankOther = 3

	classAllocHotspot      = "alloc-hotspot"
	classComplexityHotspot = "complexity-hotspot"
	classCPUHotspot        = "cpu-hotspot"
	classMixedHotspot      = "mixed-hotspot"
	classNoBenchmark       = "no-benchmark"
	classNoClearHotspot    = "no-clear-hotspot"
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

func classify(base Result, profiles profileSet, static map[string]complexity.Stat) Result {
	choices := profileChoices(profiles)
	if len(choices) == 0 {
		return withoutProfileHotspot(base, static)
	}

	sort.SliceStable(choices, func(left int, right int) bool {
		return compareChoice(choices[left], choices[right]) < 0
	})

	primary := enrichChoice(choices[0], static)
	base.Classification = primary.Classification
	base.Reason = primary.Reason
	base.Function = primary.Function
	base.File = primary.File
	base.Line = primary.Line
	base.CPU = primary.CPU
	base.Alloc = primary.Alloc
	base.Complexity = primary.Complexity

	if len(choices) > 1 {
		secondary := enrichChoice(choices[1], static)
		base.Secondary = &secondary
	}

	return base
}

// profileChoices turns the profile rows above the thresholds into candidates.
// A function that is hot in both CPU and allocations outranks the rest, so the
// single-signal candidates are only built when no mixed candidate exists.
func profileChoices(profiles profileSet) []Choice {
	cpuCandidates := profileCandidates(profiles.CPU, profileCPU)
	allocCandidates := profileCandidates(profiles.Alloc, profileAlloc)
	choices := make([]Choice, 0)

	for function, cpu := range cpuCandidates {
		if alloc, ok := allocCandidates[function]; ok {
			choices = append(choices, makeChoice(classMixedHotspot, classMixedHotspot, function, cpu, alloc))
		}
	}

	if len(choices) > 0 {
		return choices
	}

	for function, cpu := range cpuCandidates {
		choices = append(choices, makeChoice(classCPUHotspot, classCPUHotspot, function, cpu, zeroPprofRow()))
	}

	for function, alloc := range allocCandidates {
		choices = append(choices, makeChoice(classAllocHotspot, classAllocHotspot, function, zeroPprofRow(), alloc))
	}

	return choices
}

// enrichChoice adds the source position and the complexity scores that the
// static pass found for the candidate.
func enrichChoice(choice Choice, static map[string]complexity.Stat) Choice {
	stat, ok := static[staticKey(choice.Function)]
	if !ok {
		return choice
	}

	choice.File = stat.File
	choice.Line = stat.Line
	choice.Complexity = Complexity{Cyclomatic: stat.Cyclomatic, Cognitive: stat.Cognitive}

	return choice
}

// withoutProfileHotspot falls back to the static view when the profiles ran but
// produced no candidate above the thresholds.
func withoutProfileHotspot(base Result, static map[string]complexity.Stat) Result {
	result, ok := complexityOnly(base, static, caveatNoClearHotspotStatic)
	if ok {
		return result
	}

	base.Classification = classNoClearHotspot
	base.Reason = classNoClearHotspot

	if base.Caveat == "" {
		base.Caveat = caveatNoClearHotspot
	}

	return base
}

// withoutBenchmark reports the static view when no benchmark workload ran.
func withoutBenchmark(base Result, static map[string]complexity.Stat) Result {
	result, ok := complexityOnly(base, static, caveatNoBenchmarkStatic)
	if ok {
		return result
	}

	base.Classification = classNoBenchmark
	base.Reason = classNoBenchmark
	base.Caveat = appendCaveat(base.Caveat, caveatNoBenchmark)

	return base
}

// complexityOnly suggests the most complex function of the target package. The
// suggestion is a static estimate of where to look, never measured cost, so the
// caller supplies the caveat that says so.
func complexityOnly(base Result, static map[string]complexity.Stat, caveat string) (Result, bool) {
	stat, ok := topComplexity(static, base.ImportPath)
	if !ok {
		return base, false
	}

	base.Classification = classComplexityHotspot
	base.Reason = classComplexityHotspot
	base.Function = stat.Symbol
	base.File = stat.File
	base.Line = stat.Line
	base.Complexity = Complexity{Cyclomatic: stat.Cyclomatic, Cognitive: stat.Cognitive}
	base.Caveat = appendCaveat(base.Caveat, caveat)

	return base, true
}

// topComplexity returns the most complex function declared by the target
// package. Complex code elsewhere in the module enriches a measured candidate
// but is never suggested on its own, because the user asked about one package.
func topComplexity(static map[string]complexity.Stat, importPath string) (complexity.Stat, bool) {
	candidates := make([]complexity.Stat, 0, len(static))

	for _, stat := range static {
		if declaredIn(stat.Symbol, importPath) && complexEnough(stat) {
			candidates = append(candidates, stat)
		}
	}

	if len(candidates) == 0 {
		return complexity.Stat{Symbol: "", File: "", Line: 0, Cyclomatic: 0, Cognitive: 0}, false
	}

	slices.SortFunc(candidates, compareComplexity)

	return candidates[0], true
}

func compareComplexity(left complexity.Stat, right complexity.Stat) int {
	if order := cmp.Compare(complexityScore(right), complexityScore(left)); order != 0 {
		return order
	}

	return strings.Compare(left.Symbol, right.Symbol)
}

// complexityScore normalizes both scores against their thresholds so they can
// be compared without weighting one unit against the other.
func complexityScore(stat complexity.Stat) float64 {
	return max(
		float64(stat.Cyclomatic)/cyclomaticThreshold,
		float64(stat.Cognitive)/cognitiveThreshold,
	)
}

func complexEnough(stat complexity.Stat) bool {
	return float64(stat.Cyclomatic) >= cyclomaticThreshold || float64(stat.Cognitive) >= cognitiveThreshold
}

func declaredIn(symbol string, importPath string) bool {
	return importPath != "" && strings.HasPrefix(symbol, importPath+".")
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
		File:           "",
		Line:           0,
		CPU:            cpuMetric(cpu),
		Alloc:          allocMetric(alloc),
		Complexity:     Complexity{Cyclomatic: 0, Cognitive: 0},
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
