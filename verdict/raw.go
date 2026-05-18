package verdict

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// alternativeSampleSet stores raw alternatives as:
// parent benchmark -> sub-benchmark label -> normalized metric -> samples.
type alternativeSampleSet map[string]map[string]map[string][]float64

type alternativeParseState struct {
	samples             alternativeSampleSet
	hasBenchmarkRows    bool
	hasMalformedRows    bool
	hasUnsupportedRows  bool
	hasInsufficientRows bool
}

type rawFileParseState struct {
	name               string
	metrics            map[string][]float64
	hasBenchmarkRows   bool
	hasMalformedRows   bool
	hasUnsupportedRows bool
	hasMultipleSeries  bool
}

type rawBenchmarkSample struct {
	parent  string
	label   string
	metrics map[string]float64
}

const (
	metricNanosecondsPerOp  = "ns/op"
	metricBytesPerOp        = "B/op"
	metricAllocsPerOp       = "allocs/op"
	requiredAlternativePair = 2
	rawBenchmarkMinFields   = 4
	rawBenchmarkNameParts   = 2
	rawBenchmarkValueUnit   = 2
	percentScale            = 100
)

var (
	errScanningRawInput = errors.New("scanning raw alternatives input")
	errScanningRawFile  = errors.New("scanning raw benchmark file input")
)

func parseAlternatives(input string, opts Options) (Report, error) {
	state := newAlternativeParseState()

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		state.handleLine(scanner.Text())
	}

	err := scanner.Err()
	if err != nil {
		return Report{}, fmt.Errorf("%w: %w", errScanningRawInput, err)
	}

	if !state.hasBenchmarkRows {
		return inconclusiveReport("malformed-benchmark"), nil
	}

	// Parsing only records raw samples and input-shape flags. Evaluation below
	// selects labels, checks sample counts, and applies shared verdict rules.
	report := state.evaluate(opts)
	if len(report.Verdicts) == 0 {
		return state.emptyAlternativeReport(), nil
	}

	return report, nil
}

func compareRawFiles(aReader io.Reader, bReader io.Reader, opts Options) (Report, error) {
	aState, err := parseRawFile(aReader)
	if err != nil {
		return Report{}, err
	}

	bState, err := parseRawFile(bReader)
	if err != nil {
		return Report{}, err
	}

	if inconclusive := rawFileInconclusive(aState, bState); inconclusive != nil {
		return Report{Verdicts: []BenchmarkVerdict{*inconclusive}}, nil
	}

	// Raw-file comparison treats two separate benchmark series as explicit A/B
	// alternatives, unlike raw stdin mode where labels come from sub-benchmarks.
	benchmark := aState.name + "_vs_" + bState.name

	rows, ok := compareAlternativeMetrics(
		benchmark,
		aState.name,
		bState.name,
		aState.metrics,
		bState.metrics,
		opts,
	)
	if !ok {
		return Report{Verdicts: []BenchmarkVerdict{*alternativeInconclusive(benchmark, "insufficient-samples")}}, nil
	}

	if len(rows) == 0 {
		return Report{Verdicts: []BenchmarkVerdict{*alternativeInconclusive(benchmark, "unsupported-metric")}}, nil
	}

	return evaluate(rows), nil
}

func parseRawFile(reader io.Reader) (rawFileParseState, error) {
	input, err := io.ReadAll(reader)
	if err != nil {
		return rawFileParseState{}, fmt.Errorf("%w: %w", errReadingInput, err)
	}

	var state rawFileParseState

	state.metrics = map[string][]float64{}

	scanner := bufio.NewScanner(strings.NewReader(string(input)))

	for scanner.Scan() {
		state.handleLine(scanner.Text())
	}

	err = scanner.Err()
	if err != nil {
		return rawFileParseState{}, fmt.Errorf("%w: %w", errScanningRawFile, err)
	}

	return state, nil
}

func (state *rawFileParseState) handleLine(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "Benchmark") {
		return
	}

	state.hasBenchmarkRows = true

	name, metrics, ok := parseRawFileBenchmarkLine(line)
	if !ok {
		state.hasMalformedRows = true

		return
	}

	if len(metrics) == 0 {
		state.hasUnsupportedRows = true

		return
	}

	if state.name != "" && state.name != name {
		state.hasMultipleSeries = true

		return
	}

	state.name = name
	for metric, value := range metrics {
		state.metrics[metric] = append(state.metrics[metric], value)
	}
}

func parseRawFileBenchmarkLine(line string) (string, map[string]float64, bool) {
	fields := strings.Fields(line)
	if len(fields) < rawBenchmarkMinFields {
		return "", nil, false
	}

	_, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", nil, false
	}

	if len(fields[2:])%rawBenchmarkValueUnit != 0 {
		return "", nil, false
	}

	name := trimCPUSuffix(fields[0])
	if name == "" {
		return "", nil, false
	}

	metrics, ok := parseRawMetrics(fields[2:])
	if !ok {
		return "", nil, false
	}

	return name, metrics, true
}

func rawFileInconclusive(aState, bState rawFileParseState) *BenchmarkVerdict {
	switch {
	case !aState.hasBenchmarkRows || !bState.hasBenchmarkRows:
		return alternativeInconclusive("all", "malformed-benchmark")
	case aState.hasMalformedRows || bState.hasMalformedRows:
		return alternativeInconclusive("all", "malformed-benchmark")
	case aState.hasMultipleSeries || bState.hasMultipleSeries:
		return alternativeInconclusive("all", "ambiguous-benchmark")
	case aState.hasUnsupportedRows || bState.hasUnsupportedRows:
		return alternativeInconclusive("all", "unsupported-metric")
	default:
		return nil
	}
}

func newAlternativeParseState() alternativeParseState {
	var zeroState alternativeParseState

	zeroState.samples = make(alternativeSampleSet)

	return zeroState
}

func (state *alternativeParseState) handleLine(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "Benchmark") {
		return
	}

	state.hasBenchmarkRows = true

	sample, ok := parseRawBenchmarkLine(line)
	if !ok {
		state.hasMalformedRows = true

		return
	}

	if len(sample.metrics) == 0 {
		state.hasUnsupportedRows = true

		return
	}

	state.addSample(sample)
}

func (state *alternativeParseState) addSample(sample rawBenchmarkSample) {
	if _, ok := state.samples[sample.parent]; !ok {
		state.samples[sample.parent] = map[string]map[string][]float64{}
	}

	if _, ok := state.samples[sample.parent][sample.label]; !ok {
		state.samples[sample.parent][sample.label] = map[string][]float64{}
	}

	for metric, value := range sample.metrics {
		state.samples[sample.parent][sample.label][metric] = append(
			state.samples[sample.parent][sample.label][metric],
			value,
		)
	}
}

func parseRawBenchmarkLine(line string) (rawBenchmarkSample, bool) {
	var zeroSample rawBenchmarkSample

	fields := strings.Fields(line)
	if len(fields) < rawBenchmarkMinFields {
		return zeroSample, false
	}

	_, err := strconv.Atoi(fields[1])
	if err != nil {
		return zeroSample, false
	}

	parent, label, ok := splitRawBenchmarkName(fields[0])
	if !ok || len(fields[2:])%rawBenchmarkValueUnit != 0 {
		return zeroSample, false
	}

	metrics, ok := parseRawMetrics(fields[2:])
	if !ok {
		return zeroSample, false
	}

	return rawBenchmarkSample{
		parent:  parent,
		label:   label,
		metrics: metrics,
	}, true
}

func splitRawBenchmarkName(name string) (string, string, bool) {
	name = trimCPUSuffix(name)

	parts := strings.Split(name, "/")
	if len(parts) < rawBenchmarkNameParts {
		return "", "", false
	}

	parent := strings.Join(parts[:len(parts)-1], "/")
	label := parts[len(parts)-1]

	return parent, label, parent != "" && label != ""
}

func trimCPUSuffix(name string) string {
	index := strings.LastIndex(name, "-")
	if index < 0 || index == len(name)-1 {
		return name
	}

	_, err := strconv.Atoi(name[index+1:])
	if err != nil {
		return name
	}

	return name[:index]
}

func parseRawMetrics(fields []string) (map[string]float64, bool) {
	metrics := map[string]float64{}

	for index := 0; index+1 < len(fields); index += rawBenchmarkValueUnit {
		metric, ok := normalizeRawMetric(fields[index+1])
		if !ok {
			continue
		}

		value, err := strconv.ParseFloat(fields[index], 64)
		if err != nil {
			return nil, false
		}

		metrics[metric] = value
	}

	return metrics, true
}

func normalizeRawMetric(metric string) (string, bool) {
	switch metric {
	case metricNanosecondsPerOp:
		return metricSecPerOp, true
	case metricBytesPerOp:
		return metricBytesPerOp, true
	case metricAllocsPerOp:
		return metricAllocsPerOp, true
	default:
		return "", false
	}
}

func (state *alternativeParseState) evaluate(opts Options) Report {
	parents := sortedAlternativeParents(state.samples)
	verdicts := make([]BenchmarkVerdict, 0, len(parents))
	rows := make([]Comparison, 0)

	for _, parent := range parents {
		parentRows, inconclusive := state.evaluateParent(parent, opts)
		if inconclusive != nil {
			verdicts = append(verdicts, *inconclusive)

			continue
		}

		rows = append(rows, parentRows...)
	}

	if len(rows) > 0 {
		verdicts = append(verdicts, evaluate(rows).Verdicts...)
	}

	sort.Slice(verdicts, func(i, j int) bool {
		return verdicts[i].Benchmark < verdicts[j].Benchmark
	})

	return Report{Verdicts: verdicts}
}

func sortedAlternativeParents(samples alternativeSampleSet) []string {
	parents := make([]string, 0, len(samples))
	for parent := range samples {
		parents = append(parents, parent)
	}

	sort.Strings(parents)

	return parents
}

func (state *alternativeParseState) evaluateParent(parent string, opts Options) ([]Comparison, *BenchmarkVerdict) {
	labels := state.samples[parent]

	baselineLabel, candidateLabel, ok := selectAlternativeLabels(labels, opts)
	if !ok {
		return nil, alternativeInconclusive(parent, "ambiguous-alternatives")
	}

	baselineMetrics, hasBaseline := labels[baselineLabel]
	candidateMetrics, hasCandidate := labels[candidateLabel]

	switch {
	case !hasBaseline:
		return nil, alternativeInconclusive(parent, "missing-baseline")
	case !hasCandidate:
		return nil, alternativeInconclusive(parent, "missing-candidate")
	}

	rows, ok := compareAlternativeMetrics(parent, baselineLabel, candidateLabel, baselineMetrics, candidateMetrics, opts)
	if !ok {
		state.hasInsufficientRows = true

		return nil, alternativeInconclusive(parent, "insufficient-samples")
	}

	if len(rows) == 0 {
		return nil, alternativeInconclusive(parent, "unsupported-metric")
	}

	return rows, nil
}

func selectAlternativeLabels(labels map[string]map[string][]float64, opts Options) (string, string, bool) {
	if opts.Baseline != "" || opts.Candidate != "" {
		return opts.Baseline, opts.Candidate, true
	}

	if _, hasBaseline := labels[defaultBaseline]; hasBaseline {
		if _, hasCandidate := labels[defaultCandidate]; hasCandidate {
			return defaultBaseline, defaultCandidate, true
		}
	}

	names := make([]string, 0, len(labels))
	for label := range labels {
		names = append(names, label)
	}

	sort.Strings(names)

	if len(names) != requiredAlternativePair {
		return "", "", false
	}

	return names[0], names[1], true
}

func compareAlternativeMetrics(
	parent string,
	baselineLabel string,
	candidateLabel string,
	baselineMetrics map[string][]float64,
	candidateMetrics map[string][]float64,
	opts Options,
) ([]Comparison, bool) {
	metrics := commonMetrics(baselineMetrics, candidateMetrics)
	rows := make([]Comparison, 0, len(metrics))

	for _, metric := range metrics {
		baseline := baselineMetrics[metric]
		candidate := candidateMetrics[metric]

		if len(baseline) < RawComparisonMinSamples || len(candidate) < RawComparisonMinSamples {
			return nil, false
		}

		rows = append(rows, compareAlternativeMetric(
			parent,
			metric,
			baselineLabel,
			candidateLabel,
			baseline,
			candidate,
			opts,
		))
	}

	return rows, true
}

func commonMetrics(left, right map[string][]float64) []string {
	metrics := make([]string, 0, len(left))
	for metric := range left {
		if _, ok := right[metric]; ok {
			metrics = append(metrics, metric)
		}
	}

	sort.Strings(metrics)

	return metrics
}

func compareAlternativeMetric(
	parent string,
	metric string,
	baselineLabel string,
	candidateLabel string,
	baseline []float64,
	candidate []float64,
	opts Options,
) Comparison {
	baselineMean := mean(baseline)
	candidateMean := mean(candidate)
	delta := deltaPercent(baselineMean, candidateMean)
	pValue := pValueApproximation(baseline, candidate)
	significant := pValue <= opts.Alpha && math.Abs(delta) >= opts.MinDeltaPct

	return Comparison{
		Benchmark:      parent,
		Metric:         metric,
		DeltaPct:       delta,
		PValue:         pValue,
		Significant:    significant,
		Direction:      classify(metric, delta, significant),
		BaselineLabel:  displayLabel(baselineLabel),
		CandidateLabel: displayLabel(candidateLabel),
	}
}

func mean(values []float64) float64 {
	// Callers enforce non-empty sample sets before statistical comparison.
	total := 0.0
	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

func variance(values []float64, sampleMean float64) float64 {
	if len(values) < StatisticalMinSamples {
		return 0
	}

	sum := 0.0

	for _, value := range values {
		diff := value - sampleMean
		sum += diff * diff
	}

	return sum / float64(len(values)-1)
}

func deltaPercent(baselineMean, candidateMean float64) float64 {
	if baselineMean == 0 {
		return 0
	}

	return ((candidateMean - baselineMean) / baselineMean) * percentScale
}

// pValueApproximation uses a pragmatic normal approximation from repeated
// samples instead of a heavier statistics dependency. It is most useful with
// the recommended raw sample count or more.
func pValueApproximation(baseline, candidate []float64) float64 {
	baselineMean := mean(baseline)
	candidateMean := mean(candidate)
	baselineVariance := variance(baseline, baselineMean)
	candidateVariance := variance(candidate, candidateMean)
	standardError := math.Sqrt(
		baselineVariance/float64(len(baseline)) +
			candidateVariance/float64(len(candidate)),
	)

	if standardError == 0 {
		// Exact zero represents zero variance in both sample sets.
		if baselineMean == candidateMean {
			return 1
		}

		return 0
	}

	zScore := math.Abs((candidateMean - baselineMean) / standardError)

	return math.Erfc(zScore / math.Sqrt2)
}

func alternativeInconclusive(parent, reason string) *BenchmarkVerdict {
	return &BenchmarkVerdict{
		Benchmark:      parent,
		Outcome:        Inconclusive,
		Winner:         "",
		BaselineLabel:  "",
		CandidateLabel: "",
		Metrics:        nil,
		Reason:         "inconclusive alternative input",
		ReasonCode:     reason,
	}
}

func (state *alternativeParseState) emptyAlternativeReport() Report {
	switch {
	case state.hasMalformedRows:
		return inconclusiveReport("malformed-benchmark")
	case state.hasUnsupportedRows:
		return inconclusiveReport("unsupported-metric")
	case state.hasInsufficientRows:
		return inconclusiveReport("insufficient-samples")
	default:
		return inconclusiveReport("malformed-benchmark")
	}
}

func looksLikeRawBenchmarkInput(input string) bool {
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Benchmark") {
			continue
		}

		if _, ok := parseRawBenchmarkLine(line); ok {
			return true
		}
	}

	return false
}
