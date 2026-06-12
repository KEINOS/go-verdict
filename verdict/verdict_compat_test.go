package verdict_test

import (
	"io"
	"testing"

	"github.com/KEINOS/go-verdict/verdict"
	"github.com/stretchr/testify/require"
)

func TestCompatibilityV1_0_0(t *testing.T) {
	t.Parallel()

	compileV100Functions()
	compileV100Options()
	compileV100ReportTypes()

	require.Equal(t, verdict.Improved, verdict.Direction("improved"))
	require.Equal(t, verdict.Worsened, verdict.Direction("worsened"))
	require.Equal(t, verdict.Same, verdict.Direction("same"))
	require.Equal(t, verdict.NewWins, verdict.Outcome("new-wins"))
	require.Equal(t, verdict.OldWins, verdict.Outcome("old-wins"))
	require.Equal(t, verdict.Tie, verdict.Outcome("tie"))
	require.Equal(t, verdict.TradeOff, verdict.Outcome("trade-off"))
	require.Equal(t, verdict.Inconclusive, verdict.Outcome("inconclusive"))
	require.Equal(t, "auto", verdict.ModeAuto)
	require.Equal(t, "benchstat", verdict.ModeBenchstat)
	require.Equal(t, "gotestbench", verdict.ModeGoTestBench)
	require.Equal(t, "missing-pvalue", verdict.ReasonMissingPValue)
	require.Equal(t, "benchmark-set-mismatch", verdict.ReasonBenchmarkSetMismatch)
	require.Equal(t, "missing-baseline", verdict.ReasonMissingBaseline)
	require.Equal(t, "missing-candidate", verdict.ReasonMissingCandidate)
	require.Equal(t, "insufficient-samples", verdict.ReasonInsufficientSamples)
	require.Equal(t, "unsupported-metric", verdict.ReasonUnsupportedMetric)
	require.Equal(t, "malformed-benchmark", verdict.ReasonMalformedBenchmark)
	require.Equal(t, "ambiguous-labels", verdict.ReasonAmbiguousLabels)
	require.Equal(t, "ambiguous-benchmark", verdict.ReasonAmbiguousBenchmark)
	require.Equal(t, 2, verdict.StatisticalMinSamples)
	require.Equal(t, 3, verdict.RawComparisonMinSamples)
	require.Equal(t, 10, verdict.RecommendedRawSamples)
	require.InDelta(t, 2.0, verdict.DefaultMinDeltaPct, 0.001)
}

func compileV100Functions() {
	acceptParse := func(_ func(io.Reader, verdict.Options) (verdict.Report, error)) {}
	acceptCompareRawFiles := func(_ func(io.Reader, io.Reader, verdict.Options) (verdict.Report, error)) {}
	acceptNewOptions := func(_ func() verdict.Options) {}
	acceptReportWriter := func(_ func(verdict.Report, io.Writer) error) {}

	acceptParse(verdict.Parse)
	acceptCompareRawFiles(verdict.CompareRawFiles)
	acceptNewOptions(verdict.NewOptions)
	acceptReportWriter(verdict.Report.WriteText)
	acceptReportWriter(verdict.Report.WriteVerboseText)
	acceptReportWriter(verdict.Report.WriteJSON)
}

func compileV100Options() {
	options := verdict.Options{
		Alpha:       0,
		MinDeltaPct: 0,
		Mode:        "",
		Baseline:    "",
		Candidate:   "",
	}

	acceptFloat64 := func(_ float64) {}
	acceptString := func(_ string) {}

	acceptFloat64(options.Alpha)
	acceptFloat64(options.MinDeltaPct)
	acceptString(options.Mode)
	acceptString(options.Baseline)
	acceptString(options.Candidate)
	acceptString(verdict.ModeAuto)
	acceptString(verdict.ModeBenchstat)
	acceptString(verdict.ModeGoTestBench)
	acceptString(verdict.ReasonMissingPValue)
	acceptString(verdict.ReasonBenchmarkSetMismatch)
	acceptString(verdict.ReasonMissingBaseline)
	acceptString(verdict.ReasonMissingCandidate)
	acceptString(verdict.ReasonInsufficientSamples)
	acceptString(verdict.ReasonUnsupportedMetric)
	acceptString(verdict.ReasonMalformedBenchmark)
	acceptString(verdict.ReasonAmbiguousLabels)
	acceptString(verdict.ReasonAmbiguousBenchmark)
}

func compileV100ReportTypes() {
	comparison := verdict.Comparison{
		Benchmark:      "",
		Metric:         "",
		DeltaPct:       0,
		PValue:         0,
		Significant:    false,
		Direction:      "",
		BaselineLabel:  "",
		CandidateLabel: "",
	}
	benchmarkVerdict := verdict.BenchmarkVerdict{
		Benchmark:      "",
		Outcome:        "",
		Winner:         "",
		BaselineLabel:  "",
		CandidateLabel: "",
		Metrics:        nil,
		Reason:         "",
		ReasonCode:     "",
	}
	report := verdict.Report{
		Verdicts: nil,
	}

	acceptFloat64 := func(_ float64) {}
	acceptString := func(_ string) {}
	acceptBool := func(_ bool) {}
	acceptDirection := func(_ verdict.Direction) {}
	acceptOutcome := func(_ verdict.Outcome) {}
	acceptComparisons := func(_ []verdict.Comparison) {}
	acceptBenchmarkVerdicts := func(_ []verdict.BenchmarkVerdict) {}

	acceptString(comparison.Benchmark)
	acceptString(comparison.Metric)
	acceptFloat64(comparison.DeltaPct)
	acceptFloat64(comparison.PValue)
	acceptBool(comparison.Significant)
	acceptDirection(comparison.Direction)
	acceptString(comparison.BaselineLabel)
	acceptString(comparison.CandidateLabel)
	acceptString(benchmarkVerdict.Benchmark)
	acceptOutcome(benchmarkVerdict.Outcome)
	acceptString(benchmarkVerdict.Winner)
	acceptString(benchmarkVerdict.BaselineLabel)
	acceptString(benchmarkVerdict.CandidateLabel)
	acceptComparisons(benchmarkVerdict.Metrics)
	acceptString(benchmarkVerdict.Reason)
	acceptString(benchmarkVerdict.ReasonCode)
	acceptBenchmarkVerdicts(report.Verdicts)
}
