//nolint:exhaustruct // Tests intentionally use partial struct literals.
package verdict

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	benchmarkFoo          = "Foo-8"
	benchstatNewWinsInput = "name          old time/op  new time/op  delta\n" +
		"Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=0.001 n=10+10)\n"
	labelCandidate           = "candidate"
	labelNew                 = "new"
	labelNewTxt              = "new.txt"
	labelOld                 = "old"
	optionAlphaName          = "alpha"
	optionMinDeltaName       = "min-delta"
	rawBadValue              = "bad"
	reasonSame               = "same"
	altMode                  = "alternatives"
	reasonMalformedBenchmark = "malformed-benchmark"
	reasonInsufficient       = "insufficient-samples"
	reasonUnsupported        = "unsupported-metric"
	rawAltInput              = "BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n"
)

var (
	errTestRead         = errors.New("test read error")
	errTestWrite        = errors.New("test write error")
	errTestDelayedWrite = errors.New("test delayed write error")
)

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, errTestRead
}

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errTestWrite
}

func TestParseCSVFormatNewWins(t *testing.T) {
	t.Parallel()

	input := `goos: darwin
goarch: arm64
pkg: example.com/foo
cpu: Apple M4
,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,1.0e-08,1%,8.0e-09,1%,-20.00%,p=0.001 n=10
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	require.Len(t, report.Verdicts, 1,
		"unexpected verdict count, expected exactly one verdict when one benchmark is present")
	require.Equal(t, NewWins, report.Verdicts[0].Outcome,
		"unexpected outcome for new wins with csv format")
	require.Equal(t, labelNewTxt, report.Verdicts[0].Winner,
		"unexpected winner for new wins with csv format")
}

func TestParseTextFormatCapturesBenchstatLabels(t *testing.T) {
	t.Parallel()

	input := `goos: darwin
goarch: arm64
pkg: example.com/foo
cpu: Apple M4
        │ ./testdata/bench_old.txt │     ./testdata/bench_new.txt      │
        │          sec/op          │   sec/op     vs base              │
Foo-8              10.0n ± 1%             8.0n ± 1%  -20.00% (p=0.001 n=10)
`

	report, err := Parse(strings.NewReader(input), Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "bench_old.txt", got.BaselineLabel,
		"unexpected baseline label")
	require.Equal(t, "bench_new.txt", got.CandidateLabel,
		"unexpected candidate label")
	require.Equal(t, "bench_new.txt", got.Winner,
		"winner should match benchstat new label")
}

func TestParseExplicitBenchstatMode(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(benchstatNewWinsInput), Options{Mode: modeBenchstat})
	require.NoError(t, err)

	require.Equal(t, labelNew, report.Verdicts[0].Winner,
		"unexpected winner in explicit benchstat mode")
}

func TestParseTextWithoutPValueReturnsInconclusive(t *testing.T) {
	t.Parallel()

	input := `goos: darwin
goarch: arm64
pkg: example.com/foo
cpu: Apple M4
               │ old.txt │ new.txt │
               │ sec/op  │ sec/op  vs base │
Foo-8          1.00n      1.10n
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	require.Len(t, report.Verdicts, 1,
		"unexpected verdict count, expected exactly one verdict when one benchmark is present")
	require.Equal(t, Inconclusive, report.Verdicts[0].Outcome,
		"unexpected outcome, expected inconclusive when p-value is missing")
	require.Equal(t, "missing-pvalue", report.Verdicts[0].ReasonCode,
		"unexpected reason code, expected 'missing-pvalue' when p-value is missing")
}

func TestParseRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		opts Options
		want string
	}{
		{name: "negative alpha", opts: Options{Alpha: -0.1}, want: optionAlphaName},
		{name: "alpha above one", opts: Options{Alpha: 1.1}, want: optionAlphaName},
		{name: "nan alpha", opts: Options{Alpha: math.NaN()}, want: optionAlphaName},
		{name: "negative min delta", opts: Options{MinDeltaPct: -0.1}, want: optionMinDeltaName},
		{name: "infinite min delta", opts: Options{MinDeltaPct: math.Inf(1)}, want: optionMinDeltaName},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse(strings.NewReader(benchstatNewWinsInput), test.opts)
			require.Error(t, err)
			require.ErrorContains(t, err, test.want,
				"error should contain name of invalid option")
		})
	}
}

func TestParseZeroValueOptionsUseDefaults(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(benchstatNewWinsInput), Options{})
	require.NoError(t, err)

	require.Equal(t, NewWins, report.Verdicts[0].Outcome,
		"unexpected outcome with zero-value options that should use defaults")
}

func TestHigherRateMetricTreatsPositiveDeltaAsImproved(t *testing.T) {
	t.Parallel()

	input := `name          old MB/s  new MB/s  delta
Foo-8         100.0    120.0    +20.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	require.Equal(t, NewWins, report.Verdicts[0].Outcome,
		"unexpected outcome for higher-is-better metric where new is faster than old")
}

func TestMetricsAreSortedDeterministically(t *testing.T) {
	t.Parallel()

	input := `name          old alloc/op  new alloc/op  delta
Foo-8         2.00 ± 0%      1.00 ± 0%   -50.00% (p=0.001 n=10+10)

name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	require.Len(t, report.Verdicts[0].Metrics, 2,
		"expected deterministic metric ordering with two metrics")
	require.Equal(t, "alloc/op", report.Verdicts[0].Metrics[0].Metric,
		"first metric should be alloc/op")
	require.Equal(t, metricSecPerOp, report.Verdicts[0].Metrics[1].Metric,
		"second metric should be sec/op")
}

func TestParseOldBenchstatFormatNewWins(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=0.000 n=10+10)

name          old alloc/op  new alloc/op  delta
Foo-8         2.00 ± 0%     2.00 ± 0%       ~     (p=1.000 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	got := report.Verdicts[0].Outcome
	require.Equal(t, NewWins, got, "unexpected outcome for new wins with old benchstat format")
}

func TestParseNewBenchstatFormatTradeOff(t *testing.T) {
	t.Parallel()

	input := `goos: darwin
goarch: arm64
pkg: example.com/foo
cpu: Apple M4
        │ old.txt │             new.txt             │
        │ sec/op  │   sec/op     vs base            │
Foo-8     10.0n ± 1%   8.0n ± 1%  -20.00% (p=0.000 n=10)
        │ old.txt │             new.txt             │
        │ B/op    │   B/op       vs base            │
Foo-8     16.0 ± 0%   32.0 ± 0% +100.00% (p=0.000 n=10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	got := report.Verdicts[0].Outcome
	require.Equal(t, TradeOff, got,
		"unexpected outcome. One significant improvement and one significant regression should result in trade-off")
}

func TestInsignificantDifferenceIsTie(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   9.9ns ± 1%  -1.00% (p=0.300 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	got := report.Verdicts[0].Outcome
	require.Equal(t, Tie, got, "expected tie when p-value is above alpha threshold")
}

func TestHigherIsBetterMetric(t *testing.T) {
	t.Parallel()

	input := `name          old speed  new speed  delta
Foo-8         100MB/s    120MB/s   +20.00% (p=0.000 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	got := report.Verdicts[0].Outcome
	require.Equal(t, NewWins, got, "unexpected outcome for higher-is-better metric")
}

func TestParseReaderErrorContainsContext(t *testing.T) {
	t.Parallel()

	_, err := Parse(failingReader{}, Options{})
	require.Error(t, err, "expected read error")
	require.ErrorContains(t, err, "reading benchstat input",
		"error should contain context about reading benchstat input")
}

func TestParseTextScannerErrorContainsContext(t *testing.T) {
	t.Parallel()

	longLine := strings.Repeat("x", 70*1024)

	_, err := Parse(strings.NewReader(longLine), Options{})
	require.Error(t, err, "expected scanner error")
	require.ErrorContains(t, err, "scanning benchstat text input",
		"error should contain context about scanning benchstat text input")
}

func TestParseAlternativesScannerErrorContainsContext(t *testing.T) {
	t.Parallel()

	longLine := "BenchmarkEnhance/original-10 100 " + strings.Repeat("1", 70*1024) + " ns/op\n"

	_, err := Parse(strings.NewReader(longLine), Options{Mode: altMode})
	require.Error(t, err, "expected scanner error")
	require.ErrorContains(t, err, "scanning raw alternatives input",
		"error should contain context about scanning raw alternatives input")
}

func TestParseCSVErrorContainsContext(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
"Foo-8,1.0e-08,1%,8.0e-09,1%,-20.00%,0.001
`

	_, err := Parse(strings.NewReader(input), Options{})
	require.Error(t, err, "expected csv parse error")
	require.ErrorContains(t, err, "reading benchstat csv input",
		"error should contain context about reading benchstat csv input")
}

func TestParseEmptyInputReturnsError(t *testing.T) {
	t.Parallel()

	_, err := Parse(strings.NewReader(""), Options{})
	require.ErrorIs(t, err, errNoComparisonRows,
		"expected 'errNoComparisonRows' error when input is empty")
}

func TestParseCSVBenchmarkSetMismatchReturnsInconclusive(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,,1%,,1%,?,?
`

	report, err := Parse(strings.NewReader(input), Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "expected inconclusive outcome")
	require.Equal(t, "benchmark-set-mismatch", got.ReasonCode, "expected benchmark-set-mismatch reason code")
}

func TestParseCSVMissingPValueReturnsInconclusive(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,1.0,1%,0.9,1%,-10.00%,?
`

	report, err := Parse(strings.NewReader(input), Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "expected inconclusive outcome")
	require.Equal(t, "missing-pvalue", got.ReasonCode, "expected missing-pvalue reason code")
}

func TestParseCSVWithOnlyHeaderReturnsError(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
`

	_, err := Parse(strings.NewReader(input), Options{})
	require.ErrorIs(t, err, errNoComparisonRows,
		"expected 'errNoComparisonRows' error when only header rows are present")
}

func TestParseCSVSkipsMalformedRowsAndParsesValidRows(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
BadDelta-8,1.0,1%,0.9,1%,bad,0.001
BadP-8,1.0,1%,0.9,1%,-10.00%,bad
Good-8,1.0,1%,0.9,1%,-10.00%,0.001
`

	report, err := Parse(strings.NewReader(input), Options{})
	require.NoError(t, err)

	require.Len(t, report.Verdicts, 1,
		"expected only one valid verdict to be parsed")
	require.Equal(t, "Good-8", report.Verdicts[0].Benchmark,
		"expected valid row to be parsed with correct benchmark name")
}

func TestParseTextRowsWithoutUsablePValueReturnsError(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=n/a n=10+10)
`

	_, err := Parse(strings.NewReader(input), Options{})
	require.ErrorIs(t, err, errNoComparisonRows,
		"expected 'errNoComparisonRows' error when no comparison rows have usable p-value")
}

func TestParseTextRowsWithoutDeltaReturnsError(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  changed (p=0.001 n=10+10)
`

	_, err := Parse(strings.NewReader(input), Options{})
	require.ErrorIs(t, err, errNoComparisonRows,
		"expected 'errNoComparisonRows' error when no comparison rows have usable delta")
}

func TestMinDeltaThresholdMakesSignificantSmallChangeTie(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   9.9ns ± 1%  -1.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 2})
	require.NoError(t, err)
	require.Equal(t, Tie, report.Verdicts[0].Outcome,
		"expected tie when change is below min delta threshold even if p-value is significant")
}

func TestOldWinsWhenOnlyRegressionExists(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   12.0ns ± 1%  +20.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05})
	require.NoError(t, err)
	require.Equal(t, OldWins, report.Verdicts[0].Outcome,
		"expected old wins when only regression exists")
}

func TestWriteTextIncludesReasonCodeAndAllMetricMarks(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{
			{
				Benchmark:  benchmarkFoo,
				Outcome:    TradeOff,
				Reason:     "mixed result",
				ReasonCode: "example",
				Metrics: []Comparison{
					{Metric: metricSecPerOp, DeltaPct: -10, PValue: 0.001, Direction: Improved},
					{Metric: "B/op", DeltaPct: 20, PValue: 0.001, Direction: Worsened},
					{Metric: "allocs/op", DeltaPct: 0, PValue: 1, Direction: Same},
				},
			},
		},
	}

	var output strings.Builder

	err := report.WriteVerboseText(&output)
	require.NoError(t, err)

	got := output.String()
	for _, want := range []string{"Foo-8: trade-off", "reason_code=example", "+ sec/op", "- B/op", "= allocs/op"} {
		require.Contains(t, got, want, "output should contain %q", want)
	}
}

func TestWriteTextUsesConciseHumanWinner(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{
			{Benchmark: benchmarkFoo, Outcome: NewWins, CandidateLabel: labelNewTxt, Winner: labelNewTxt, Reason: "wins"},
			{Benchmark: "Bar-8", Outcome: Tie, Reason: reasonSame},
		},
	}

	var output strings.Builder

	err := report.WriteText(&output)
	require.NoError(t, err)

	expect := "Foo-8: new.txt wins\nBar-8: tie\n"
	actual := output.String()
	require.Equal(t, expect, actual, "unexpected text report output")
}

func TestWriteTextNoVerdictsWritesNothing(t *testing.T) {
	t.Parallel()

	var output strings.Builder

	err := (Report{}).WriteText(&output)
	require.NoError(t, err)
	require.Empty(t, output.String(), "expected no output when there are no verdicts")
}

func TestWriteTextErrorContainsContext(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	err := report.WriteText(failingWriter{})
	require.Error(t, err, "expected write error")
	require.ErrorContains(t, err, "writing text report",
		"error should contain context about writing text report")
}

func TestWriteTextReasonErrorContainsContext(t *testing.T) {
	t.Parallel()

	writer := failAfterWriter{limit: 2}
	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	err := report.WriteVerboseText(&writer)
	require.Error(t, err,
		"expected reason write error")
	require.ErrorContains(t, err, "writing text report",
		"error should contain context about writing text report")
}

func TestWriteJSONSuccess(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	var output strings.Builder

	err := report.WriteJSON(&output)
	require.NoError(t, err)
	require.Contains(t, output.String(), `"benchmark": "`+benchmarkFoo+`"`,
		"output should contain benchmark name in json")
}

func TestWriteJSONErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := (Report{}).WriteJSON(failingWriter{})
	require.Error(t, err,
		"expected json write error")
	require.ErrorContains(t, err, "writing json report",
		"error should contain context about writing json report")
}

func TestDirectionMarkDefault(t *testing.T) {
	t.Parallel()

	got := directionMark(Direction("unknown"))
	require.Equal(t, "=", got, "unexpected direction mark")
}

func TestPrivateEdgeBranches(t *testing.T) {
	t.Parallel()

	state := newCSVParseState()

	state.handleRecord(nil, Options{})
	require.Empty(t, state.rows,
		"no rows should be added when record is nil")

	state.updateBenchmarkSetMismatch([]string{benchmarkFoo})
	require.False(t, state.hasBenchmarkSetMismatch,
		"benchmark set mismatch should remain false when fields are missing")

	state.metric = metricSecPerOp
	state.pValueIndex = 1

	_, ok := state.parseComparison([]string{benchmarkFoo, "0.001"}, Options{})
	require.False(t, ok,
		"comparison with missing delta index should not parse")

	got := findFieldIndex([]string{"foo"}, "bar")
	require.Equal(t, -1, got,
		"expect indext to be -1")

	for _, rawDelta := range []string{"", "~", "?", rawBadValue} {
		_, ok := parseDeltaPercent(rawDelta)
		require.False(t, ok,
			"invalid delta %q should not parse", rawDelta)
	}

	isCompLine := looksLikeComparisonLine("Foo")
	require.False(t, isCompLine,
		"single-field line should not look like comparison")
}

func TestDecideNoMetricsIsInconclusive(t *testing.T) {
	t.Parallel()

	outcome, reason := decide(0, 0, 0)
	require.Equal(t, Inconclusive, outcome,
		"expected inconclusive outcome when no metrics are present")
	require.NotEmpty(t, reason,
		"expected reason to be non-empty when no metrics are present")
}

func TestWriteTextMetricErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := writeTextMetric(failingWriter{}, Comparison{Direction: Same})
	require.Error(t, err,
		"expected metric write error")
	require.ErrorContains(t, err, "writing text report",
		"error should contain context about writing text report")
}

func TestWriteTextReasonCodeErrorContainsContext(t *testing.T) {
	t.Parallel()

	writer := failAfterWriter{limit: 3}
	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame, ReasonCode: "example"}},
	}

	err := report.WriteVerboseText(&writer)
	require.Error(t, err,
		"expected reason code write error")
	require.ErrorContains(t, err, "writing text report",
		"error should contain context about writing text report")
}

func TestWriteTextMetricErrorFromReportContainsContext(t *testing.T) {
	t.Parallel()

	writer := failAfterWriter{limit: 3}
	report := Report{
		Verdicts: []BenchmarkVerdict{
			{
				Benchmark: benchmarkFoo,
				Outcome:   Tie,
				Reason:    reasonSame,
				Metrics:   []Comparison{{Metric: metricSecPerOp, Direction: Same}},
			},
		},
	}

	err := report.WriteVerboseText(&writer)
	require.Error(t, err,
		"expected metric write error")
	require.ErrorContains(t, err, "writing text report",
		"error should contain context about writing text report")
}

func TestParseAlternativesModeNewWins(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(rawAltInput), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "BenchmarkEnhance", got.Benchmark,
		"expected benchmark name to be inferred as common prefix of benchmark names")
	require.Equal(t, NewWins, got.Outcome,
		"expected outcome to be new-wins based on significant improvement in sec/op")
	require.Len(t, got.Metrics, 3,
		"expected all three metrics to be parsed in alternatives mode")
	require.Equal(t, "enhanced", got.Winner,
		"expected winner to be enhanced based on benchmark names")
}

func TestParseAutoModeRawAlternativesInfersLabels(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(rawAltInput), Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "BenchmarkEnhance", got.Benchmark,
		"expected benchmark name to be inferred as common prefix of benchmark names")
	require.Equal(t, "enhanced", got.Winner,
		"expected winner to be inferred as enhanced based on benchmark names")
}

func TestParseAutoModeRawAlternativesInfersNonDefaultLabels(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/base-10 100 10 ns/op
BenchmarkEnhance/candidate-10 100 8 ns/op
BenchmarkEnhance/base-10 100 10 ns/op
BenchmarkEnhance/candidate-10 100 8 ns/op
BenchmarkEnhance/base-10 100 10 ns/op
BenchmarkEnhance/candidate-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "base", got.BaselineLabel,
		"expected baseline label to be inferred as 'base'")
	require.Equal(t, labelCandidate, got.CandidateLabel,
		"expected candidate label to be inferred as 'candidate'")
	require.Equal(t, labelCandidate, got.Winner,
		"expected winner to be inferred as 'candidate'")
}

func TestParseAutoModeRawAlternativesRejectsAmbiguousLabels(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/a-10 100 10 ns/op
BenchmarkEnhance/b-10 100 8 ns/op
BenchmarkEnhance/c-10 100 8 ns/op
BenchmarkEnhance/a-10 100 10 ns/op
BenchmarkEnhance/b-10 100 8 ns/op
BenchmarkEnhance/c-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome,
		"expected outcome to be inconclusive")
	require.Equal(t, "ambiguous-alternatives", got.ReasonCode,
		"expected reason code to be 'ambiguous-alternatives'")
}

func TestCompareRawFilesDifferentBenchmarkNames(t *testing.T) {
	t.Parallel()

	fast := strings.NewReader(strings.Repeat("BenchmarkExampleFast-10 100 1 ns/op\n", 10))
	slow := strings.NewReader(strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10))

	report, err := CompareRawFiles(fast, slow, Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "BenchmarkExampleFast_vs_BenchmarkExampleSlow", got.Benchmark,
		"expected benchmark name to be combined from both inputs")
	require.Equal(t, "BenchmarkExampleFast", got.Winner,
		"expected winner to be BenchmarkExampleFast based on faster sec/op")
	require.Equal(t, OldWins, got.Outcome,
		"expected outcome to be old wins based on faster sec/op")
}

type rawFileInconclusiveCase struct {
	name   string
	aInput string
	bInput string
	reason string
}

// data provider for inconclusive cases in CompareRawFiles tests.
func rawFileInconclusiveCases() []rawFileInconclusiveCase {
	return []rawFileInconclusiveCase{
		{
			name:   "missing rows",
			aInput: "PASS\n",
			bInput: strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10),
			reason: reasonMalformedBenchmark,
		},
		{
			name:   "malformed row",
			aInput: "BenchmarkExampleFast-10 nope 1 ns/op\n",
			bInput: strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10),
			reason: reasonMalformedBenchmark,
		},
		{
			name: "multiple series",
			aInput: strings.Repeat("BenchmarkExampleFast-10 100 1 ns/op\n", 10) +
				strings.Repeat("BenchmarkOther-10 100 1 ns/op\n", 10),
			bInput: strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10),
			reason: "ambiguous-benchmark",
		},
		{
			name:   "unsupported metric",
			aInput: strings.Repeat("BenchmarkExampleFast-10 100 1 MB/s\n", 10),
			bInput: strings.Repeat("BenchmarkExampleSlow-10 100 10 MB/s\n", 10),
			reason: reasonUnsupported,
		},
		{
			name:   "insufficient samples",
			aInput: "BenchmarkExampleFast-10 100 1 ns/op\n",
			bInput: strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10),
			reason: reasonInsufficient,
		},
		{
			name:   "no common metric",
			aInput: strings.Repeat("BenchmarkExampleFast-10 100 1 ns/op\n", 10),
			bInput: strings.Repeat("BenchmarkExampleSlow-10 100 1 allocs/op\n", 10),
			reason: reasonUnsupported,
		},
	}
}

func TestCompareRawFilesInconclusiveCases(t *testing.T) {
	t.Parallel()

	for _, test := range rawFileInconclusiveCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			report, err := CompareRawFiles(strings.NewReader(test.aInput), strings.NewReader(test.bInput), Options{})
			require.NoError(t, err)

			got := report.Verdicts[0]
			if got.Outcome != Inconclusive || got.ReasonCode != test.reason {
				require.Failf(t, "assertion failed", "verdict = %+v, want %s", got, test.reason)
			}
		})
	}
}

func TestCompareRawFilesSampleBoundaries(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		count       int
		wantReason  string
		wantOutcome Outcome
	}{
		{count: StatisticalMinSamples, wantReason: reasonInsufficient, wantOutcome: Inconclusive},
		{count: RawComparisonMinSamples, wantReason: "", wantOutcome: OldWins},
		{count: 8, wantReason: "", wantOutcome: OldWins},
		{count: RecommendedRawSamples - 1, wantReason: "", wantOutcome: OldWins},
		{count: RecommendedRawSamples, wantReason: "", wantOutcome: OldWins},
	} {
		t.Run(fmt.Sprintf("count_%d", test.count), func(t *testing.T) {
			t.Parallel()

			report, err := CompareRawFiles(
				strings.NewReader(rawFileSamples("BenchmarkExampleFast", 1, test.count)),
				strings.NewReader(rawFileSamples("BenchmarkExampleSlow", 10, test.count)),
				Options{},
			)
			require.NoError(t, err)

			got := report.Verdicts[0]

			require.Equal(t, test.wantOutcome, got.Outcome,
				"expected outcome should match for sample count %d", test.count)
			require.Equal(t, test.wantReason, got.ReasonCode,
				"expected reason code should match for sample count %d", test.count)
		})
	}
}

func rawAlternativeSamples(count int) string {
	var input strings.Builder

	for range count {
		input.WriteString("BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n")
		input.WriteString("BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n")
	}

	return input.String()
}

func rawFileSamples(name string, nsPerOp int, count int) string {
	var input strings.Builder

	for range count {
		fmt.Fprintf(&input, "%s-10 100 %d ns/op\n", name, nsPerOp)
	}

	return input.String()
}

func TestCompareRawFilesReadErrors(t *testing.T) {
	t.Parallel()

	_, err := CompareRawFiles(failingReader{}, strings.NewReader(""), Options{})
	require.Error(t, err, "expected a reader error")

	_, err = CompareRawFiles(strings.NewReader("PASS\n"), failingReader{}, Options{})
	require.Error(t, err, "expected b reader error")

	longLine := strings.NewReader(strings.Repeat("x", 70*1024))

	_, err = CompareRawFiles(longLine, strings.NewReader(""), Options{})
	require.Error(t, err, "expected scanner error")
	require.ErrorContains(t, err, "scanning raw benchmark file input",
		"expected scanner error context in a read error")
}

func TestCompareRawFilesRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := CompareRawFiles(
		strings.NewReader(rawFileSamples("BenchmarkExampleFast", 1, RawComparisonMinSamples)),
		strings.NewReader(rawFileSamples("BenchmarkExampleSlow", 10, RawComparisonMinSamples)),
		Options{Alpha: math.Inf(1)},
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "alpha")
}

func TestParseTextBenchmarkSetMismatchReturnsLabels(t *testing.T) {
	t.Parallel()

	input := `goos: darwin
goarch: arm64
pkg: example.com/foo
cpu: Apple M4
               │ ./fast.txt │ ./slow.txt │
               │   sec/op   │ sec/op vs base │
Fast-10               1.0n
Slow-10                         10.0n
geomean               1.0n      10.0n ? ¹ ²
¹ benchmark set differs from baseline; geomeans may not be comparable
`

	report, err := Parse(strings.NewReader(input), Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "benchmark-set-mismatch", got.ReasonCode,
		"expected reason code to indicate benchmark set mismatch")
	require.Equal(t, "./fast.txt", got.BaselineLabel,
		"unexpected baseline label")
	require.Equal(t, "./slow.txt", got.CandidateLabel,
		"unexpected candidate label")
}

func TestPrivateRawFileBenchmarkLineBranches(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"BenchmarkExampleFast-10 100",
		"BenchmarkExampleFast-10 nope 1 ns/op",
		"BenchmarkExampleFast-10 100 1",
		"BenchmarkExampleFast-10 100 1 ns/op 2",
		"-10 100 1 ns/op",
	} {
		_, _, ok := parseRawFileBenchmarkLine(line)
		require.False(t, ok, "line '%q' should not parse", line)
	}
}

func TestParseAlternativesModeNestedNameAndCustomLabels(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/group/base-10 100 12 ns/op
BenchmarkEnhance/group/candidate-10 100 10 ns/op
BenchmarkEnhance/group/base-10 100 12 ns/op
BenchmarkEnhance/group/candidate-10 100 10 ns/op
BenchmarkEnhance/group/base-10 100 12 ns/op
BenchmarkEnhance/group/candidate-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{
		Alpha:       0,
		MinDeltaPct: 0,
		Mode:        altMode,
		Baseline:    "base",
		Candidate:   labelCandidate,
	})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "BenchmarkEnhance/group", got.Benchmark,
		"unexpected benchmark name")
	require.Equal(t, NewWins, got.Outcome,
		"unexpected outcome")
}

func TestParseAlternativesModeTradeOff(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op 8 B/op
BenchmarkEnhance/enhanced-10 100 8 ns/op 16 B/op
BenchmarkEnhance/original-10 100 10 ns/op 8 B/op
BenchmarkEnhance/enhanced-10 100 8 ns/op 16 B/op
BenchmarkEnhance/original-10 100 10 ns/op 8 B/op
BenchmarkEnhance/enhanced-10 100 8 ns/op 16 B/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	require.Equal(t, TradeOff, report.Verdicts[0].Outcome, "unexpected outcome")
}

func TestParseAlternativesModeTie(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	require.Equal(t, Tie, report.Verdicts[0].Outcome, "unexpected outcome")
}

func TestParseAlternativesModeOldWins(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	require.Equal(t, OldWins, report.Verdicts[0].Outcome, "unexpected outcome")
}

func TestParseAlternativesModeMissingBaseline(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/enhanced-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, "missing-baseline", got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesModeMissingCandidate(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/original-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, "missing-candidate", got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesModeInsufficientSamples(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 7 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonInsufficient, got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesRawSampleBoundaries(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		count       int
		wantReason  string
		wantOutcome Outcome
	}{
		{count: StatisticalMinSamples, wantReason: reasonInsufficient, wantOutcome: Inconclusive},
		{count: RawComparisonMinSamples, wantReason: "", wantOutcome: NewWins},
		{count: 8, wantReason: "", wantOutcome: NewWins},
		{count: RecommendedRawSamples - 1, wantReason: "", wantOutcome: NewWins},
		{count: RecommendedRawSamples, wantReason: "", wantOutcome: NewWins},
	} {
		t.Run(fmt.Sprintf("count_%d", test.count), func(t *testing.T) {
			t.Parallel()

			report, err := Parse(strings.NewReader(rawAlternativeSamples(test.count)), Options{Mode: altMode})
			require.NoError(t, err)

			got := report.Verdicts[0]

			require.Equal(t, test.wantOutcome, got.Outcome, "unexpected outcome")
			require.Equal(t, test.wantReason, got.ReasonCode, "unexpected reason code")
		})
	}
}

func TestParseAlternativesModeUnsupportedMetric(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 MB/s
BenchmarkEnhance/enhanced-10 100 9 MB/s
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, "unsupported-metric", got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesModeNoCommonMetric(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 1 allocs/op
BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 1 allocs/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, "unsupported-metric", got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesModeMalformedBenchmark(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance 100 8 ns/op\n"), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesModeMalformedShortRow(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance/original-10 100\n"), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesModeMalformedIteration(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance/original-10 nope 8 ns/op\n"), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesModeMalformedSupportedMetricValue(t *testing.T) {
	t.Parallel()

	report, err := Parse(
		strings.NewReader("BenchmarkEnhance/original-10 100 nope ns/op\n"),
		Options{Mode: altMode},
	)
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestCompareRawFilesMalformedSupportedMetricValue(t *testing.T) {
	t.Parallel()

	report, err := CompareRawFiles(
		strings.NewReader("BenchmarkExampleFast-10 100 nope ns/op\n"),
		strings.NewReader(strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10)),
		Options{},
	)
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesModeNoBenchmarkRows(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("PASS\n"), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestParseAlternativesModeSkipsUnrequestedLabels(t *testing.T) {
	t.Parallel()

	input := rawAltInput + "BenchmarkEnhance/control-10 100 1 ns/op\n"

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)
	require.Equal(t, NewWins, report.Verdicts[0].Outcome, "unexpected outcome")
}

func TestParseAlternativesModeSortsMixedVerdicts(t *testing.T) {
	t.Parallel()

	input := `BenchmarkZ/original-10 100 10 ns/op
BenchmarkZ/enhanced-10 100 8 ns/op
BenchmarkZ/original-10 100 10 ns/op
BenchmarkZ/enhanced-10 100 8 ns/op
BenchmarkZ/original-10 100 10 ns/op
BenchmarkZ/enhanced-10 100 8 ns/op
BenchmarkA/original-10 100 10 ns/op
BenchmarkA/original-10 100 10 ns/op
BenchmarkA/original-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	require.Equal(t, "BenchmarkA", report.Verdicts[0].Benchmark,
		"expected BenchmarkA to be sorted before BenchmarkZ")
	require.Equal(t, "BenchmarkZ", report.Verdicts[1].Benchmark,
		"expected BenchmarkZ to be sorted after BenchmarkA")
}

func TestParseAlternativesModeVariableSamplesUsePValueApproximation(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
BenchmarkEnhance/original-10 100 12 ns/op
BenchmarkEnhance/enhanced-10 100 9 ns/op
BenchmarkEnhance/original-10 100 11 ns/op
BenchmarkEnhance/enhanced-10 100 8.5 ns/op
`
	report, err := Parse(strings.NewReader(input), Options{Mode: altMode, Alpha: 1})
	require.NoError(t, err)

	got := report.Verdicts[0].Metrics[0].PValue
	if got <= 0 || got >= 1 {
		t.Fatalf("p-value = %f, want normal approximation between 0 and 1", got)
	}
}

func TestPrivateAlternativeBranches(t *testing.T) {
	t.Parallel()

	got := trimCPUSuffix("BenchmarkFoo/original")
	require.Equal(t, "BenchmarkFoo/original", got,
		"benchmark name without numeric suffix should not be trimmed")

	got = trimCPUSuffix("BenchmarkFoo/original-fast")
	require.Equal(t, "BenchmarkFoo/original-fast", got,
		"benchmark name with non-numeric suffix should not be trimmed")

	_, _, ok := splitRawBenchmarkName("BenchmarkFoo-10")
	require.False(t, ok, "benchmark name without sub-benchmark should not split")

	metric, ok := normalizeRawMetric("MB/s")
	require.False(t, ok, "unsupported metric should not parse")
	require.Empty(t, metric, "unsupported metric should return empty")
}

func TestPrivateAlternativeMathBranches(t *testing.T) {
	t.Parallel()

	metrics, ok := parseRawMetrics([]string{rawBadValue, "MB/s", "10", metricNanosecondsPerOp})
	require.True(t, ok, "unsupported bad metric value should not make the row malformed")
	require.InDelta(t, 10.0, metrics[metricSecPerOp], 1e-9, "valid metric should be parsed")

	_, ok = parseRawMetrics([]string{rawBadValue, metricNanosecondsPerOp})
	require.False(t, ok, "bad metric value should not parse")

	got := variance([]float64{1}, 1)
	require.InDelta(t, 0.0, got, 1e-9, "variance of one sample should be zero")

	got = deltaPercent(0, 10)
	require.InDelta(t, 0.0, got, 1e-9, "delta percent with zero baseline should be zero")
}

func TestPrivateAlternativeEmptyReportBranches(t *testing.T) {
	t.Parallel()

	insufficientState := alternativeParseState{hasInsufficientRows: true}

	got := insufficientState.emptyAlternativeReport()
	require.Equal(t, reasonInsufficient, got.Verdicts[0].ReasonCode,
		"report reason code should indicate insufficient samples when state has insufficient rows")

	emptyState := alternativeParseState{}

	got = emptyState.emptyAlternativeReport()
	require.Equal(t, reasonMalformedBenchmark, got.Verdicts[0].ReasonCode,
		"report reason code should indicate malformed benchmark when state is empty")
}

func TestPrivateTextLabelBranches(t *testing.T) {
	t.Parallel()

	textState := textParseState{baselineLabel: "already", candidateLabel: "set"}
	textState.captureLabels("│ old.txt │ new.txt │")
	require.Equal(t, "already", textState.baselineLabel,
		"existing baseline label should not be overwritten")
	require.Equal(t, "set", textState.candidateLabel,
		"existing candidate label should not be overwritten")

	emptyTextState := textParseState{}
	emptyTextState.captureLabels("│ sec/op │ sec/op vs base │")
	require.Empty(t, emptyTextState.baselineLabel,
		"metric header should not set baseline label")
	require.Empty(t, emptyTextState.candidateLabel,
		"metric header should not set candidate label")

	_, ok := parseBenchstatTextLabels("│ sec/op │ sec/op vs base │")
	require.False(t, ok,
		"metric header should not parse as labels")

	got := emptyTextState.rawBaselineLabel()
	require.Equal(t, labelOld, got,
		"raw baseline fallback should be old when baseline label is empty")

	got = emptyTextState.rawCandidateLabel()
	require.Equal(t, labelNew, got,
		"raw candidate fallback should be new when candidate label is empty")
}

func TestPrivateCSVLabelBranches(t *testing.T) {
	t.Parallel()

	csvState := csvParseState{}
	csvState.captureLabels([]string{"", "sec/op", "CI", "sec/op", "CI", "vs base", "P"})
	csvState.captureLabels([]string{"", "", "", labelNewTxt})

	require.Empty(t, csvState.baselineLabel,
		"csv header rows should not set baseline label")
	require.Empty(t, csvState.candidateLabel,
		"csv header rows should not set candidate label")
	require.Equal(t, labelOld, csvState.displayBaselineLabel(),
		"display baseline fallback should be old")
	require.Equal(t, labelNew, csvState.displayCandidateLabel(),
		"display candidate fallback should be new")
	require.Equal(t, labelOld, csvState.rawBaselineLabel(),
		"raw baseline fallback should be old")
	require.Equal(t, labelNew, csvState.rawCandidateLabel(),
		"raw candidate fallback should be new")
}

func TestPrivateDisplayLabelBranches(t *testing.T) {
	t.Parallel()

	require.Empty(t, displayLabel(""), "empty label should display as empty")
	require.Equal(t, ".", displayLabel("."), "dot label should display as dot")

	baselineLabel, candidateLabel := comparisonLabels([]Comparison{{}})
	require.Equal(t, labelOld, baselineLabel, "blank comparison baseline label should be old")
	require.Equal(t, labelNew, candidateLabel, "blank comparison candidate label should be new")

	baselineLabel, candidateLabel = comparisonLabels(nil)
	require.Equal(t, labelOld, baselineLabel, "nil comparison baseline label should be old")
	require.Equal(t, labelNew, candidateLabel, "nil comparison candidate label should be new")

	require.Empty(t, winnerLabel(Outcome("unknown"), labelOld, labelNew),
		"unknown outcome winner should be empty")
}

func TestWriteVerboseTextHeaderErrorContainsContext(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	err := report.WriteVerboseText(failingWriter{})
	require.Error(t, err,
		"expected header write error")
	require.ErrorContains(t, err, "writing text report",
		"expected header write error context")
}

// ----------------------------------------------------------------------------
//  Test helpers
// ----------------------------------------------------------------------------

type failAfterWriter struct {
	limit  int
	writes int
}

func (writer *failAfterWriter) Write(data []byte) (int, error) {
	writer.writes++
	if writer.writes >= writer.limit {
		return 0, errTestDelayedWrite
	}

	return len(data), nil
}

// assert that failAfterWriter implements io.Writer.
var _ io.Writer = (*failAfterWriter)(nil)
