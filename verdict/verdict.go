// Package verdict parses benchstat comparison output and turns it into simple benchmark verdicts.
package verdict

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Direction describes whether one metric improved, worsened, or stayed effectively the same.
type Direction string

const (
	// Improved means the new benchmark value is better than the old value.
	Improved Direction = "improved"
	// Worsened means the new benchmark value is worse than the old value.
	Worsened Direction = "worsened"
	// Same means the metric has no significant practical difference.
	Same Direction = "same"
)

// Outcome is the benchmark-level decision after all comparable metrics are evaluated.
type Outcome string

const (
	// NewWins means the new result has improvements and no regressions.
	NewWins Outcome = "new-wins"
	// OldWins means the new result has regressions and no improvements.
	OldWins Outcome = "old-wins"
	// Tie means no metric changed enough to matter.
	Tie Outcome = "tie"
	// TradeOff means at least one metric improved and at least one metric regressed.
	TradeOff Outcome = "trade-off"
	// Inconclusive means the input does not contain enough comparable data.
	Inconclusive Outcome = "inconclusive"
)

// Options controls the statistical and practical thresholds used by Parse.
// Alpha defaults to 0.05 when left zero; non-zero Alpha values must be finite
// and greater than 0 and at most 1. MinDeltaPct must be finite and non-negative.
type Options struct {
	Alpha       float64
	MinDeltaPct float64
	Mode        string
	Baseline    string
	Candidate   string
}

// Comparison is one parsed metric comparison for one benchmark.
type Comparison struct {
	Benchmark      string    `json:"benchmark"`
	Metric         string    `json:"metric"`
	DeltaPct       float64   `json:"delta_pct"`
	PValue         float64   `json:"p_value"`
	Significant    bool      `json:"significant"`
	Direction      Direction `json:"direction"`
	BaselineLabel  string    `json:"-"`
	CandidateLabel string    `json:"-"`
}

// BenchmarkVerdict is the final outcome for one benchmark name.
type BenchmarkVerdict struct {
	Benchmark      string       `json:"benchmark"`
	Outcome        Outcome      `json:"outcome"`
	Winner         string       `json:"winner,omitempty"`
	BaselineLabel  string       `json:"baseline_label,omitempty"`
	CandidateLabel string       `json:"candidate_label,omitempty"`
	Metrics        []Comparison `json:"metrics"`
	Reason         string       `json:"reason"`
	ReasonCode     string       `json:"reason_code,omitempty"`
}

// Report is the complete parse and evaluation result.
type Report struct {
	Verdicts []BenchmarkVerdict `json:"verdicts"`
}

type textParseState struct {
	rows                      []Comparison
	currentMetric             string
	baselineLabel             string
	candidateLabel            string
	hasBenchmarkSetMismatch   bool
	hasComparisonRowsWithoutP bool
}

type csvParseState struct {
	rows                    []Comparison
	metric                  string
	baselineLabel           string
	candidateLabel          string
	deltaIndex              int
	pValueIndex             int
	baseIndex               int
	newIndex                int
	hasRowWithoutP          bool
	hasBenchmarkSetMismatch bool
}

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

var (
	deltaRe      = regexp.MustCompile(`([+-]\d+(?:\.\d+)?)%|\s~\s`)
	pValueRe     = regexp.MustCompile(`\bp=([0-9]*\.?[0-9]+|n/a)\b`)
	metricRe     = regexp.MustCompile(metricPattern)
	oldHeaderRe  = regexp.MustCompile(`(?i)^name\s+old\s+(\S+)\s+new\s+\S+\s+delta`)
	benchNameCut = regexp.MustCompile(`\s+`)
)

const (
	// StatisticalMinSamples is the minimum needed for variance-based calculations.
	StatisticalMinSamples = 2
	// RawComparisonMinSamples is the minimum accepted sample count for raw benchmark comparisons.
	RawComparisonMinSamples = 3
	// RecommendedRawSamples is the recommended sample count for stable raw benchmark decisions.
	RecommendedRawSamples = 10
)

const (
	defaultAlpha            = 0.05
	defaultBaseline         = "original"
	defaultCandidate        = "enhanced"
	fallbackBaselineLabel   = "old"
	fallbackCandidateLabel  = "new"
	modeAuto                = "auto"
	modeAlternatives        = "alternatives"
	modeBenchstat           = "benchstat"
	metricPattern           = `(?i)\b(?:sec/op|ns/op|time/op|b/op|bytes/op|allocs/op|[a-zµμ]+/op|[a-zµμ]+/s)\b`
	metricNanosecondsPerOp  = "ns/op"
	metricSecPerOp          = "sec/op"
	metricBytesPerOp        = "B/op"
	metricAllocsPerOp       = "allocs/op"
	minimumComparisonFields = 2
	requiredAlternativePair = 2
	benchmarkSplitCount     = 2
	rawBenchmarkMinFields   = 4
	rawBenchmarkNameParts   = 2
	rawBenchmarkValueUnit   = 2
	percentScale            = 100
)

var (
	errReadingInput      = errors.New("reading benchstat input")
	errReadingCSVInput   = errors.New("reading benchstat csv input")
	errScanningTextInput = errors.New("scanning benchstat text input")
	errScanningRawInput  = errors.New("scanning raw alternatives input")
	errScanningRawFile   = errors.New("scanning raw benchmark file input")
	errInvalidOptions    = errors.New("invalid options")
	errNoComparisonRows  = errors.New("no benchstat comparison rows found")
	errWritingTextOutput = errors.New("writing text report")
	errWritingJSONOutput = errors.New("writing json report")
)

// Parse reads benchstat output and returns a benchmark verdict report.
func Parse(reader io.Reader, opts Options) (Report, error) {
	opts = normalizeOptions(opts)
	if err := validateOptions(opts); err != nil {
		return Report{}, err
	}

	input, err := io.ReadAll(reader)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %w", errReadingInput, err)
	}

	text := string(input)

	switch opts.Mode {
	case modeAlternatives:
		return parseAlternatives(text, opts)
	case modeBenchstat:
		return parseBenchstat(text, opts)
	default:
		if looksLikeRawBenchmarkInput(text) {
			return parseAlternatives(text, opts)
		}

		return parseBenchstat(text, opts)
	}
}

func parseBenchstat(text string, opts Options) (Report, error) {
	if strings.Contains(text, ",vs base,P") {
		return parseCSV(text, opts)
	}

	return parseText(text, opts)
}

func normalizeOptions(opts Options) Options {
	if opts.Alpha == 0 {
		opts.Alpha = defaultAlpha
	}

	if opts.Mode == "" {
		opts.Mode = modeAuto
	}

	if opts.Mode == modeAlternatives && opts.Baseline == "" {
		opts.Baseline = defaultBaseline
	}

	if opts.Mode == modeAlternatives && opts.Candidate == "" {
		opts.Candidate = defaultCandidate
	}

	return opts
}

func validateOptions(opts Options) error {
	switch {
	case math.IsNaN(opts.Alpha) || math.IsInf(opts.Alpha, 0) || opts.Alpha <= 0 || opts.Alpha > 1:
		return fmt.Errorf("%w: alpha must be greater than 0 and at most 1", errInvalidOptions)
	case math.IsNaN(opts.MinDeltaPct) || math.IsInf(opts.MinDeltaPct, 0) || opts.MinDeltaPct < 0:
		return fmt.Errorf("%w: min-delta must be finite and non-negative", errInvalidOptions)
	default:
		return nil
	}
}

func parseText(input string, opts Options) (Report, error) {
	state := textParseState{}

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		state.handleLine(scanner.Text(), opts)
	}

	err := scanner.Err()
	if err != nil {
		return Report{}, fmt.Errorf("%w: %w", errScanningTextInput, err)
	}

	if len(state.rows) == 0 {
		if state.hasBenchmarkSetMismatch {
			return state.inconclusiveReport("benchmark-set-mismatch"), nil
		}

		if state.hasComparisonRowsWithoutP {
			return state.inconclusiveReport("missing-pvalue"), nil
		}

		return Report{}, errNoComparisonRows
	}

	return evaluate(state.rows), nil
}

func (state *textParseState) handleLine(rawLine string, opts Options) {
	line := strings.TrimSpace(rawLine)
	if shouldSkip(line) {
		return
	}

	if strings.Contains(line, "benchmark set differs") {
		state.hasBenchmarkSetMismatch = true
	}

	state.captureLabels(line)

	if match := oldHeaderRe.FindStringSubmatch(line); match != nil {
		state.currentMetric = normalizeMetric(match[1])

		return
	}

	if !strings.Contains(line, "p=") {
		state.handleLineWithoutPValue(line)

		return
	}

	row, ok := parseComparisonLine(line, state.currentMetric, opts)
	if ok {
		row.BaselineLabel = state.displayBaselineLabel()
		row.CandidateLabel = state.displayCandidateLabel()
		state.rows = append(state.rows, row)
	}
}

func (state *textParseState) captureLabels(line string) {
	if state.baselineLabel != "" || !strings.Contains(line, "│") {
		return
	}

	labels, ok := parseBenchstatTextLabels(line)
	if !ok {
		return
	}

	state.baselineLabel = labels[0]
	state.candidateLabel = labels[1]
}

func parseBenchstatTextLabels(line string) ([2]string, bool) {
	parts := strings.Split(line, "│")
	cells := make([]string, 0, len(parts))

	for _, part := range parts {
		cell := strings.TrimSpace(part)
		if cell != "" {
			cells = append(cells, cell)
		}
	}

	if len(cells) < 2 || metricRe.MatchString(cells[0]) || metricRe.MatchString(cells[1]) {
		return [2]string{}, false
	}

	return [2]string{cells[0], cells[1]}, true
}

func (state *textParseState) displayBaselineLabel() string {
	if state.baselineLabel != "" {
		return displayLabel(state.baselineLabel)
	}

	return fallbackBaselineLabel
}

func (state *textParseState) displayCandidateLabel() string {
	if state.candidateLabel != "" {
		return displayLabel(state.candidateLabel)
	}

	return fallbackCandidateLabel
}

func (state *textParseState) inconclusiveReport(reason string) Report {
	return labeledInconclusiveReport(
		reason,
		state.rawBaselineLabel(),
		state.rawCandidateLabel(),
	)
}

func (state *textParseState) rawBaselineLabel() string {
	if state.baselineLabel != "" {
		return state.baselineLabel
	}

	return fallbackBaselineLabel
}

func (state *textParseState) rawCandidateLabel() string {
	if state.candidateLabel != "" {
		return state.candidateLabel
	}

	return fallbackCandidateLabel
}

func (state *textParseState) handleLineWithoutPValue(line string) {
	if matches := metricRe.FindAllString(line, -1); len(matches) > 0 {
		state.currentMetric = normalizeMetric(matches[0])
	}

	if state.currentMetric != "" && looksLikeComparisonLine(line) {
		state.hasComparisonRowsWithoutP = true
	}
}

func parseCSV(input string, opts Options) (Report, error) {
	reader := csv.NewReader(strings.NewReader(input))
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return Report{}, fmt.Errorf("%w: %w", errReadingCSVInput, err)
	}

	state := newCSVParseState()

	for _, record := range records {
		state.handleRecord(record, opts)
	}

	if len(state.rows) == 0 {
		if state.hasBenchmarkSetMismatch {
			return labeledInconclusiveReport(
				"benchmark-set-mismatch",
				state.rawBaselineLabel(),
				state.rawCandidateLabel(),
			), nil
		}

		if state.hasRowWithoutP {
			return inconclusiveReport("missing-pvalue"), nil
		}

		return Report{}, errNoComparisonRows
	}

	return evaluate(state.rows), nil
}

func parseAlternatives(input string, opts Options) (Report, error) {
	state := newAlternativeParseState()

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		state.handleLine(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return Report{}, fmt.Errorf("%w: %w", errScanningRawInput, err)
	}

	if !state.hasBenchmarkRows {
		return inconclusiveReport("malformed-benchmark"), nil
	}

	report := state.evaluate(opts)
	if len(report.Verdicts) == 0 {
		return state.emptyAlternativeReport(), nil
	}

	return report, nil
}

// CompareRawFiles compares two raw go test benchmark result files as explicit A/B inputs.
func CompareRawFiles(aReader io.Reader, bReader io.Reader, opts Options) (Report, error) {
	opts = normalizeOptions(opts)
	if err := validateOptions(opts); err != nil {
		return Report{}, err
	}

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

	state := rawFileParseState{metrics: map[string][]float64{}}
	scanner := bufio.NewScanner(strings.NewReader(string(input)))

	for scanner.Scan() {
		state.handleLine(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
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

	if _, err := strconv.Atoi(fields[1]); err != nil {
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
	return alternativeParseState{
		samples: alternativeSampleSet{},
	}
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

type rawBenchmarkSample struct {
	parent  string
	label   string
	metrics map[string]float64
}

func parseRawBenchmarkLine(line string) (rawBenchmarkSample, bool) {
	fields := strings.Fields(line)
	if len(fields) < rawBenchmarkMinFields {
		return rawBenchmarkSample{}, false
	}

	if _, err := strconv.Atoi(fields[1]); err != nil {
		return rawBenchmarkSample{}, false
	}

	parent, label, ok := splitRawBenchmarkName(fields[0])
	if !ok || len(fields[2:])%rawBenchmarkValueUnit != 0 {
		return rawBenchmarkSample{}, false
	}

	metrics, ok := parseRawMetrics(fields[2:])
	if !ok {
		return rawBenchmarkSample{}, false
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

	if _, err := strconv.Atoi(name[index+1:]); err != nil {
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
// samples. It is most useful with the recommended raw sample count or more.
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
		Benchmark:  parent,
		Outcome:    Inconclusive,
		Metrics:    nil,
		Reason:     "inconclusive alternative input",
		ReasonCode: reason,
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

func newCSVParseState() csvParseState {
	return csvParseState{
		deltaIndex:  -1,
		pValueIndex: -1,
		baseIndex:   -1,
		newIndex:    -1,
	}
}

func (state *csvParseState) handleRecord(record []string, opts Options) {
	if len(record) == 0 {
		return
	}

	fields := trimRecord(record)
	state.captureLabels(fields)

	if isCSVMetricHeader(fields) {
		state.setMetricHeader(fields)

		return
	}

	if !state.isComparableBenchmarkRow(fields) {
		return
	}

	state.updateBenchmarkSetMismatch(fields)

	row, ok := state.parseComparison(fields, opts)
	if ok {
		state.rows = append(state.rows, row)
	}
}

func trimRecord(record []string) []string {
	fields := make([]string, len(record))
	for index, field := range record {
		fields[index] = strings.TrimSpace(field)
	}

	return fields
}

func (state *csvParseState) captureLabels(fields []string) {
	if state.baselineLabel != "" || len(fields) < 4 || strings.TrimSpace(fields[0]) != "" {
		return
	}

	if isCSVMetricHeader(fields) {
		return
	}

	if fields[1] == "" || fields[3] == "" {
		return
	}

	state.baselineLabel = fields[1]
	state.candidateLabel = fields[3]
}

func (state *csvParseState) setMetricHeader(fields []string) {
	state.metric = normalizeMetric(fields[1])
	state.deltaIndex = findFieldIndex(fields, "vs base")
	state.pValueIndex = findFieldIndex(fields, "P")
	state.baseIndex = 1
	state.newIndex = 3
}

func (state *csvParseState) isComparableBenchmarkRow(fields []string) bool {
	return state.metric != "" && fields[0] != "" && !strings.EqualFold(fields[0], "geomean")
}

func (state *csvParseState) updateBenchmarkSetMismatch(fields []string) {
	if !hasField(fields, state.baseIndex) || !hasField(fields, state.newIndex) {
		return
	}

	if fields[state.baseIndex] == "" || fields[state.newIndex] == "" {
		state.hasBenchmarkSetMismatch = true
	}
}

func (state *csvParseState) parseComparison(fields []string, opts Options) (Comparison, bool) {
	if state.isMissingPValue(fields) {
		state.hasRowWithoutP = true

		return Comparison{}, false
	}

	if !hasField(fields, state.deltaIndex) {
		return Comparison{}, false
	}

	delta, ok := parseDeltaPercent(fields[state.deltaIndex])
	if !ok {
		return Comparison{}, false
	}

	pValue, ok := parsePValue(fields[state.pValueIndex])
	if !ok {
		return Comparison{}, false
	}

	significant := pValue <= opts.Alpha && math.Abs(delta) >= opts.MinDeltaPct

	return Comparison{
		Benchmark:      fields[0],
		Metric:         state.metric,
		DeltaPct:       delta,
		PValue:         pValue,
		Significant:    significant,
		Direction:      classify(state.metric, delta, significant),
		BaselineLabel:  state.displayBaselineLabel(),
		CandidateLabel: state.displayCandidateLabel(),
	}, true
}

func (state *csvParseState) displayBaselineLabel() string {
	if state.baselineLabel != "" {
		return displayLabel(state.baselineLabel)
	}

	return fallbackBaselineLabel
}

func (state *csvParseState) displayCandidateLabel() string {
	if state.candidateLabel != "" {
		return displayLabel(state.candidateLabel)
	}

	return fallbackCandidateLabel
}

func (state *csvParseState) rawBaselineLabel() string {
	if state.baselineLabel != "" {
		return state.baselineLabel
	}

	return fallbackBaselineLabel
}

func (state *csvParseState) rawCandidateLabel() string {
	if state.candidateLabel != "" {
		return state.candidateLabel
	}

	return fallbackCandidateLabel
}

func (state *csvParseState) isMissingPValue(fields []string) bool {
	return !hasField(fields, state.pValueIndex) ||
		fields[state.pValueIndex] == "" ||
		fields[state.pValueIndex] == "?"
}

func parsePValue(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if match := pValueRe.FindStringSubmatch(raw); match != nil && match[1] != "n/a" {
		pValue, err := strconv.ParseFloat(match[1], 64)

		return pValue, err == nil
	}

	pValue, err := strconv.ParseFloat(raw, 64)

	return pValue, err == nil
}

func hasField(fields []string, index int) bool {
	return index >= 0 && index < len(fields)
}

func isCSVMetricHeader(fields []string) bool {
	if len(fields) < 2 || strings.TrimSpace(fields[0]) != "" {
		return false
	}

	hasVsBase := false
	hasP := false

	for _, field := range fields {
		switch strings.TrimSpace(strings.ToLower(field)) {
		case "vs base":
			hasVsBase = true
		case "p":
			hasP = true
		}
	}

	return hasVsBase && hasP
}

func findFieldIndex(fields []string, target string) int {
	target = strings.ToLower(strings.TrimSpace(target))

	for i, field := range fields {
		if strings.ToLower(strings.TrimSpace(field)) == target {
			return i
		}
	}

	return -1
}

func parseDeltaPercent(rawDelta string) (float64, bool) {
	rawDelta = strings.TrimSpace(strings.TrimSuffix(rawDelta, "%"))
	if rawDelta == "" || rawDelta == "~" || rawDelta == "?" {
		return 0, false
	}

	delta, err := strconv.ParseFloat(rawDelta, 64)
	if err != nil {
		return 0, false
	}

	return delta, true
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

func looksLikeComparisonLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < minimumComparisonFields {
		return false
	}

	name := fields[0]

	return !strings.HasPrefix(name, "│") && strings.Contains(name, "-")
}

func inconclusiveReport(reason string) Report {
	return labeledInconclusiveReport(reason, "", "")
}

func labeledInconclusiveReport(reason, baselineLabel, candidateLabel string) Report {
	return Report{
		Verdicts: []BenchmarkVerdict{{
			Benchmark:      "all",
			Outcome:        Inconclusive,
			BaselineLabel:  baselineLabel,
			CandidateLabel: candidateLabel,
			Metrics:        nil,
			Reason:         "inconclusive input",
			ReasonCode:     reason,
		}},
	}
}

func shouldSkip(line string) bool {
	return line == "" ||
		strings.HasPrefix(line, "goos:") ||
		strings.HasPrefix(line, "goarch:") ||
		strings.HasPrefix(line, "pkg:") ||
		strings.HasPrefix(line, "cpu:")
}

func parseComparisonLine(line, metric string, opts Options) (Comparison, bool) {
	pMatch := pValueRe.FindStringSubmatch(line)
	if pMatch == nil || pMatch[1] == "n/a" || metric == "" {
		return Comparison{}, false
	}

	pValue, _ := strconv.ParseFloat(pMatch[1], 64)

	dMatch := deltaRe.FindStringSubmatch(line)
	if dMatch == nil {
		return Comparison{}, false
	}

	isApproxEqual := strings.TrimSpace(dMatch[0]) == "~"
	delta := 0.0

	if !isApproxEqual {
		parsedDelta, _ := parseDeltaPercent(dMatch[1])
		delta = parsedDelta
	}

	name := benchNameCut.Split(line, benchmarkSplitCount)[0]
	significant := pValue <= opts.Alpha &&
		math.Abs(delta) >= opts.MinDeltaPct &&
		!isApproxEqual

	return Comparison{
		Benchmark:   name,
		Metric:      metric,
		DeltaPct:    delta,
		PValue:      pValue,
		Significant: significant,
		Direction:   classify(metric, delta, significant),
	}, true
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

func normalizeMetric(metric string) string {
	metric = strings.TrimSpace(metric)

	switch strings.ToLower(metric) {
	case "time/op", "ns/op":
		return metricSecPerOp
	case "bytes/op", "b/op":
		return "B/op"
	default:
		return metric
	}
}

func evaluate(rows []Comparison) Report {
	grouped := map[string][]Comparison{}

	for _, row := range rows {
		grouped[row.Benchmark] = append(grouped[row.Benchmark], row)
	}

	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}

	sort.Strings(names)

	verdicts := make([]BenchmarkVerdict, 0, len(names))

	for _, name := range names {
		metrics := grouped[name]
		sort.Slice(metrics, func(i, j int) bool {
			return metrics[i].Metric < metrics[j].Metric
		})

		improved, worsened := 0, 0

		for _, metric := range metrics {
			switch metric.Direction {
			case Improved:
				improved++
			case Worsened:
				worsened++
			case Same:
				continue
			}
		}

		outcome, reason := decide(improved, worsened, len(metrics))
		baselineLabel, candidateLabel := comparisonLabels(metrics)

		verdicts = append(verdicts, BenchmarkVerdict{
			Benchmark:      name,
			Outcome:        outcome,
			Winner:         winnerLabel(outcome, baselineLabel, candidateLabel),
			BaselineLabel:  baselineLabel,
			CandidateLabel: candidateLabel,
			Metrics:        metrics,
			Reason:         reason,
			ReasonCode:     "",
		})
	}

	return Report{Verdicts: verdicts}
}

func comparisonLabels(metrics []Comparison) (string, string) {
	if len(metrics) == 0 {
		return fallbackBaselineLabel, fallbackCandidateLabel
	}

	baselineLabel := metrics[0].BaselineLabel
	candidateLabel := metrics[0].CandidateLabel

	if baselineLabel == "" {
		baselineLabel = fallbackBaselineLabel
	}

	if candidateLabel == "" {
		candidateLabel = fallbackCandidateLabel
	}

	return baselineLabel, candidateLabel
}

func winnerLabel(outcome Outcome, baselineLabel, candidateLabel string) string {
	switch outcome {
	case NewWins:
		return candidateLabel
	case OldWins:
		return baselineLabel
	case Tie, TradeOff, Inconclusive:
		return ""
	default:
		return ""
	}
}

func decide(improved, worsened, total int) (Outcome, string) {
	switch {
	case total == 0:
		return Inconclusive, "no comparable metrics"
	case improved > 0 && worsened == 0:
		return NewWins, "new is Pareto-superior: better in one or more metrics and not worse in any metric"
	case worsened > 0 && improved == 0:
		return OldWins, "old is Pareto-superior: new is worse in one or more metrics and not better in any metric"
	case improved == 0 && worsened == 0:
		return Tie, "no statistically significant practical difference"
	default:
		return TradeOff, "significant improvements and regressions coexist"
	}
}

// WriteText writes the report in a compact text format.
func (r Report) WriteText(w io.Writer) error {
	for _, verdict := range r.Verdicts {
		if err := writeTextVerdict(w, verdict); err != nil {
			return err
		}
	}

	return nil
}

func writeTextVerdict(writer io.Writer, verdict BenchmarkVerdict) error {
	_, err := fmt.Fprintf(writer, "%s: %s\n", verdict.Benchmark, textOutcome(verdict))
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingTextOutput, err)
	}

	return nil
}

func textOutcome(verdict BenchmarkVerdict) string {
	if verdict.Winner != "" {
		return verdict.Winner + " wins"
	}

	return string(verdict.Outcome)
}

// WriteVerboseText writes the report in a detailed text format.
func (r Report) WriteVerboseText(w io.Writer) error {
	for _, verdict := range r.Verdicts {
		if err := writeVerboseTextVerdict(w, verdict); err != nil {
			return err
		}
	}

	return nil
}

func writeVerboseTextVerdict(writer io.Writer, verdict BenchmarkVerdict) error {
	if err := writeTextVerdict(writer, verdict); err != nil {
		return err
	}

	_, err := fmt.Fprintf(writer, "  %s\n", verdict.Reason)
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingTextOutput, err)
	}

	if verdict.ReasonCode != "" {
		_, err = fmt.Fprintf(writer, "  reason_code=%s\n", verdict.ReasonCode)
		if err != nil {
			return fmt.Errorf("%w: %w", errWritingTextOutput, err)
		}
	}

	for _, metric := range verdict.Metrics {
		if err := writeTextMetric(writer, metric); err != nil {
			return err
		}
	}

	return nil
}

func writeTextMetric(writer io.Writer, metric Comparison) error {
	_, err := fmt.Fprintf(
		writer,
		"  %s %-9s %8.2f%% p=%.3g %s\n",
		directionMark(metric.Direction),
		metric.Metric,
		metric.DeltaPct,
		metric.PValue,
		metric.Direction,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingTextOutput, err)
	}

	return nil
}

func directionMark(direction Direction) string {
	switch direction {
	case Improved:
		return "+"
	case Worsened:
		return "-"
	case Same:
		return "="
	default:
		return "="
	}
}

// WriteJSON writes the report as indented JSON.
func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("%w: %w", errWritingJSONOutput, err)
	}

	return nil
}
