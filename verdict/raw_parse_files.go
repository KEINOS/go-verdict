package verdict

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type rawFileParseState struct {
	name               string
	metrics            map[string][]float64
	hasBenchmarkRows   bool
	hasMalformedRows   bool
	hasUnsupportedRows bool
	hasMultipleSeries  bool
}

var errScanningRawFile = errors.New("scanning raw benchmark file input")

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
	decoded, ok := decodeRawBenchmarkLine(line)
	if !ok {
		return "", nil, false
	}

	name := trimCPUSuffix(decoded.name)
	if name == "" {
		return "", nil, false
	}

	return name, decoded.metrics, true
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
