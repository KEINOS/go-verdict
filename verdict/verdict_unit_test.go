//nolint:exhaustruct // Tests intentionally use partial struct literals.
package verdict

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser"
	"github.com/KEINOS/go-verdict/verdict/internal/benchparser/rawbench"
	"github.com/KEINOS/go-verdict/verdict/internal/pareto"
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
	reasonMixed              = "mixed result"
	reasonSame               = "same"
	reasonMalformedBenchmark = "malformed-benchmark"
	reasonInsufficient       = "insufficient-samples"
	reasonUnsupported        = "unsupported-metric"
	rawGoTestBenchInput      = "BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
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

	report, err := Parse(strings.NewReader(benchstatNewWinsInput), Options{Mode: ModeBenchstat})
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

func TestParseModeValidation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		mode    string
		wantErr bool
	}{
		{name: "empty mode uses auto"},
		{name: "auto mode", mode: ModeAuto},
		{name: "benchstat mode", mode: ModeBenchstat},
		{name: "go test bench mode", mode: ModeGoTestBench},
		{name: "removed alternatives mode", mode: "alternatives", wantErr: true},
		{name: "uppercase mode", mode: "GOTESTBENCH", wantErr: true},
		{name: "whitespace-only mode", mode: " ", wantErr: true},
		{name: "unknown mode", mode: "sideways", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse(strings.NewReader(benchstatNewWinsInput), Options{Mode: test.mode})
			if test.wantErr {
				require.ErrorContains(t, err, "invalid options: unknown mode",
					"unexpected invalid mode error")

				return
			}

			require.NoError(t, err)
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

func TestParseGoTestBenchScannerErrorContainsContext(t *testing.T) {
	t.Parallel()

	longLine := "BenchmarkEnhance/original-10 100 " + strings.Repeat("1", 70*1024) + " ns/op\n"

	_, err := Parse(strings.NewReader(longLine), Options{Mode: ModeGoTestBench})
	require.Error(t, err, "expected scanner error")
	require.ErrorContains(t, err, "scanning raw go test -bench input",
		"error should contain context about scanning raw go test -bench input")
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
				Reason:     reasonMixed,
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

func TestVerboseTextMetricRowAlignment(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{
			{
				Benchmark: benchmarkFoo,
				Outcome:   TradeOff,
				Reason:    reasonMixed,
				Metrics: []Comparison{
					{Metric: "MB/s", DeltaPct: 15.79, PValue: 0.0000123, Direction: Improved},
					{Metric: metricSecPerOp, DeltaPct: -20, PValue: 0.001, Direction: Improved},
					{Metric: "allocs/op", DeltaPct: 100, PValue: 1, Direction: Worsened},
					{Metric: "gc_count/op", DeltaPct: 50, PValue: 0.25, Direction: Worsened},
				},
			},
		},
	}

	var output strings.Builder

	err := report.WriteVerboseText(&output)
	require.NoError(t, err)

	expected := strings.Join([]string{
		"Foo-8: trade-off",
		"  mixed result",
		"  + MB/s           15.79% p=    1.23e-05 improved",
		"  + sec/op        -20.00% p=       0.001 improved",
		"  - allocs/op     100.00% p=           1 worsened",
		"  - gc_count/op    50.00% p=        0.25 worsened",
		"",
	}, "\n")
	require.Equal(t, expected, output.String(),
		"verbose metric rows should align labels, deltas, and p-values")
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

func TestWriteTextConciseOutputUnchanged(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{
			{Benchmark: benchmarkFoo, Outcome: NewWins, CandidateLabel: labelNewTxt, Winner: labelNewTxt, Reason: "wins"},
			{Benchmark: "Bar-8", Outcome: Tie, Reason: reasonSame},
			{Benchmark: "Baz-8", Outcome: TradeOff, Reason: reasonMixed},
		},
	}

	var output strings.Builder

	err := report.WriteText(&output)
	require.NoError(t, err)

	expected := "Foo-8: new.txt wins\nBar-8: tie\nBaz-8: trade-off\n"
	require.Equal(t, expected, output.String(),
		"concise text output must not change during verbose alignment work")
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

func TestWriteJSONSemanticOutputUnchanged(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{
			{
				Benchmark:      benchmarkFoo,
				Outcome:        NewWins,
				Winner:         labelCandidate,
				BaselineLabel:  labelOld,
				CandidateLabel: labelCandidate,
				Reason:         "candidate dominates",
				Metrics: []Comparison{
					{Metric: metricSecPerOp, DeltaPct: -20, PValue: 0.001, Direction: Improved},
				},
			},
		},
	}

	var output strings.Builder

	err := report.WriteJSON(&output)
	require.NoError(t, err)

	var got Report

	err = json.Unmarshal([]byte(output.String()), &got)
	require.NoError(t, err)
	require.Equal(t, report, got,
		"JSON report semantics must not change during verbose alignment work")
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

func TestDecideNoMetricsIsInconclusive(t *testing.T) {
	t.Parallel()

	outcome, reason := decide(pareto.Inconclusive)
	require.Equal(t, Inconclusive, outcome,
		"expected inconclusive outcome when no metrics are present")
	require.NotEmpty(t, reason,
		"expected reason to be non-empty when no metrics are present")
}

func TestMetricRelationUnknownDirectionIsSame(t *testing.T) {
	t.Parallel()

	require.Equal(t, pareto.Same, metricRelation(Direction("unknown")),
		"unknown public direction should not count as an improvement or regression")
}

func TestDecideUnknownRelationIsInconclusive(t *testing.T) {
	t.Parallel()

	outcome, reason := decide(pareto.Relation("unknown"))
	require.Equal(t, Inconclusive, outcome,
		"unknown internal relation should produce an inconclusive public outcome")
	require.NotEmpty(t, reason,
		"expected reason to be non-empty when internal relation is unknown")
}

func TestWriteTextMetricErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := writeTextMetric(failingWriter{}, Comparison{Direction: Same}, 0)
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

func TestParseGoTestBenchModeNewWins(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(rawGoTestBenchInput), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "BenchmarkEnhance", got.Benchmark,
		"expected benchmark name to be inferred as common prefix of benchmark names")
	require.Equal(t, NewWins, got.Outcome,
		"expected outcome to be new-wins based on significant improvement in sec/op")
	require.Len(t, got.Metrics, 3,
		"expected all three metrics to be parsed in gotestbench mode")
	require.Equal(t, "enhanced", got.Winner,
		"expected winner to be enhanced based on benchmark names")
}

func TestParseAutoModeRawGoTestBenchInfersLabels(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(rawGoTestBenchInput), Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "BenchmarkEnhance", got.Benchmark,
		"expected benchmark name to be inferred as common prefix of benchmark names")
	require.Equal(t, "enhanced", got.Winner,
		"expected winner to be inferred as enhanced based on benchmark names")
}

func TestParseAutoModeRawGoTestBenchInfersNonDefaultLabels(t *testing.T) {
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

func TestParseNewOptionsPreservesAutoModeRawAlternativeInference(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/base-10 100 10 ns/op
BenchmarkEnhance/candidate-10 100 8 ns/op
BenchmarkEnhance/base-10 100 10 ns/op
BenchmarkEnhance/candidate-10 100 8 ns/op
BenchmarkEnhance/base-10 100 10 ns/op
BenchmarkEnhance/candidate-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), NewOptions())
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, "base", got.BaselineLabel,
		"unexpected baseline label")
	require.Equal(t, labelCandidate, got.CandidateLabel,
		"unexpected candidate label")
	require.Equal(t, labelCandidate, got.Winner,
		"unexpected winner")
}

func TestParseAutoModeRawGoTestBenchRejectsAmbiguousLabels(t *testing.T) {
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
	require.Equal(t, "ambiguous-labels", got.ReasonCode,
		"expected reason code to be 'ambiguous-labels'")
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
			require.Equal(t, Inconclusive, got.Outcome,
				"unexpected outcome")
			require.Equal(t, test.reason, got.ReasonCode,
				"unexpected reason code")
		})
	}
}

func TestRawInconclusiveReasonCodeContracts(t *testing.T) {
	t.Parallel()

	for _, test := range rawInconclusiveReasonCodeContractCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			report := test.run(t)

			require.Len(t, report.Verdicts, 1, "unexpected verdict count")
			require.Equal(t, Inconclusive, report.Verdicts[0].Outcome, "unexpected outcome")
			require.Equal(t, test.reason, report.Verdicts[0].ReasonCode, "unexpected reason code")
		})
	}
}

type rawReasonCodeContractCase struct {
	name   string
	reason string
	run    func(t *testing.T) Report
}

func rawInconclusiveReasonCodeContractCases() []rawReasonCodeContractCase {
	return []rawReasonCodeContractCase{
		rawParseReasonCodeCase(
			"malformed benchmark",
			reasonMalformedBenchmark,
			"BenchmarkEnhance/original-10 nope 8 ns/op\n",
			Options{Mode: ModeGoTestBench},
		),
		rawParseReasonCodeCase(
			"unsupported metric",
			reasonUnsupported,
			"BenchmarkEnhance/original-10 100 8 MB/s\n",
			Options{Mode: ModeGoTestBench},
		),
		rawParseReasonCodeCase(
			"insufficient samples",
			reasonInsufficient,
			"BenchmarkEnhance/original-10 100 8 ns/op\nBenchmarkEnhance/enhanced-10 100 7 ns/op\n",
			Options{Mode: ModeGoTestBench},
		),
		rawParseReasonCodeCase(
			"ambiguous labels",
			"ambiguous-labels",
			"BenchmarkEnhance/a-10 100 10 ns/op\nBenchmarkEnhance/b-10 100 8 ns/op\nBenchmarkEnhance/c-10 100 8 ns/op\n",
			Options{},
		),
		rawParseReasonCodeCase(
			"missing baseline",
			"missing-baseline",
			"BenchmarkEnhance/enhanced-10 100 8 ns/op\n",
			Options{Mode: ModeGoTestBench},
		),
		rawParseReasonCodeCase(
			"missing candidate",
			"missing-candidate",
			"BenchmarkEnhance/original-10 100 8 ns/op\n",
			Options{Mode: ModeGoTestBench},
		),
		rawFileReasonCodeCase("ambiguous benchmark", "ambiguous-benchmark"),
	}
}

func rawParseReasonCodeCase(name string, reason string, input string, opts Options) rawReasonCodeContractCase {
	return rawReasonCodeContractCase{
		name:   name,
		reason: reason,
		run: func(t *testing.T) Report {
			t.Helper()

			report, err := Parse(strings.NewReader(input), opts)
			require.NoError(t, err)

			return report
		},
	}
}

func rawFileReasonCodeCase(name string, reason string) rawReasonCodeContractCase {
	return rawReasonCodeContractCase{
		name:   name,
		reason: reason,
		run: func(t *testing.T) Report {
			t.Helper()

			report, err := CompareRawFiles(
				strings.NewReader("BenchmarkExampleFast-10 100 1 ns/op\nBenchmarkOther-10 100 1 ns/op\n"),
				strings.NewReader("BenchmarkExampleSlow-10 100 10 ns/op\n"),
				Options{},
			)
			require.NoError(t, err)

			return report
		},
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

func rawGoTestBenchSamples(count int) string {
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
	require.ErrorContains(t, err, "alpha",
		"error should mention invalid alpha")
}

func TestCompareRawFilesIgnoresModeAndLabelOptions(t *testing.T) {
	t.Parallel()

	aInput := rawFileSamples("BenchmarkExampleFast", 1, RawComparisonMinSamples)
	bInput := rawFileSamples("BenchmarkExampleSlow", 10, RawComparisonMinSamples)

	want, err := CompareRawFiles(strings.NewReader(aInput), strings.NewReader(bInput), Options{})
	require.NoError(t, err)

	got, err := CompareRawFiles(
		strings.NewReader(aInput),
		strings.NewReader(bInput),
		Options{Mode: "sideways", Baseline: "ignored-baseline", Candidate: "ignored-candidate"},
	)
	require.NoError(t, err)
	require.Equal(t, want, got,
		"raw-file comparison should ignore mode and label options")
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

func TestParseGoTestBenchModeNestedNameAndCustomLabels(t *testing.T) {
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
		Mode:        ModeGoTestBench,
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

func TestParseGoTestBenchModeTradeOff(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op 8 B/op
BenchmarkEnhance/enhanced-10 100 8 ns/op 16 B/op
BenchmarkEnhance/original-10 100 10 ns/op 8 B/op
BenchmarkEnhance/enhanced-10 100 8 ns/op 16 B/op
BenchmarkEnhance/original-10 100 10 ns/op 8 B/op
BenchmarkEnhance/enhanced-10 100 8 ns/op 16 B/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	require.Equal(t, TradeOff, report.Verdicts[0].Outcome, "unexpected outcome")
}

func TestParseGoTestBenchModeTie(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	require.Equal(t, Tie, report.Verdicts[0].Outcome, "unexpected outcome")
}

func TestParseGoTestBenchModeOldWins(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	require.Equal(t, OldWins, report.Verdicts[0].Outcome, "unexpected outcome")
}

func TestParseGoTestBenchModeMissingBaseline(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/enhanced-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, "missing-baseline", got.ReasonCode, "unexpected reason code")
}

func TestParseGoTestBenchModeMissingCandidate(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/original-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, "missing-candidate", got.ReasonCode, "unexpected reason code")
}

func TestParseGoTestBenchModeInsufficientSamples(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 7 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonInsufficient, got.ReasonCode, "unexpected reason code")
}

func TestParseGoTestBenchRawSampleBoundaries(t *testing.T) {
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

			report, err := Parse(strings.NewReader(rawGoTestBenchSamples(test.count)), Options{Mode: ModeGoTestBench})
			require.NoError(t, err)

			got := report.Verdicts[0]

			require.Equal(t, test.wantOutcome, got.Outcome, "unexpected outcome")
			require.Equal(t, test.wantReason, got.ReasonCode, "unexpected reason code")
		})
	}
}

func TestParseGoTestBenchModeUnsupportedMetric(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 MB/s
BenchmarkEnhance/enhanced-10 100 9 MB/s
`

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, "unsupported-metric", got.ReasonCode, "unexpected reason code")
}

func TestParseGoTestBenchModeNoCommonMetric(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 1 allocs/op
BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 1 allocs/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, "unsupported-metric", got.ReasonCode, "unexpected reason code")
}

func TestParseGoTestBenchModeMalformedBenchmark(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance 100 8 ns/op\n"), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestParseGoTestBenchModeMalformedShortRow(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance/original-10 100\n"), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestParseGoTestBenchModeMalformedIteration(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance/original-10 nope 8 ns/op\n"), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestParseGoTestBenchModeMalformedSupportedMetricValue(t *testing.T) {
	t.Parallel()

	report, err := Parse(
		strings.NewReader("BenchmarkEnhance/original-10 100 nope ns/op\n"),
		Options{Mode: ModeGoTestBench},
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

func TestParseGoTestBenchModeNoBenchmarkRows(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("PASS\n"), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	got := report.Verdicts[0]

	require.Equal(t, Inconclusive, got.Outcome, "unexpected outcome")
	require.Equal(t, reasonMalformedBenchmark, got.ReasonCode, "unexpected reason code")
}

func TestParseGoTestBenchModeSkipsUnrequestedLabels(t *testing.T) {
	t.Parallel()

	input := rawGoTestBenchInput + "BenchmarkEnhance/control-10 100 1 ns/op\n"

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)
	require.Equal(t, NewWins, report.Verdicts[0].Outcome, "unexpected outcome")
}

func TestParseGoTestBenchModeSortsMixedVerdicts(t *testing.T) {
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

	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench})
	require.NoError(t, err)

	require.Equal(t, "BenchmarkA", report.Verdicts[0].Benchmark,
		"expected BenchmarkA to be sorted before BenchmarkZ")
	require.Equal(t, "BenchmarkZ", report.Verdicts[1].Benchmark,
		"expected BenchmarkZ to be sorted after BenchmarkA")
}

func TestSortedAlternativeParentsOrdersMapKeys(t *testing.T) {
	t.Parallel()

	samples := rawbench.Samples{
		"BenchmarkZ": {},
		"BenchmarkA": {},
		"BenchmarkM": {},
	}

	require.Equal(t, []string{"BenchmarkA", "BenchmarkM", "BenchmarkZ"}, sortedAlternativeParents(samples),
		"alternative parents should sort map keys deterministically")
}

func TestCommonMetricsOrdersMapKeys(t *testing.T) {
	t.Parallel()

	left := map[string][]float64{
		benchparser.MetricBytesPerOp:  {1},
		metricSecPerOp:                {1},
		benchparser.MetricAllocsPerOp: {1},
	}
	right := map[string][]float64{
		metricSecPerOp:                {1},
		benchparser.MetricAllocsPerOp: {1},
		benchparser.MetricBytesPerOp:  {1},
	}

	want := []string{benchparser.MetricBytesPerOp, benchparser.MetricAllocsPerOp, metricSecPerOp}
	require.Equal(t, want, commonMetrics(left, right),
		"common metrics should sort map keys deterministically")
}

func TestParseGoTestBenchModeVariableSamplesUsePValueApproximation(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
BenchmarkEnhance/original-10 100 12 ns/op
BenchmarkEnhance/enhanced-10 100 9 ns/op
BenchmarkEnhance/original-10 100 11 ns/op
BenchmarkEnhance/enhanced-10 100 8.5 ns/op
`
	report, err := Parse(strings.NewReader(input), Options{Mode: ModeGoTestBench, Alpha: 1})
	require.NoError(t, err)

	got := report.Verdicts[0].Metrics[0].PValue
	require.Greater(t, got, 0.0,
		"unexpected p-value")
	require.Less(t, got, 1.0,
		"unexpected p-value")
}

func TestPrivateAlternativeMathBranches(t *testing.T) {
	t.Parallel()

	got := variance([]float64{1}, 1)
	require.InDelta(t, 0.0, got, 1e-9, "variance of one sample should be zero")

	got = deltaPercent(0, 10)
	require.InDelta(t, 0.0, got, 1e-9, "delta percent with zero baseline should be zero")
}

func TestPrivateAlternativeEmptyReportBranches(t *testing.T) {
	t.Parallel()

	unsupportedState := rawbench.GoTestBench{HasUnsupportedRows: true}

	got := emptyGoTestBenchReport(unsupportedState)
	require.Equal(t, reasonUnsupported, got.Verdicts[0].ReasonCode,
		"report reason code should indicate unsupported metrics when state has unsupported rows")

	emptyState := rawbench.GoTestBench{}

	got = emptyGoTestBenchReport(emptyState)
	require.Equal(t, reasonMalformedBenchmark, got.Verdicts[0].ReasonCode,
		"report reason code should indicate malformed benchmark when state is empty")
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

func TestPrivateDisplayLabelWithFallbackBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		label    string
		fallback string
		want     string
	}{
		{
			name:     "empty label uses fallback",
			label:    "",
			fallback: labelOld,
			want:     labelOld,
		},
		{
			name:     "label is displayed when set",
			label:    "path/to/new.txt",
			fallback: labelOld,
			want:     labelNewTxt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, displayLabelWithFallback(tt.label, tt.fallback))
		})
	}
}

func TestLabelWithFallbackBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, labelOld, labelWithFallback("", labelOld))
	require.Equal(t, labelNew, labelWithFallback(labelNew, labelOld))
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
