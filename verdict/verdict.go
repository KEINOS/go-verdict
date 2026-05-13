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

var (
	deltaRe      = regexp.MustCompile(`([+-]\d+(?:\.\d+)?)%|\s~\s`)
	pValueRe     = regexp.MustCompile(`\bp=([0-9]*\.?[0-9]+|n/a)\b`)
	metricRe     = regexp.MustCompile(metricPattern)
	oldHeaderRe  = regexp.MustCompile(`(?i)^name\s+old\s+(\S+)\s+new\s+\S+\s+delta`)
	benchNameCut = regexp.MustCompile(`\s+`)
)

const (
	defaultAlpha            = 0.05
	metricPattern           = `(?i)\b(?:sec/op|ns/op|time/op|b/op|bytes/op|allocs/op|[a-zµμ]+/op|[a-zµμ]+/s)\b`
	metricSecPerOp          = "sec/op"
	minimumComparisonFields = 2
	benchmarkSplitCount     = 2
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
	if opts.Alpha == 0 {
		opts.Alpha = defaultAlpha
	}

	input, err := io.ReadAll(r)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %w", errReadingInput, err)
	}

	text := string(input)
	if strings.Contains(text, ",vs base,P") {
		return parseCSV(text, opts)
	}

	return parseText(text, opts)
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
