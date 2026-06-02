// Package rawbench decodes raw Go benchmark input.
package rawbench

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser"
)

const (
	rawBenchmarkMinFields = 4
	rawBenchmarkNameParts = 2
	rawBenchmarkValueUnit = 2
)

var (
	errReadingInput        = errors.New("reading benchstat input")
	errScanningGoTestBench = errors.New("scanning raw go test -bench input")
	errScanningFile        = errors.New("scanning raw benchmark file input")
)

// Samples stores raw go test -bench sub-benchmarks as parent benchmark, label, metric, and samples.
type Samples map[string]map[string]map[string][]float64

// GoTestBench contains decoded raw go test -bench sub-benchmarks and input-shape flags.
type GoTestBench struct {
	Samples            Samples
	HasBenchmarkRows   bool
	HasMalformedRows   bool
	HasUnsupportedRows bool
}

// File contains one decoded raw benchmark series and input-shape flags.
type File struct {
	Name               string
	Metrics            map[string][]float64
	HasBenchmarkRows   bool
	HasMalformedRows   bool
	HasUnsupportedRows bool
	HasMultipleSeries  bool
}

type decodedLine struct {
	name    string
	metrics map[string]float64
}

type sample struct {
	parent  string
	label   string
	metrics map[string]float64
}

// ParseGoTestBench decodes raw go test -bench sub-benchmarks.
func ParseGoTestBench(input string) (GoTestBench, error) {
	result := GoTestBench{
		Samples:            make(Samples),
		HasBenchmarkRows:   false,
		HasMalformedRows:   false,
		HasUnsupportedRows: false,
	}

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		result.handleLine(scanner.Text())
	}

	err := scanner.Err()
	if err != nil {
		return GoTestBench{}, fmt.Errorf("%w: %w", errScanningGoTestBench, err)
	}

	return result, nil
}

// LooksLikeGoTestBench reports whether input contains a decodable raw go test -bench row.
func LooksLikeGoTestBench(input string) bool {
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "Benchmark") {
			continue
		}

		if _, ok := parseSample(line); ok {
			return true
		}
	}

	return false
}

// ParseFile decodes one raw Go benchmark result file.
func ParseFile(reader io.Reader) (File, error) {
	input, err := io.ReadAll(reader)
	if err != nil {
		return File{}, fmt.Errorf("%w: %w", errReadingInput, err)
	}

	result := File{
		Name:               "",
		Metrics:            map[string][]float64{},
		HasBenchmarkRows:   false,
		HasMalformedRows:   false,
		HasUnsupportedRows: false,
		HasMultipleSeries:  false,
	}
	scanner := bufio.NewScanner(strings.NewReader(string(input)))

	for scanner.Scan() {
		result.handleLine(scanner.Text())
	}

	err = scanner.Err()
	if err != nil {
		return File{}, fmt.Errorf("%w: %w", errScanningFile, err)
	}

	return result, nil
}

func (result *GoTestBench) handleLine(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "Benchmark") {
		return
	}

	result.HasBenchmarkRows = true

	parsed, ok := parseSample(line)
	if !ok {
		result.HasMalformedRows = true

		return
	}

	if len(parsed.metrics) == 0 {
		result.HasUnsupportedRows = true

		return
	}

	result.addSample(parsed)
}

func (result *GoTestBench) addSample(parsed sample) {
	if _, ok := result.Samples[parsed.parent]; !ok {
		result.Samples[parsed.parent] = map[string]map[string][]float64{}
	}

	if _, ok := result.Samples[parsed.parent][parsed.label]; !ok {
		result.Samples[parsed.parent][parsed.label] = map[string][]float64{}
	}

	for metric, value := range parsed.metrics {
		result.Samples[parsed.parent][parsed.label][metric] = append(
			result.Samples[parsed.parent][parsed.label][metric],
			value,
		)
	}
}

func (result *File) handleLine(line string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "Benchmark") {
		return
	}

	result.HasBenchmarkRows = true

	name, metrics, ok := parseFileLine(line)
	if !ok {
		result.HasMalformedRows = true

		return
	}

	if len(metrics) == 0 {
		result.HasUnsupportedRows = true

		return
	}

	if result.Name != "" && result.Name != name {
		result.HasMultipleSeries = true

		return
	}

	result.Name = name
	for metric, value := range metrics {
		result.Metrics[metric] = append(result.Metrics[metric], value)
	}
}

func parseSample(line string) (sample, bool) {
	var zeroSample sample

	decoded, ok := decodeLine(line)
	if !ok {
		return zeroSample, false
	}

	parent, label, ok := splitName(decoded.name)
	if !ok {
		return zeroSample, false
	}

	return sample{parent: parent, label: label, metrics: decoded.metrics}, true
}

func parseFileLine(line string) (string, map[string]float64, bool) {
	decoded, ok := decodeLine(line)
	if !ok {
		return "", nil, false
	}

	name := trimCPUSuffix(decoded.name)
	if name == "" {
		return "", nil, false
	}

	return name, decoded.metrics, true
}

func decodeLine(line string) (decodedLine, bool) {
	var zeroLine decodedLine

	fields := strings.Fields(line)
	if len(fields) < rawBenchmarkMinFields {
		return zeroLine, false
	}

	_, err := strconv.Atoi(fields[1])
	if err != nil {
		return zeroLine, false
	}

	if len(fields[2:])%rawBenchmarkValueUnit != 0 {
		return zeroLine, false
	}

	metrics, ok := parseMetrics(fields[2:])
	if !ok {
		return zeroLine, false
	}

	return decodedLine{name: fields[0], metrics: metrics}, true
}

func parseMetrics(fields []string) (map[string]float64, bool) {
	metrics := map[string]float64{}

	for index := 0; index+1 < len(fields); index += rawBenchmarkValueUnit {
		metric, ok := benchparser.NormalizeRawMetric(fields[index+1])
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

func splitName(name string) (string, string, bool) {
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
