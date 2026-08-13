package main

import (
	"errors"
	"fmt"
	"slices"

	"github.com/KEINOS/go-verdict/internal/pareto"
	"github.com/KEINOS/go-verdict/verdict"
)

const (
	complexityStatusCompared  = "compared"
	complexityStatusNotMapped = "not-mapped"
	complexityReasonNoMetrics = "no comparable metrics"
)

var errComplexityMapping = errors.New("applying complexity mapping")

type complexityDetail struct {
	Baseline  *complexityMeasurement `json:"baseline,omitempty"`
	Candidate *complexityMeasurement `json:"candidate,omitempty"`
	Status    string                 `json:"status"`
	Direction verdict.Direction      `json:"direction,omitempty"`
}

type complexityReport struct {
	Details map[string]complexityDetail
	Report  verdict.Report
}

func enrichComplexityReport(
	report verdict.Report,
	mappings map[string]complexityMapping,
	resolver complexityResolver,
) (complexityReport, error) {
	err := validateMappedBenchmarks(report, mappings)
	if err != nil {
		return complexityReport{}, err
	}

	result := complexityReport{
		Details: make(map[string]complexityDetail, len(report.Verdicts)),
		Report: verdict.Report{
			Verdicts: slices.Clone(report.Verdicts),
		},
	}

	for index := range result.Report.Verdicts {
		item := &result.Report.Verdicts[index]

		mapping, mapped := mappings[item.Benchmark]
		if !mapped {
			result.Details[item.Benchmark] = complexityDetail{
				Baseline:  nil,
				Candidate: nil,
				Status:    complexityStatusNotMapped,
				Direction: "",
			}

			continue
		}

		detail, err := resolveComplexityDetail(mapping, resolver)
		if err != nil {
			return complexityReport{}, fmt.Errorf("%w for %q: %w",
				errComplexityMapping, item.Benchmark, err)
		}

		result.Details[item.Benchmark] = detail
		if item.Outcome != verdict.Inconclusive {
			applyComplexityOutcome(item, detail.Direction)
		}
	}

	return result, nil
}

func validateMappedBenchmarks(report verdict.Report, mappings map[string]complexityMapping) error {
	reportNames := make(map[string]struct{}, len(report.Verdicts))
	for _, item := range report.Verdicts {
		reportNames[item.Benchmark] = struct{}{}
	}

	for benchmark := range mappings {
		if _, exists := reportNames[benchmark]; !exists {
			return fmt.Errorf("%w: benchmark %q does not exist in benchmark report",
				errComplexityMapping, benchmark)
		}
	}

	return nil
}

func resolveComplexityDetail(
	mapping complexityMapping,
	resolver complexityResolver,
) (complexityDetail, error) {
	baseline, err := resolver.resolve(mapping.Baseline)
	if err != nil {
		return complexityDetail{}, fmt.Errorf("resolving baseline: %w", err)
	}

	candidate, err := resolver.resolve(mapping.Candidate)
	if err != nil {
		return complexityDetail{}, fmt.Errorf("resolving candidate: %w", err)
	}

	direction := verdict.Same
	if candidate.Score < baseline.Score {
		direction = verdict.Improved
	} else if candidate.Score > baseline.Score {
		direction = verdict.Worsened
	}

	return complexityDetail{
		Baseline:  &baseline,
		Candidate: &candidate,
		Status:    complexityStatusCompared,
		Direction: direction,
	}, nil
}

func applyComplexityOutcome(item *verdict.BenchmarkVerdict, direction verdict.Direction) {
	relations := make([]pareto.Metric, 0, len(item.Metrics)+1)
	for _, metric := range item.Metrics {
		relations = append(relations, paretoMetric(metric.Direction))
	}

	relations = append(relations, paretoMetric(direction))

	item.Outcome, item.Reason = complexityDecision(pareto.Compare(relations...))
	item.Winner = complexityWinner(*item)
}

func paretoMetric(direction verdict.Direction) pareto.Metric {
	switch direction {
	case verdict.Improved:
		return pareto.Better
	case verdict.Worsened:
		return pareto.Worse
	case verdict.Same:
		return pareto.Same
	default:
		return pareto.Same
	}
}

func complexityDecision(relation pareto.Relation) (verdict.Outcome, string) {
	switch relation {
	case pareto.CandidateWins:
		return verdict.NewWins,
			"new is Pareto-superior: better in one or more metrics and not worse in any metric"
	case pareto.BaselineWins:
		return verdict.OldWins,
			"old is Pareto-superior: new is worse in one or more metrics and not better in any metric"
	case pareto.Tie:
		return verdict.Tie, "no statistically significant practical difference"
	case pareto.TradeOff:
		return verdict.TradeOff, "significant improvements and regressions coexist"
	case pareto.Inconclusive:
		return verdict.Inconclusive, complexityReasonNoMetrics
	default:
		return verdict.Inconclusive, complexityReasonNoMetrics
	}
}

func complexityWinner(item verdict.BenchmarkVerdict) string {
	switch item.Outcome {
	case verdict.NewWins:
		if item.CandidateLabel != "" {
			return item.CandidateLabel
		}

		return "new"
	case verdict.OldWins:
		if item.BaselineLabel != "" {
			return item.BaselineLabel
		}

		return "old"
	case verdict.Tie, verdict.TradeOff, verdict.Inconclusive:
		return ""
	default:
		return ""
	}
}
