package verdict

import (
	"bufio"
	"errors"
	"fmt"
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

type rawBenchmarkSample struct {
	parent  string
	label   string
	metrics map[string]float64
}

var errScanningRawInput = errors.New("scanning raw alternatives input")

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

	decoded, ok := decodeRawBenchmarkLine(line)
	if !ok {
		return zeroSample, false
	}

	parent, label, ok := splitRawBenchmarkName(decoded.name)
	if !ok {
		return zeroSample, false
	}

	return rawBenchmarkSample{
		parent:  parent,
		label:   label,
		metrics: decoded.metrics,
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
