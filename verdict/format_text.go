package verdict

import (
	"errors"
	"fmt"
	"io"
)

var errWritingTextOutput = errors.New("writing text report")

// WriteText writes the report in a compact text format.
func (r Report) WriteText(w io.Writer) error {
	for _, verdict := range r.Verdicts {
		err := writeTextVerdict(w, verdict)
		if err != nil {
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
		err := writeVerboseTextVerdict(w, verdict)
		if err != nil {
			return err
		}
	}

	return nil
}

func writeVerboseTextVerdict(writer io.Writer, verdict BenchmarkVerdict) error {
	err := writeTextVerdict(writer, verdict)
	if err != nil {
		return err
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

	maxMetricNameLen := maxComparisonMetricNameLen(verdict.Metrics)
	for _, metric := range verdict.Metrics {
		err = writeTextMetric(writer, metric, maxMetricNameLen)
		if err != nil {
			return err
		}
	}

	return nil
}

func maxComparisonMetricNameLen(metrics []Comparison) int {
	maxLen := 0
	for _, metric := range metrics {
		if len(metric.Metric) > maxLen {
			maxLen = len(metric.Metric)
		}
	}

	return maxLen
}

func writeTextMetric(writer io.Writer, metric Comparison, metricNameWidth int) error {
	pValue := fmt.Sprintf("%.3g", metric.PValue)

	_, err := fmt.Fprintf(
		writer,
		"  %s %-*s %8.2f%% p=%12s %s\n",
		directionMark(metric.Direction),
		metricNameWidth,
		metric.Metric,
		metric.DeltaPct,
		pValue,
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
