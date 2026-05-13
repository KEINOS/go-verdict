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
type Options struct {
	Alpha       float64
	MinDeltaPct float64
	Mode        string
	Baseline    string
	Candidate   string
}

// Comparison is one parsed metric comparison for one benchmark.
type Comparison struct {
	Benchmark   string    `json:"benchmark"`
	Metric      string    `json:"metric"`
	DeltaPct    float64   `json:"delta_pct"`
	PValue      float64   `json:"p_value"`
	Significant bool      `json:"significant"`
	Direction   Direction `json:"direction"`
}

// BenchmarkVerdict is the final outcome for one benchmark name.
type BenchmarkVerdict struct {
	Benchmark  string       `json:"benchmark"`
	Outcome    Outcome      `json:"outcome"`
	Metrics    []Comparison `json:"metrics"`
	Reason     string       `json:"reason"`
	ReasonCode string       `json:"reason_code,omitempty"`
}

// Report is the complete parse and evaluation result.
type Report struct {
	Verdicts []BenchmarkVerdict `json:"verdicts"`
}

type textParseState struct {
	rows                      []Comparison
	currentMetric             string
	hasComparisonRowsWithoutP bool
}

type csvParseState struct {
	rows                    []Comparison
	metric                  string
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

var (
	deltaRe      = regexp.MustCompile(`([+-]\d+(?:\.\d+)?)%|\s~\s`)
	pValueRe     = regexp.MustCompile(`\bp=([0-9]*\.?[0-9]+|n/a)\b`)
	metricRe     = regexp.MustCompile(metricPattern)
	oldHeaderRe  = regexp.MustCompile(`(?i)^name\s+old\s+(\S+)\s+new\s+\S+\s+delta`)
	benchNameCut = regexp.MustCompile(`\s+`)
)

const (
	defaultAlpha            = 0.05
	defaultBaseline         = "original"
	defaultCandidate        = "enhanced"
	modeAlternatives        = "alternatives"
	modeBenchstat           = "benchstat"
	metricPattern           = `(?i)\b(?:sec/op|ns/op|time/op|b/op|bytes/op|allocs/op|[a-zµμ]+/op|[a-zµμ]+/s)\b`
	metricNanosecondsPerOp  = "ns/op"
	metricSecPerOp          = "sec/op"
	metricBytesPerOp        = "B/op"
	metricAllocsPerOp       = "allocs/op"
	minimumComparisonFields = 2
	benchmarkSplitCount     = 2
	minimumSamples          = 2
	rawBenchmarkMinFields   = 4
	rawBenchmarkNameParts   = 2
	rawBenchmarkValueUnit   = 2
	percentScale            = 100
)

var (
	errReadingInput      = errors.New("reading benchstat input")
	errReadingCSVInput   = errors.New("reading benchstat csv input")
	errScanningTextInput = errors.New("scanning benchstat text input")
	errNoComparisonRows  = errors.New("no benchstat comparison rows found")
	errWritingTextOutput = errors.New("writing text report")
	errWritingJSONOutput = errors.New("writing json report")
)

// Parse reads benchstat output and returns a benchmark verdict report.
func Parse(r io.Reader, opts Options) (Report, error) {
	opts = normalizeOptions(opts)

	input, err := io.ReadAll(r)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %w", errReadingInput, err)
	}

	text := string(input)
	if opts.Mode == modeAlternatives {
		return parseAlternatives(text, opts), nil
	}

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
		opts.Mode = modeBenchstat
	}

	if opts.Baseline == "" {
		opts.Baseline = defaultBaseline
	}

	if opts.Candidate == "" {
		opts.Candidate = defaultCandidate
	}

	return opts
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
		if state.hasComparisonRowsWithoutP {
			return inconclusiveReport("missing-pvalue"), nil
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
		state.rows = append(state.rows, row)
	}
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
			return inconclusiveReport("benchmark-set-mismatch"), nil
		}

		if state.hasRowWithoutP {
			return inconclusiveReport("missing-pvalue"), nil
		}

		return Report{}, errNoComparisonRows
	}

	return evaluate(state.rows), nil
}

func parseAlternatives(input string, opts Options) Report {
	state := newAlternativeParseState()

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		state.handleLine(scanner.Text())
	}

	if !state.hasBenchmarkRows {
		return inconclusiveReport("malformed-benchmark")
	}

	report := state.evaluate(opts)
	if len(report.Verdicts) == 0 {
		return state.emptyAlternativeReport()
	}

	return report
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

	metrics := parseRawMetrics(fields[2:])

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

func parseRawMetrics(fields []string) map[string]float64 {
	metrics := map[string]float64{}

	for index := 0; index+1 < len(fields); index += rawBenchmarkValueUnit {
		value, err := strconv.ParseFloat(fields[index], 64)
		if err != nil {
			continue
		}

		metric, ok := normalizeRawMetric(fields[index+1])
		if ok {
			metrics[metric] = value
		}
	}

	return metrics
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
	baselineMetrics, hasBaseline := labels[opts.Baseline]
	candidateMetrics, hasCandidate := labels[opts.Candidate]

	switch {
	case !hasBaseline:
		return nil, alternativeInconclusive(parent, "missing-baseline")
	case !hasCandidate:
		return nil, alternativeInconclusive(parent, "missing-candidate")
	}

	rows, ok := compareAlternativeMetrics(parent, baselineMetrics, candidateMetrics, opts)
	if !ok {
		state.hasInsufficientRows = true

		return nil, alternativeInconclusive(parent, "insufficient-samples")
	}

	if len(rows) == 0 {
		return nil, alternativeInconclusive(parent, "unsupported-metric")
	}

	return rows, nil
}

func compareAlternativeMetrics(
	parent string,
	baselineMetrics map[string][]float64,
	candidateMetrics map[string][]float64,
	opts Options,
) ([]Comparison, bool) {
	metrics := commonMetrics(baselineMetrics, candidateMetrics)
	rows := make([]Comparison, 0, len(metrics))

	for _, metric := range metrics {
		baseline := baselineMetrics[metric]
		candidate := candidateMetrics[metric]

		if len(baseline) < minimumSamples || len(candidate) < minimumSamples {
			return nil, false
		}

		rows = append(rows, compareAlternativeMetric(parent, metric, baseline, candidate, opts))
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
		Benchmark:   parent,
		Metric:      metric,
		DeltaPct:    delta,
		PValue:      pValue,
		Significant: significant,
		Direction:   classify(metric, delta, significant),
	}
}

func mean(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

func variance(values []float64, sampleMean float64) float64 {
	if len(values) < minimumSamples {
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

	pValue, err := strconv.ParseFloat(fields[state.pValueIndex], 64)
	if err != nil {
		return Comparison{}, false
	}

	significant := pValue <= opts.Alpha && math.Abs(delta) >= opts.MinDeltaPct

	return Comparison{
		Benchmark:   fields[0],
		Metric:      state.metric,
		DeltaPct:    delta,
		PValue:      pValue,
		Significant: significant,
		Direction:   classify(state.metric, delta, significant),
	}, true
}

func (state *csvParseState) isMissingPValue(fields []string) bool {
	return !hasField(fields, state.pValueIndex) ||
		fields[state.pValueIndex] == "" ||
		fields[state.pValueIndex] == "?"
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

func looksLikeComparisonLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < minimumComparisonFields {
		return false
	}

	name := fields[0]

	return !strings.HasPrefix(name, "│") && strings.Contains(name, "-")
}

func inconclusiveReport(reason string) Report {
	return Report{
		Verdicts: []BenchmarkVerdict{{
			Benchmark:  "all",
			Outcome:    Inconclusive,
			Metrics:    nil,
			Reason:     "inconclusive input",
			ReasonCode: reason,
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

		verdicts = append(verdicts, BenchmarkVerdict{
			Benchmark:  name,
			Outcome:    outcome,
			Metrics:    metrics,
			Reason:     reason,
			ReasonCode: "",
		})
	}

	return Report{Verdicts: verdicts}
}

func decide(improved, worsened, total int) (Outcome, string) {
	switch {
	case total == 0:
		return Inconclusive, "no comparable metrics"
	case improved > 0 && worsened == 0:
		return NewWins, "new is Pareto-superior: at least one significant improvement and no significant regressions"
	case worsened > 0 && improved == 0:
		return OldWins, "old is Pareto-superior: new has significant regressions and no significant improvements"
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
	_, err := fmt.Fprintf(writer, "%s: %s\n", verdict.Benchmark, verdict.Outcome)
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingTextOutput, err)
	}

	_, err = fmt.Fprintf(writer, "  %s\n", verdict.Reason)
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
