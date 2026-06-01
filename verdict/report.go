package verdict

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/KEINOS/go-verdict/verdict/internal/pareto"
)

const (
	metricSecPerOp = "sec/op"
)

var (
	errWritingTextOutput = errors.New("writing text report")
	errWritingJSONOutput = errors.New("writing json report")
)

func displayLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return label
	}

	base := filepath.Base(label)
	if base == "." || base == string(filepath.Separator) {
		return label
	}

	return base
}

func displayLabelWithFallback(label, fallback string) string {
	if label != "" {
		return displayLabel(label)
	}

	return fallback
}

func inconclusiveReport(reason string) Report {
	return labeledInconclusiveReport(reason, "", "")
}

func labeledInconclusiveReport(reason, baselineLabel, candidateLabel string) Report {
	return Report{
		Verdicts: []BenchmarkVerdict{{
			Benchmark:      "all",
			Outcome:        Inconclusive,
			Winner:         "",
			BaselineLabel:  baselineLabel,
			CandidateLabel: candidateLabel,
			Metrics:        nil,
			Reason:         "inconclusive input",
			ReasonCode:     reason,
		}},
	}
}

func classify(metric string, delta float64, significant bool) Direction {
	if !significant || delta == 0 {
		// Exact zero is a final guard after significance and min-delta checks.
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
		return metricBytesPerOp
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

		outcome, reason := decide(pareto.Compare(metricRelations(metrics)...))
		baselineLabel, candidateLabel := comparisonLabels(metrics)

		verdicts = append(verdicts, BenchmarkVerdict{
			Benchmark:      name,
			Outcome:        outcome,
			Winner:         winnerLabel(outcome, baselineLabel, candidateLabel),
			BaselineLabel:  baselineLabel,
			CandidateLabel: candidateLabel,
			Metrics:        metrics,
			Reason:         reason,
			ReasonCode:     "",
		})
	}

	return Report{Verdicts: verdicts}
}

func metricRelations(metrics []Comparison) []pareto.Metric {
	relations := make([]pareto.Metric, 0, len(metrics))

	for _, metric := range metrics {
		relations = append(relations, metricRelation(metric.Direction))
	}

	return relations
}

func metricRelation(direction Direction) pareto.Metric {
	switch direction {
	case Improved:
		return pareto.Better
	case Worsened:
		return pareto.Worse
	case Same:
		return pareto.Same
	default:
		return pareto.Same
	}
}

func comparisonLabels(metrics []Comparison) (string, string) {
	if len(metrics) == 0 {
		return fallbackBaselineLabel, fallbackCandidateLabel
	}

	baselineLabel := metrics[0].BaselineLabel
	candidateLabel := metrics[0].CandidateLabel

	if baselineLabel == "" {
		baselineLabel = fallbackBaselineLabel
	}

	if candidateLabel == "" {
		candidateLabel = fallbackCandidateLabel
	}

	return baselineLabel, candidateLabel
}

func winnerLabel(outcome Outcome, baselineLabel, candidateLabel string) string {
	switch outcome {
	case NewWins:
		return candidateLabel
	case OldWins:
		return baselineLabel
	case Tie, TradeOff, Inconclusive:
		return ""
	default:
		return ""
	}
}

func decide(relation pareto.Relation) (Outcome, string) {
	switch relation {
	case pareto.Inconclusive:
		return Inconclusive, "no comparable metrics"
	case pareto.CandidateWins:
		return NewWins, "new is Pareto-superior: better in one or more metrics and not worse in any metric"
	case pareto.BaselineWins:
		return OldWins, "old is Pareto-superior: new is worse in one or more metrics and not better in any metric"
	case pareto.Tie:
		return Tie, "no statistically significant practical difference"
	case pareto.TradeOff:
		return TradeOff, "significant improvements and regressions coexist"
	default:
		return Inconclusive, "no comparable metrics"
	}
}

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

// WriteJSON writes the report as indented JSON.
func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	err := enc.Encode(r)
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingJSONOutput, err)
	}

	return nil
}
