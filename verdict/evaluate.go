package verdict

import (
	"sort"

	"github.com/KEINOS/go-verdict/verdict/internal/pareto"
)

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
