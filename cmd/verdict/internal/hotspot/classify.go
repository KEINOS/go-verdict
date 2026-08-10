package hotspot

// This file fuses every signal into candidates and ranks them with a Pareto
// comparison, so a function that is hot in several ways outranks one that is
// hot in only one.

import (
	"cmp"
	"slices"
	"strings"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/complexity"
	"github.com/KEINOS/go-verdict/internal/pareto"
)

const (
	allocCumThreshold  = 10.0
	allocFlatThreshold = 5.0
	cpuCumThreshold    = 10.0
	cpuFlatThreshold   = 5.0

	// Complexity thresholds mark code worth reading, not code that is slow.
	cognitiveThreshold  = 15.0
	cyclomaticThreshold = 10.0

	// scoreEpsilon keeps float noise from looking like a real difference.
	scoreEpsilon = 1e-9

	// qualifies is the normalized score at which a signal counts.
	qualifies = 1.0

	signalCPU          = "cpu"
	signalAllocBytes   = "alloc-bytes"
	signalAllocObjects = "alloc-objects"
	signalRetained     = "retained"
	signalComplexity   = "complexity"

	classAllocHotspot      = "alloc-hotspot"
	classAllocRateHotspot  = "alloc-rate-hotspot"
	classComplexityHotspot = "complexity-hotspot"
	classCPUHotspot        = "cpu-hotspot"
	classHotAndComplex     = "hot-and-complex"
	classMixedHotspot      = "mixed-hotspot"
	classNoBenchmark       = "no-benchmark"
	classNoClearHotspot    = "no-clear-hotspot"
	classRetentionHotspot  = "retention-hotspot"
)

// Signal indexes. Every candidate carries one normalized score per signal.
const (
	idxCPU = iota
	idxAllocBytes
	idxAllocObjects
	idxRetained
	idxComplexity
	signalCount
)

// candidate is one function with every signal it triggered.
type candidate struct {
	function     string
	file         string
	cpu          Metric
	allocBytes   Metric
	allocObjects Metric
	retained     Metric
	complexity   Complexity
	scores       [signalCount]float64
	line         int
}

// reportConfig describes how a report explains the absence of measured cost.
type reportConfig struct {
	staticCaveat string
	emptyCaveat  string
	emptyClass   string
	top          int
}

/* Constructors and Methods */

// candidate

// measured reports whether any profile signal, as opposed to the static
// estimate, put this candidate above its threshold.
func (item candidate) measured() bool {
	for index := range idxComplexity {
		if item.above(index) {
			return true
		}
	}

	return false
}

// above reports whether one signal put this candidate over its threshold.
func (item candidate) above(index int) bool {
	return item.scores[index] >= qualifies
}

// anyMemory reports whether any of the three memory views qualified.
func (item candidate) anyMemory() bool {
	return item.above(idxAllocBytes) || item.above(idxAllocObjects) || item.above(idxRetained)
}

// signals names every signal that put this candidate above its threshold.
func (item candidate) signals() []string {
	names := make([]string, 0, signalCount)

	for index, name := range signalNames() {
		if item.above(index) {
			names = append(names, name)
		}
	}

	return names
}

// topScore is the strongest single signal, used to break ties inside the
// Pareto front where no candidate dominates another.
func (item candidate) topScore() float64 {
	best := 0.0

	for _, score := range item.scores {
		best = max(best, score)
	}

	return best
}

// classification names the shape of the candidate from its signals. Measured
// cost that is also complex is the headline case, so it gets its own name.
func (item candidate) classification() string {
	if item.measured() && item.above(idxComplexity) {
		return classHotAndComplex
	}

	return item.singleShapeClassification()
}

func (item candidate) singleShapeClassification() string {
	switch {
	case item.above(idxCPU) && item.anyMemory():
		return classMixedHotspot
	case item.above(idxCPU):
		return classCPUHotspot
	case item.above(idxAllocBytes):
		return classAllocHotspot
	case item.above(idxAllocObjects):
		return classAllocRateHotspot
	case item.above(idxRetained):
		return classRetentionHotspot
	default:
		return classComplexityHotspot
	}
}

func (item candidate) choice() Choice {
	return Choice{
		Classification: item.classification(),
		Function:       item.function,
		File:           item.file,
		Line:           item.line,
		Signals:        item.signals(),
		CPU:            item.cpu,
		AllocBytes:     item.allocBytes,
		AllocObjects:   item.allocObjects,
		Retained:       item.retained,
		Complexity:     item.complexity,
	}
}

/* Helper Functions */

func signalNames() [signalCount]string {
	return [signalCount]string{signalCPU, signalAllocBytes, signalAllocObjects, signalRetained, signalComplexity}
}

// classify ranks the profile and static signals into one report.
func classify(base Result, profiles profileSet, static map[string]complexity.Stat, top int) Result {
	return report(base, profiles, static, reportConfig{
		staticCaveat: caveatNoClearHotspotStatic,
		emptyCaveat:  caveatNoClearHotspot,
		emptyClass:   classNoClearHotspot,
		top:          top,
	})
}

// withoutBenchmark reports the static view when no benchmark workload ran.
func withoutBenchmark(base Result, static map[string]complexity.Stat, top int) Result {
	return report(base, emptyProfiles(), static, reportConfig{
		staticCaveat: caveatNoBenchmarkStatic,
		emptyCaveat:  caveatNoBenchmark,
		emptyClass:   classNoBenchmark,
		top:          top,
	})
}

// report builds candidates, ranks them, and fills the result.
func report(base Result, profiles profileSet, static map[string]complexity.Stat, cfg reportConfig) Result {
	ranked := rankCandidates(buildCandidates(profiles, static, base.ImportPath))
	if len(ranked) == 0 {
		base.Classification = cfg.emptyClass
		base.Caveat = appendCaveat(base.Caveat, cfg.emptyCaveat)

		return base
	}

	primary := ranked[0]
	base.Classification = primary.classification()
	base.Function = primary.function
	base.File = primary.file
	base.Line = primary.line
	base.Signals = primary.signals()
	base.CPU = primary.cpu
	base.AllocBytes = primary.allocBytes
	base.AllocObjects = primary.allocObjects
	base.Retained = primary.retained
	base.Complexity = primary.complexity
	base.Candidates = runnersUp(ranked, cfg.top)

	if !primary.measured() {
		base.Caveat = appendCaveat(base.Caveat, cfg.staticCaveat)
	}

	return base
}

// runnersUp returns the candidates after the suggestion, capped so that the
// report never names more than top candidates in total.
func runnersUp(ranked []candidate, top int) []Choice {
	limit := min(top, len(ranked))
	choices := make([]Choice, 0, max(limit-1, 0))

	for _, item := range ranked[1:limit] {
		choices = append(choices, item.choice())
	}

	return choices
}

// buildCandidates unions every function that any signal put above its
// threshold. Complex code anywhere in the module enriches a measured
// candidate, but only the target package can qualify on complexity alone.
func buildCandidates(profiles profileSet, static map[string]complexity.Stat, importPath string) []candidate {
	items := make(map[string]*candidate)

	addRows(items, profiles.CPU, idxCPU)
	addRows(items, profiles.Alloc, idxAllocBytes)
	addRows(items, profiles.AllocObjects, idxAllocObjects)
	addRows(items, profiles.Inuse, idxRetained)
	addStatic(items, static, importPath)

	qualified := make([]candidate, 0, len(items))

	for _, item := range items {
		if slices.ContainsFunc(item.scores[:], func(score float64) bool { return score >= qualifies }) {
			qualified = append(qualified, *item)
		}
	}

	return qualified
}

func addRows(items map[string]*candidate, rows map[string]pprofRow, index int) {
	for function, row := range rows {
		item := itemFor(items, function)
		item.scores[index] = profileScore(row, index)

		switch index {
		case idxCPU:
			item.cpu = metricOf(row, unitMS)
		case idxAllocBytes:
			item.allocBytes = metricOf(row, unitBytes)
		case idxAllocObjects:
			item.allocObjects = metricOf(row, unitObjects)
		case idxRetained:
			item.retained = metricOf(row, unitBytes)
		}
	}
}

// addStatic attaches the source position and complexity of every function the
// static pass scored, and admits target-package functions as candidates.
func addStatic(items map[string]*candidate, static map[string]complexity.Stat, importPath string) {
	for key, item := range items {
		stat, ok := static[staticKey(key)]
		if !ok {
			continue
		}

		applyStatic(item, stat, importPath)
	}

	for _, stat := range static {
		if !declaredIn(stat.Symbol, importPath) || complexityScore(stat) < qualifies {
			continue
		}

		if _, ok := items[stat.Symbol]; ok {
			continue
		}

		applyStatic(itemFor(items, stat.Symbol), stat, importPath)
	}
}

func applyStatic(item *candidate, stat complexity.Stat, importPath string) {
	item.file = stat.File
	item.line = stat.Line
	item.complexity = Complexity{Cyclomatic: stat.Cyclomatic, Cognitive: stat.Cognitive}

	if declaredIn(stat.Symbol, importPath) {
		item.scores[idxComplexity] = complexityScore(stat)
	}
}

func itemFor(items map[string]*candidate, function string) *candidate {
	item, ok := items[function]
	if !ok {
		item = &candidate{
			function:     function,
			file:         "",
			line:         0,
			cpu:          zeroMetric(unitMS),
			allocBytes:   zeroMetric(unitBytes),
			allocObjects: zeroMetric(unitObjects),
			retained:     zeroMetric(unitBytes),
			complexity:   Complexity{Cyclomatic: 0, Cognitive: 0},
			scores:       [signalCount]float64{},
		}
		items[function] = item
	}

	return item
}

// rankCandidates puts the Pareto front first. A candidate is in the front when
// no other candidate is at least as strong on every signal and stronger on one.
func rankCandidates(candidates []candidate) []candidate {
	front := make([]candidate, 0, len(candidates))
	rest := make([]candidate, 0, len(candidates))

	for _, item := range candidates {
		if dominated(item, candidates) {
			rest = append(rest, item)

			continue
		}

		front = append(front, item)
	}

	slices.SortFunc(front, compareCandidates)
	slices.SortFunc(rest, compareCandidates)

	return append(front, rest...)
}

func dominated(item candidate, candidates []candidate) bool {
	for _, other := range candidates {
		if other.function == item.function {
			continue
		}

		if pareto.Compare(signalRelations(other, item)...) == pareto.CandidateWins {
			return true
		}
	}

	return false
}

// signalRelations maps every signal to a Pareto metric. A higher score always
// means more worth inspecting, so the units never have to be weighed together.
func signalRelations(left candidate, right candidate) []pareto.Metric {
	relations := make([]pareto.Metric, 0, signalCount)

	for index := range signalCount {
		relations = append(relations, scoreRelation(left.scores[index], right.scores[index]))
	}

	return relations
}

func scoreRelation(left float64, right float64) pareto.Metric {
	switch {
	case left > right+scoreEpsilon:
		return pareto.Better
	case left < right-scoreEpsilon:
		return pareto.Worse
	default:
		return pareto.Same
	}
}

// compareCandidates orders candidates that no Pareto rule separates. Measured
// cost outranks a static estimate, then breadth of evidence, then strength.
func compareCandidates(left candidate, right candidate) int {
	if order := cmp.Compare(measuredRank(right), measuredRank(left)); order != 0 {
		return order
	}

	if order := cmp.Compare(len(right.signals()), len(left.signals())); order != 0 {
		return order
	}

	if order := cmp.Compare(right.topScore(), left.topScore()); order != 0 {
		return order
	}

	return strings.Compare(left.function, right.function)
}

func measuredRank(item candidate) int {
	if item.measured() {
		return 1
	}

	return 0
}

// profileScore normalizes one profile row against its thresholds, so a score of
// one means the row is exactly at the threshold.
func profileScore(row pprofRow, index int) float64 {
	flatThreshold, cumThreshold := cpuFlatThreshold, cpuCumThreshold
	if index != idxCPU {
		flatThreshold, cumThreshold = allocFlatThreshold, allocCumThreshold
	}

	return max(row.FlatPct/flatThreshold, row.CumPct/cumThreshold)
}

// complexityScore normalizes both complexity scores against their thresholds so
// they can be compared without weighting one unit against the other.
func complexityScore(stat complexity.Stat) float64 {
	return max(
		float64(stat.Cyclomatic)/cyclomaticThreshold,
		float64(stat.Cognitive)/cognitiveThreshold,
	)
}

func declaredIn(symbol string, importPath string) bool {
	return importPath != "" && strings.HasPrefix(symbol, importPath+".")
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
