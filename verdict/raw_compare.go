package verdict

import (
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser/rawbench"
	"github.com/KEINOS/go-verdict/verdict/internal/stat"
)

const requiredAlternativePair = 2

func compareRawFiles(aReader io.Reader, bReader io.Reader, opts Options) (Report, error) {
	aState, err := rawbench.ParseFile(aReader)
	if err != nil {
		return Report{}, fmt.Errorf("parsing side a raw benchmark file: %w", err)
	}

	bState, err := rawbench.ParseFile(bReader)
	if err != nil {
		return Report{}, fmt.Errorf("parsing side b raw benchmark file: %w", err)
	}

	if inconclusive := rawFileInconclusive(aState, bState); inconclusive != nil {
		return Report{Verdicts: []BenchmarkVerdict{*inconclusive}}, nil
	}

	// Raw-file comparison treats two separate benchmark series as explicit A/B
	// alternatives, unlike raw stdin mode where labels come from sub-benchmarks.
	benchmark := aState.Name + "_vs_" + bState.Name

	rows, ok := compareAlternativeMetrics(
		benchmark,
		aState.Name,
		bState.Name,
		aState.Metrics,
		bState.Metrics,
		opts,
	)
	if !ok {
		return Report{Verdicts: []BenchmarkVerdict{*alternativeInconclusive(benchmark, ReasonInsufficientSamples)}}, nil
	}

	if len(rows) == 0 {
		return Report{Verdicts: []BenchmarkVerdict{*alternativeInconclusive(benchmark, ReasonUnsupportedMetric)}}, nil
	}

	return evaluate(rows), nil
}

func rawFileInconclusive(aState, bState rawbench.File) *BenchmarkVerdict {
	switch {
	case !aState.HasBenchmarkRows || !bState.HasBenchmarkRows:
		return alternativeInconclusive("all", ReasonMalformedBenchmark)
	case aState.HasMalformedRows || bState.HasMalformedRows:
		return alternativeInconclusive("all", ReasonMalformedBenchmark)
	case aState.HasMultipleSeries || bState.HasMultipleSeries:
		return alternativeInconclusive("all", ReasonAmbiguousBenchmark)
	case aState.HasUnsupportedRows || bState.HasUnsupportedRows:
		return alternativeInconclusive("all", ReasonUnsupportedMetric)
	default:
		return nil
	}
}

func evaluateGoTestBench(state rawbench.GoTestBench, opts Options) Report {
	parents := sortedAlternativeParents(state.Samples)
	verdicts := make([]BenchmarkVerdict, 0, len(parents))
	rows := make([]Comparison, 0)

	for _, parent := range parents {
		parentRows, inconclusive := evaluateAlternativeParent(state.Samples, parent, opts)
		if inconclusive != nil {
			verdicts = append(verdicts, *inconclusive)

			continue
		}

		rows = append(rows, parentRows...)
	}

	if len(rows) > 0 {
		verdicts = append(verdicts, evaluate(rows).Verdicts...)
	}

	sort.Slice(verdicts, func(i, j int) bool {
		return verdicts[i].Benchmark < verdicts[j].Benchmark
	})

	return Report{Verdicts: verdicts}
}

func sortedAlternativeParents(samples rawbench.Samples) []string {
	parents := make([]string, 0, len(samples))
	for parent := range samples {
		parents = append(parents, parent)
	}

	sort.Strings(parents)

	return parents
}

func evaluateAlternativeParent(
	samples rawbench.Samples,
	parent string,
	opts Options,
) ([]Comparison, *BenchmarkVerdict) {
	labels := samples[parent]

	baselineLabel, candidateLabel, ok := selectAlternativeLabels(labels, opts)
	if !ok {
		return nil, alternativeInconclusive(parent, ReasonAmbiguousLabels)
	}

	baselineMetrics, hasBaseline := labels[baselineLabel]
	candidateMetrics, hasCandidate := labels[candidateLabel]

	switch {
	case !hasBaseline:
		return nil, alternativeInconclusive(parent, ReasonMissingBaseline)
	case !hasCandidate:
		return nil, alternativeInconclusive(parent, ReasonMissingCandidate)
	}

	rows, ok := compareAlternativeMetrics(parent, baselineLabel, candidateLabel, baselineMetrics, candidateMetrics, opts)
	if !ok {
		return nil, alternativeInconclusive(parent, ReasonInsufficientSamples)
	}

	if len(rows) == 0 {
		return nil, alternativeInconclusive(parent, ReasonUnsupportedMetric)
	}

	return rows, nil
}

func selectAlternativeLabels(labels map[string]map[string][]float64, opts Options) (string, string, bool) {
	if opts.Baseline != "" || opts.Candidate != "" {
		return opts.Baseline, opts.Candidate, true
	}

	if _, hasBaseline := labels[defaultBaseline]; hasBaseline {
		if _, hasCandidate := labels[defaultCandidate]; hasCandidate {
			return defaultBaseline, defaultCandidate, true
		}
	}

	names := make([]string, 0, len(labels))
	for label := range labels {
		names = append(names, label)
	}

	sort.Strings(names)

	if len(names) != requiredAlternativePair {
		return "", "", false
	}

	return names[0], names[1], true
}

func compareAlternativeMetrics(
	parent string,
	baselineLabel string,
	candidateLabel string,
	baselineMetrics map[string][]float64,
	candidateMetrics map[string][]float64,
	opts Options,
) ([]Comparison, bool) {
	metrics := commonMetrics(baselineMetrics, candidateMetrics)
	rows := make([]Comparison, 0, len(metrics))

	for _, metric := range metrics {
		baseline := baselineMetrics[metric]
		candidate := candidateMetrics[metric]

		if len(baseline) < RawComparisonMinSamples || len(candidate) < RawComparisonMinSamples {
			return nil, false
		}

		rows = append(rows, compareAlternativeMetric(
			parent,
			metric,
			baselineLabel,
			candidateLabel,
			baseline,
			candidate,
			opts,
		))
	}

	return rows, true
}

func commonMetrics(left, right map[string][]float64) []string {
	metrics := make([]string, 0, len(left))
	for metric := range left {
		if _, ok := right[metric]; ok {
			metrics = append(metrics, metric)
		}
	}

	sort.Strings(metrics)

	return metrics
}

func compareAlternativeMetric(
	parent string,
	metric string,
	baselineLabel string,
	candidateLabel string,
	baseline []float64,
	candidate []float64,
	opts Options,
) Comparison {
	baselineMean := stat.Mean(baseline)
	candidateMean := stat.Mean(candidate)
	delta := stat.DeltaPercent(baselineMean, candidateMean)
	pValue := stat.PValueApproximation(baseline, candidate, StatisticalMinSamples)
	significant := pValue <= opts.Alpha && math.Abs(delta) >= opts.MinDeltaPct

	return Comparison{
		Benchmark:      parent,
		Metric:         metric,
		DeltaPct:       delta,
		PValue:         pValue,
		Significant:    significant,
		Direction:      classify(metric, delta, significant),
		BaselineLabel:  displayLabel(baselineLabel),
		CandidateLabel: displayLabel(candidateLabel),
	}
}

func alternativeInconclusive(parent, reason string) *BenchmarkVerdict {
	return &BenchmarkVerdict{
		Benchmark:      parent,
		Outcome:        Inconclusive,
		Winner:         "",
		BaselineLabel:  "",
		CandidateLabel: "",
		Metrics:        nil,
		Reason:         "inconclusive alternative input",
		ReasonCode:     reason,
	}
}

func emptyGoTestBenchReport(state rawbench.GoTestBench) Report {
	switch {
	case state.HasMalformedRows:
		return inconclusiveReport(ReasonMalformedBenchmark)
	case state.HasUnsupportedRows:
		return inconclusiveReport(ReasonUnsupportedMetric)
	default:
		return inconclusiveReport(ReasonMalformedBenchmark)
	}
}
