// Package benchstat decodes benchstat text and CSV input.
package benchstat

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser"
)

const (
	// ReasonBenchmarkSetMismatch identifies benchstat input with different benchmark sets.
	ReasonBenchmarkSetMismatch = "benchmark-set-mismatch"
	// ReasonMissingPValue identifies benchstat comparisons without a usable p-value.
	ReasonMissingPValue = "missing-pvalue"

	metricPattern           = `(?i)\b(?:sec/op|ns/op|time/op|b/op|bytes/op|allocs/op|[a-zµμ]+/op|[a-zµμ]+/s)\b`
	minimumComparisonFields = 2
	benchmarkSplitCount     = 2
)

var (
	// ErrNoComparisonRows indicates that benchstat input has no usable comparison rows.
	ErrNoComparisonRows = errors.New("no benchstat comparison rows found")

	errReadingCSVInput   = errors.New("reading benchstat csv input")
	errScanningTextInput = errors.New("scanning benchstat text input")
	deltaRe              = regexp.MustCompile(`([+-]\d+(?:\.\d+)?)%|\s~\s`)
	pValueRe             = regexp.MustCompile(`\bp=([0-9]*\.?[0-9]+|n/a)\b`)
	metricRe             = regexp.MustCompile(metricPattern)
	oldHeaderRe          = regexp.MustCompile(`(?i)^name\s+old\s+(\S+)\s+new\s+\S+\s+delta`)
	benchNameCut         = regexp.MustCompile(`\s+`)
)

// Result contains decoded comparisons or a neutral inconclusive input state.
type Result struct {
	Comparisons        []benchparser.Comparison
	InconclusiveReason string
	BaselineLabel      string
	CandidateLabel     string
	IncludeLabels      bool
}

// Parse decodes benchstat text or CSV input.
func Parse(input string) (Result, error) {
	if strings.Contains(input, ",vs base,P") {
		return parseCSV(input)
	}

	return parseText(input)
}

type csvParseState struct {
	rows                    []benchparser.Comparison
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

func parseCSV(input string) (Result, error) {
	reader := csv.NewReader(strings.NewReader(input))
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", errReadingCSVInput, err)
	}

	state := newCSVParseState()
	for _, record := range records {
		state.handleRecord(record)
	}

	if len(state.rows) == 0 {
		switch {
		case state.hasBenchmarkSetMismatch:
			return state.inconclusiveResult(ReasonBenchmarkSetMismatch, true), nil
		case state.hasRowWithoutP:
			return state.inconclusiveResult(ReasonMissingPValue, false), nil
		default:
			return Result{}, ErrNoComparisonRows
		}
	}

	return Result{
		Comparisons:        state.rows,
		InconclusiveReason: "",
		BaselineLabel:      "",
		CandidateLabel:     "",
		IncludeLabels:      false,
	}, nil
}

func newCSVParseState() csvParseState {
	return csvParseState{
		rows:                    make([]benchparser.Comparison, 0),
		deltaIndex:              -1,
		pValueIndex:             -1,
		baseIndex:               -1,
		newIndex:                -1,
		metric:                  "",
		baselineLabel:           "",
		candidateLabel:          "",
		hasRowWithoutP:          false,
		hasBenchmarkSetMismatch: false,
	}
}

func (state *csvParseState) handleRecord(record []string) {
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

	row, ok := state.parseComparison(fields)
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
	if state.baselineLabel != "" || len(fields) < 4 || fields[0] != "" || isCSVMetricHeader(fields) {
		return
	}

	if fields[1] == "" || fields[3] == "" {
		return
	}

	state.baselineLabel = fields[1]
	state.candidateLabel = fields[3]
}

func (state *csvParseState) setMetricHeader(fields []string) {
	state.metric = benchparser.NormalizeMetric(fields[1])
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

func (state *csvParseState) parseComparison(fields []string) (benchparser.Comparison, bool) {
	var zeroComparison benchparser.Comparison

	if state.isMissingPValue(fields) {
		state.hasRowWithoutP = true

		return zeroComparison, false
	}

	if !hasField(fields, state.deltaIndex) {
		return zeroComparison, false
	}

	isApproxEqual := strings.TrimSpace(fields[state.deltaIndex]) == "~"
	delta := 0.0

	if !isApproxEqual {
		var ok bool

		delta, ok = parseDeltaPercent(fields[state.deltaIndex])
		if !ok {
			return zeroComparison, false
		}
	}

	pValue, ok := parsePValue(fields[state.pValueIndex])
	if !ok {
		return zeroComparison, false
	}

	return benchparser.Comparison{
		Benchmark:      fields[0],
		Metric:         state.metric,
		DeltaPct:       delta,
		PValue:         pValue,
		BaselineLabel:  state.baselineLabel,
		CandidateLabel: state.candidateLabel,
		ApproxEqual:    isApproxEqual,
	}, true
}

func (state *csvParseState) isMissingPValue(fields []string) bool {
	return !hasField(fields, state.pValueIndex) ||
		fields[state.pValueIndex] == "" ||
		fields[state.pValueIndex] == "?"
}

func (state *csvParseState) inconclusiveResult(reason string, includeLabels bool) Result {
	return Result{
		Comparisons:        nil,
		InconclusiveReason: reason,
		BaselineLabel:      state.baselineLabel,
		CandidateLabel:     state.candidateLabel,
		IncludeLabels:      includeLabels,
	}
}

type textParseState struct {
	rows                      []benchparser.Comparison
	currentMetric             string
	baselineLabel             string
	candidateLabel            string
	hasBenchmarkSetMismatch   bool
	hasComparisonRowsWithoutP bool
}

func parseText(input string) (Result, error) {
	state := textParseState{
		rows:                      make([]benchparser.Comparison, 0),
		currentMetric:             "",
		baselineLabel:             "",
		candidateLabel:            "",
		hasBenchmarkSetMismatch:   false,
		hasComparisonRowsWithoutP: false,
	}

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		state.handleLine(scanner.Text())
	}

	err := scanner.Err()
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", errScanningTextInput, err)
	}

	if len(state.rows) == 0 {
		switch {
		case state.hasBenchmarkSetMismatch:
			return state.inconclusiveResult(ReasonBenchmarkSetMismatch), nil
		case state.hasComparisonRowsWithoutP:
			return state.inconclusiveResult(ReasonMissingPValue), nil
		default:
			return Result{}, ErrNoComparisonRows
		}
	}

	return Result{
		Comparisons:        state.rows,
		InconclusiveReason: "",
		BaselineLabel:      "",
		CandidateLabel:     "",
		IncludeLabels:      false,
	}, nil
}

func (state *textParseState) handleLine(rawLine string) {
	line := strings.TrimSpace(rawLine)
	if shouldSkip(line) {
		return
	}

	if strings.Contains(line, "benchmark set differs") {
		state.hasBenchmarkSetMismatch = true
	}

	state.captureLabels(line)

	if match := oldHeaderRe.FindStringSubmatch(line); match != nil {
		state.currentMetric = benchparser.NormalizeMetric(match[1])

		return
	}

	if !strings.Contains(line, "p=") {
		state.handleLineWithoutPValue(line)

		return
	}

	row, ok := parseComparisonLine(line, state.currentMetric)
	if ok {
		row.BaselineLabel = state.baselineLabel
		row.CandidateLabel = state.candidateLabel
		state.rows = append(state.rows, row)
	}
}

func (state *textParseState) captureLabels(line string) {
	if state.baselineLabel != "" || !strings.Contains(line, "│") {
		return
	}

	labels, ok := parseTextLabels(line)
	if !ok {
		return
	}

	state.baselineLabel = labels[0]
	state.candidateLabel = labels[1]
}

func parseTextLabels(line string) ([2]string, bool) {
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

func (state *textParseState) handleLineWithoutPValue(line string) {
	if matches := metricRe.FindAllString(line, -1); len(matches) > 0 {
		state.currentMetric = benchparser.NormalizeMetric(matches[0])
	}

	if state.currentMetric != "" && looksLikeComparisonLine(line) {
		state.hasComparisonRowsWithoutP = true
	}
}

func (state *textParseState) inconclusiveResult(reason string) Result {
	return Result{
		Comparisons:        nil,
		InconclusiveReason: reason,
		BaselineLabel:      state.baselineLabel,
		CandidateLabel:     state.candidateLabel,
		IncludeLabels:      true,
	}
}

func looksLikeComparisonLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < minimumComparisonFields {
		return false
	}

	return !strings.HasPrefix(fields[0], "│") && strings.Contains(fields[0], "-")
}

func shouldSkip(line string) bool {
	return line == "" ||
		strings.HasPrefix(line, "goos:") ||
		strings.HasPrefix(line, "goarch:") ||
		strings.HasPrefix(line, "pkg:") ||
		strings.HasPrefix(line, "cpu:")
}

func parseComparisonLine(line, metric string) (benchparser.Comparison, bool) {
	var zeroComparison benchparser.Comparison

	pMatch := pValueRe.FindStringSubmatch(line)
	if pMatch == nil || pMatch[1] == "n/a" || metric == "" {
		return zeroComparison, false
	}

	pValue, _ := strconv.ParseFloat(pMatch[1], 64)

	dMatch := deltaRe.FindStringSubmatch(line)
	if dMatch == nil {
		return zeroComparison, false
	}

	isApproxEqual := strings.TrimSpace(dMatch[0]) == "~"
	delta := 0.0

	if !isApproxEqual {
		delta, _ = parseDeltaPercent(dMatch[1])
	}

	return benchparser.Comparison{
		Benchmark:      benchNameCut.Split(line, benchmarkSplitCount)[0],
		Metric:         metric,
		DeltaPct:       delta,
		PValue:         pValue,
		BaselineLabel:  "",
		CandidateLabel: "",
		ApproxEqual:    isApproxEqual,
	}, true
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

func hasField(fields []string, index int) bool {
	return index >= 0 && index < len(fields)
}

func isCSVMetricHeader(fields []string) bool {
	if len(fields) < 2 || fields[0] != "" {
		return false
	}

	hasVsBase, hasP := false, false

	for _, field := range fields {
		switch strings.ToLower(strings.TrimSpace(field)) {
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
	for index, field := range fields {
		if strings.ToLower(strings.TrimSpace(field)) == target {
			return index
		}
	}

	return -1
}
