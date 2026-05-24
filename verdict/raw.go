package verdict

import (
	"strconv"
	"strings"
)

type rawDecodedLine struct {
	name    string
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

func decodeRawBenchmarkLine(line string) (rawDecodedLine, bool) {
	var zeroLine rawDecodedLine

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

	metrics, ok := parseRawMetrics(fields[2:])
	if !ok {
		return zeroLine, false
	}

	return rawDecodedLine{
		name:    fields[0],
		metrics: metrics,
	}, true
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
