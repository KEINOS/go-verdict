package verdict

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type textParseState struct {
	rows                      []Comparison
	currentMetric             string
	baselineLabel             string
	candidateLabel            string
	hasBenchmarkSetMismatch   bool
	hasComparisonRowsWithoutP bool
}

var (
	deltaRe      = regexp.MustCompile(`([+-]\d+(?:\.\d+)?)%|\s~\s`)
	pValueRe     = regexp.MustCompile(`\bp=([0-9]*\.?[0-9]+|n/a)\b`)
	metricRe     = regexp.MustCompile(metricPattern)
	oldHeaderRe  = regexp.MustCompile(`(?i)^name\s+old\s+(\S+)\s+new\s+\S+\s+delta`)
	benchNameCut = regexp.MustCompile(`\s+`)
)

const (
	metricPattern           = `(?i)\b(?:sec/op|ns/op|time/op|b/op|bytes/op|allocs/op|[a-zµμ]+/op|[a-zµμ]+/s)\b`
	minimumComparisonFields = 2
	benchmarkSplitCount     = 2
)

var errScanningTextInput = errors.New("scanning benchstat text input")

func parseText(input string, opts Options) (Report, error) {
	var state textParseState

	state.rows = make([]Comparison, 0)

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

func looksLikeComparisonLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < minimumComparisonFields {
		return false
	}

	name := fields[0]

	return !strings.HasPrefix(name, "│") && strings.Contains(name, "-")
}

func shouldSkip(line string) bool {
	return line == "" ||
		strings.HasPrefix(line, "goos:") ||
		strings.HasPrefix(line, "goarch:") ||
		strings.HasPrefix(line, "pkg:") ||
		strings.HasPrefix(line, "cpu:")
}

func parseComparisonLine(line, metric string, opts Options) (Comparison, bool) {
	var zeroComparison Comparison

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
		parsedDelta, _ := parseDeltaPercent(dMatch[1])
		delta = parsedDelta
	}

	name := benchNameCut.Split(line, benchmarkSplitCount)[0]
	significant := pValue <= opts.Alpha &&
		math.Abs(delta) >= opts.MinDeltaPct &&
		!isApproxEqual

	return Comparison{
		Benchmark:      name,
		Metric:         metric,
		DeltaPct:       delta,
		PValue:         pValue,
		Significant:    significant,
		Direction:      classify(metric, delta, significant),
		BaselineLabel:  "",
		CandidateLabel: "",
	}, true
}
