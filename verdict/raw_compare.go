package verdict

import (
	"io"
	"math"
	"sort"
)

func compareRawFiles(aReader io.Reader, bReader io.Reader, opts Options) (Report, error) {
	aState, err := parseRawFile(aReader)
	if err != nil {
		return Report{}, err
	}

	bState, err := parseRawFile(bReader)
	if err != nil {
		return Report{}, err
	}

	if inconclusive := rawFileInconclusive(aState, bState); inconclusive != nil {
		return Report{Verdicts: []BenchmarkVerdict{*inconclusive}}, nil
	}

	// Raw-file comparison treats two separate benchmark series as explicit A/B
	// alternatives, unlike raw stdin mode where labels come from sub-benchmarks.
	benchmark := aState.name + "_vs_" + bState.name

	rows, ok := compareAlternativeMetrics(
		benchmark,
		aState.name,
		bState.name,
		aState.metrics,
		bState.metrics,
		opts,
	)
	if !ok {
		return Report{Verdicts: []BenchmarkVerdict{*alternativeInconclusive(benchmark, "insufficient-samples")}}, nil
	}

	if len(rows) == 0 {
		return Report{Verdicts: []BenchmarkVerdict{*alternativeInconclusive(benchmark, "unsupported-metric")}}, nil
	}

	return evaluate(rows), nil
}

func (state *alternativeParseState) evaluate(opts Options) Report {
	parents := sortedAlternativeParents(state.samples)
	verdicts := make([]BenchmarkVerdict, 0, len(parents))
	rows := make([]Comparison, 0)

	for _, parent := range parents {
		parentRows, inconclusive := state.evaluateParent(parent, opts)
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

func sortedAlternativeParents(samples alternativeSampleSet) []string {
	parents := make([]string, 0, len(samples))
	for parent := range samples {
		parents = append(parents, parent)
	}

	sort.Strings(parents)

	return parents
}

func (state *alternativeParseState) evaluateParent(parent string, opts Options) ([]Comparison, *BenchmarkVerdict) {
	labels := state.samples[parent]

	baselineLabel, candidateLabel, ok := selectAlternativeLabels(labels, opts)
	if !ok {
		return nil, alternativeInconclusive(parent, "ambiguous-alternatives")
	}

	baselineMetrics, hasBaseline := labels[baselineLabel]
	candidateMetrics, hasCandidate := labels[candidateLabel]

	switch {
	case !hasBaseline:
		return nil, alternativeInconclusive(parent, "missing-baseline")
	case !hasCandidate:
		return nil, alternativeInconclusive(parent, "missing-candidate")
	}

	rows, ok := compareAlternativeMetrics(parent, baselineLabel, candidateLabel, baselineMetrics, candidateMetrics, opts)
	if !ok {
		state.hasInsufficientRows = true

		return nil, alternativeInconclusive(parent, "insufficient-samples")
	}

	if len(rows) == 0 {
		return nil, alternativeInconclusive(parent, "unsupported-metric")
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
	baselineMean := mean(baseline)
	candidateMean := mean(candidate)
	delta := deltaPercent(baselineMean, candidateMean)
	pValue := pValueApproximation(baseline, candidate)
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

func (state *alternativeParseState) emptyAlternativeReport() Report {
	switch {
	case state.hasMalformedRows:
		return inconclusiveReport("malformed-benchmark")
	case state.hasUnsupportedRows:
		return inconclusiveReport("unsupported-metric")
	case state.hasInsufficientRows:
		return inconclusiveReport("insufficient-samples")
	default:
		return inconclusiveReport("malformed-benchmark")
	}
}
