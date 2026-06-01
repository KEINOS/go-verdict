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
	require.Equal(t, 2, verdict.StatisticalMinSamples)
	require.Equal(t, 3, verdict.RawComparisonMinSamples)
	require.Equal(t, 10, verdict.RecommendedRawSamples)
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
