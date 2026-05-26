package verdict

import (
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

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

var (
	errReadingCSVInput = errors.New("reading benchstat csv input")
)

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

func newCSVParseState() csvParseState {
	var zeroState csvParseState

	zeroState.deltaIndex = -1
	zeroState.pValueIndex = -1
	zeroState.baseIndex = -1
	zeroState.newIndex = -1
	zeroState.rows = make([]Comparison, 0)

	return zeroState
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
	var zeroComparison Comparison

	if state.isMissingPValue(fields) {
		state.hasRowWithoutP = true

		return zeroComparison, false
	}

	if !hasField(fields, state.deltaIndex) {
		return zeroComparison, false
	}

	delta, ok := parseDeltaPercent(fields[state.deltaIndex])
	if !ok {
		return zeroComparison, false
	}

	pValue, ok := parsePValue(fields[state.pValueIndex])
	if !ok {
		return zeroComparison, false
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
	return displayLabelWithFallback(state.baselineLabel, fallbackBaselineLabel)
}

func (state *csvParseState) displayCandidateLabel() string {
	return displayLabelWithFallback(state.candidateLabel, fallbackCandidateLabel)
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
