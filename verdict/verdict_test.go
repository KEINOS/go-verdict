package verdict

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	benchmarkFoo             = "Foo-8"
	labelCandidate           = "candidate"
	labelNew                 = "new"
	labelNewTxt              = "new.txt"
	labelOld                 = "old"
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

	if len(report.Verdicts) != 1 {
		require.Failf(t, "assertion failed", "verdict count = %d, want 1", len(report.Verdicts))
	}

	if report.Verdicts[0].Outcome != NewWins {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", report.Verdicts[0].Outcome, NewWins)
	}

	if report.Verdicts[0].Winner != labelNewTxt {
		require.Failf(t, "assertion failed", "winner = %q, want %q", report.Verdicts[0].Winner, labelNewTxt)
	}
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

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Mode: modeBenchstat})
	require.NoError(t, err)

	if report.Verdicts[0].Winner != labelNew {
		require.Failf(t, "assertion failed", "winner = %q, want new", report.Verdicts[0].Winner)
	}
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

	if len(report.Verdicts) != 1 {
		require.Failf(t, "assertion failed", "verdict count = %d, want 1", len(report.Verdicts))
	}

	if report.Verdicts[0].Outcome != Inconclusive {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", report.Verdicts[0].Outcome, Inconclusive)
	}

	if report.Verdicts[0].ReasonCode != "missing-pvalue" {
		require.Failf(t, "assertion failed", "reasonCode = %q, want %q", report.Verdicts[0].ReasonCode, "missing-pvalue")
	}
}

func TestHigherRateMetricTreatsPositiveDeltaAsImproved(t *testing.T) {
	t.Parallel()

	input := `name          old MB/s  new MB/s  delta
Foo-8         100.0    120.0    +20.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	if report.Verdicts[0].Outcome != NewWins {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", report.Verdicts[0].Outcome, NewWins)
	}
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
	if got != NewWins {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", got, NewWins)
	}
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
	if got != TradeOff {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", got, TradeOff)
	}
}

func TestInsignificantDifferenceIsTie(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   9.9ns ± 1%  -1.00% (p=0.300 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	got := report.Verdicts[0].Outcome
	if got != Tie {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", got, Tie)
	}
}

func TestHigherIsBetterMetric(t *testing.T) {
	t.Parallel()

	input := `name          old speed  new speed  delta
Foo-8         100MB/s    120MB/s   +20.00% (p=0.000 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	require.NoError(t, err)

	got := report.Verdicts[0].Outcome
	if got != NewWins {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", got, NewWins)
	}
}

func TestParseReaderErrorContainsContext(t *testing.T) {
	t.Parallel()

	_, err := Parse(failingReader{}, Options{})
	require.Error(t, err, "expected read error")

	if !strings.Contains(err.Error(), "reading benchstat input") {
		require.Failf(t, "assertion failed", "error = %q, want reading context", err.Error())
	}
}

func TestParseTextScannerErrorContainsContext(t *testing.T) {
	t.Parallel()

	longLine := strings.Repeat("x", 70*1024)

	_, err := Parse(strings.NewReader(longLine), Options{})
	require.Error(t, err, "expected scanner error")

	if !strings.Contains(err.Error(), "scanning benchstat text input") {
		require.Failf(t, "assertion failed", "error = %q, want scanner context", err.Error())
	}
}

func TestParseAlternativesScannerErrorContainsContext(t *testing.T) {
	t.Parallel()

	longLine := "BenchmarkEnhance/original-10 100 " + strings.Repeat("1", 70*1024) + " ns/op\n"

	_, err := Parse(strings.NewReader(longLine), Options{Mode: altMode})
	require.Error(t, err, "expected scanner error")

	if !strings.Contains(err.Error(), "scanning raw alternatives input") {
		require.Failf(t, "assertion failed", "error = %q, want raw alternatives scanner context", err.Error())
	}
}

func TestParseCSVErrorContainsContext(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
"Foo-8,1.0e-08,1%,8.0e-09,1%,-20.00%,0.001
`

	_, err := Parse(strings.NewReader(input), Options{})
	require.Error(t, err, "expected csv parse error")

	if !strings.Contains(err.Error(), "reading benchstat csv input") {
		require.Failf(t, "assertion failed", "error = %q, want csv context", err.Error())
	}
}

func TestParseEmptyInputReturnsError(t *testing.T) {
	t.Parallel()

	_, err := Parse(strings.NewReader(""), Options{})
	if !errors.Is(err, errNoComparisonRows) {
		require.Failf(t, "assertion failed", "error = %v, want %v", err, errNoComparisonRows)
	}
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
	if got.Outcome != Inconclusive || got.ReasonCode != "benchmark-set-mismatch" {
		require.Failf(t, "assertion failed", "verdict = %+v, want benchmark-set-mismatch inconclusive", got)
	}
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
	if got.Outcome != Inconclusive || got.ReasonCode != "missing-pvalue" {
		require.Failf(t, "assertion failed", "verdict = %+v, want missing-pvalue inconclusive", got)
	}
}

func TestParseCSVWithOnlyHeaderReturnsError(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
`

	_, err := Parse(strings.NewReader(input), Options{})
	if !errors.Is(err, errNoComparisonRows) {
		require.Failf(t, "assertion failed", "error = %v, want %v", err, errNoComparisonRows)
	}
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

	if len(report.Verdicts) != 1 || report.Verdicts[0].Benchmark != "Good-8" {
		require.Failf(t, "assertion failed", "verdicts = %+v, want only Good-8", report.Verdicts)
	}
}

func TestParseTextRowsWithoutUsablePValueReturnsError(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=n/a n=10+10)
`

	_, err := Parse(strings.NewReader(input), Options{})
	if !errors.Is(err, errNoComparisonRows) {
		require.Failf(t, "assertion failed", "error = %v, want %v", err, errNoComparisonRows)
	}
}

func TestParseTextRowsWithoutDeltaReturnsError(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  changed (p=0.001 n=10+10)
`

	_, err := Parse(strings.NewReader(input), Options{})
	if !errors.Is(err, errNoComparisonRows) {
		require.Failf(t, "assertion failed", "error = %v, want %v", err, errNoComparisonRows)
	}
}

func TestMinDeltaThresholdMakesSignificantSmallChangeTie(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   9.9ns ± 1%  -1.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 2})
	require.NoError(t, err)

	if report.Verdicts[0].Outcome != Tie {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", report.Verdicts[0].Outcome, Tie)
	}
}

func TestOldWinsWhenOnlyRegressionExists(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   12.0ns ± 1%  +20.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05})
	require.NoError(t, err)

	if report.Verdicts[0].Outcome != OldWins {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", report.Verdicts[0].Outcome, OldWins)
	}
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
	if err := report.WriteVerboseText(&output); err != nil {
		require.NoError(t, err)
	}

	got := output.String()
	for _, want := range []string{"Foo-8: trade-off", "reason_code=example", "+ sec/op", "- B/op", "= allocs/op"} {
		if !strings.Contains(got, want) {
			require.Failf(t, "assertion failed", "output = %q, want %q", got, want)
		}
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
	if err := report.WriteText(&output); err != nil {
		require.NoError(t, err)
	}

	want := "Foo-8: new.txt wins\nBar-8: tie\n"
	if output.String() != want {
		require.Failf(t, "assertion failed", "output = %q, want %q", output.String(), want)
	}
}

func TestWriteTextNoVerdictsWritesNothing(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	if err := (Report{}).WriteText(&output); err != nil {
		require.NoError(t, err)
	}

	if output.String() != "" {
		require.Failf(t, "assertion failed", "output = %q, want empty", output.String())
	}
}

func TestWriteTextErrorContainsContext(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	err := report.WriteText(failingWriter{})
	require.Error(t, err, "expected write error")

	if !strings.Contains(err.Error(), "writing text report") {
		require.Failf(t, "assertion failed", "error = %q, want text output context", err.Error())
	}
}

func TestWriteTextReasonErrorContainsContext(t *testing.T) {
	t.Parallel()

	writer := failAfterWriter{limit: 2}
	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	err := report.WriteVerboseText(&writer)
	require.Error(t, err, "expected reason write error")

	if !strings.Contains(err.Error(), "writing text report") {
		require.Failf(t, "assertion failed", "error = %q, want text output context", err.Error())
	}
}

func TestWriteJSONSuccess(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	var output strings.Builder
	if err := report.WriteJSON(&output); err != nil {
		require.NoError(t, err)
	}

	if !strings.Contains(output.String(), `"benchmark": "`+benchmarkFoo+`"`) {
		require.Failf(t, "assertion failed", "output = %q, want benchmark json", output.String())
	}
}

func TestWriteJSONErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := (Report{}).WriteJSON(failingWriter{})
	require.Error(t, err, "expected json write error")

	if !strings.Contains(err.Error(), "writing json report") {
		require.Failf(t, "assertion failed", "error = %q, want json output context", err.Error())
	}
}

func TestDirectionMarkDefault(t *testing.T) {
	t.Parallel()

	if got := directionMark(Direction("unknown")); got != "=" {
		require.Failf(t, "assertion failed", "mark = %q, want =", got)
	}
}

func TestPrivateEdgeBranches(t *testing.T) {
	t.Parallel()

	state := newCSVParseState()
	state.handleRecord(nil, Options{})

	if len(state.rows) != 0 {
		require.Failf(t, "assertion failed", "rows = %d, want 0", len(state.rows))
	}

	state.updateBenchmarkSetMismatch([]string{benchmarkFoo})

	if state.hasBenchmarkSetMismatch {
		require.FailNow(t, "benchmark set mismatch should remain false when fields are missing")
	}

	state.metric = metricSecPerOp
	state.pValueIndex = 1

	_, ok := state.parseComparison([]string{benchmarkFoo, "0.001"}, Options{})
	if ok {
		require.FailNow(t, "comparison with missing delta index should not parse")
	}

	if got := findFieldIndex([]string{"foo"}, "bar"); got != -1 {
		require.Failf(t, "assertion failed", "index = %d, want -1", got)
	}

	for _, rawDelta := range []string{"", "~", "?", "bad"} {
		if _, ok := parseDeltaPercent(rawDelta); ok {
			require.Failf(t, "assertion failed", "delta %q parsed, want false", rawDelta)
		}
	}

	if looksLikeComparisonLine("Foo") {
		require.FailNow(t, "single-field line should not look like comparison")
	}
}

func TestDecideNoMetricsIsInconclusive(t *testing.T) {
	t.Parallel()

	outcome, reason := decide(0, 0, 0)
	if outcome != Inconclusive || reason == "" {
		require.Failf(t, "assertion failed", "decide = %s, %q; want inconclusive with reason", outcome, reason)
	}
}

func TestWriteTextMetricErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := writeTextMetric(failingWriter{}, Comparison{Direction: Same})
	require.Error(t, err, "expected metric write error")

	if !strings.Contains(err.Error(), "writing text report") {
		require.Failf(t, "assertion failed", "error = %q, want text output context", err.Error())
	}
}

func TestWriteTextReasonCodeErrorContainsContext(t *testing.T) {
	t.Parallel()

	writer := failAfterWriter{limit: 3}
	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame, ReasonCode: "example"}},
	}

	err := report.WriteVerboseText(&writer)
	require.Error(t, err, "expected reason code write error")

	if !strings.Contains(err.Error(), "writing text report") {
		require.Failf(t, "assertion failed", "error = %q, want text output context", err.Error())
	}
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
	require.Error(t, err, "expected metric write error")

	if !strings.Contains(err.Error(), "writing text report") {
		require.Failf(t, "assertion failed", "error = %q, want text output context", err.Error())
	}
}

func TestParseAlternativesModeNewWins(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(rawAltInput), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Benchmark != "BenchmarkEnhance" || got.Outcome != NewWins {
		require.Failf(t, "assertion failed", "verdict = %+v, want BenchmarkEnhance new-wins", got)
	}

	if len(got.Metrics) != 3 {
		require.Failf(t, "assertion failed", "metrics = %d, want 3", len(got.Metrics))
	}

	if got.Winner != "enhanced" {
		require.Failf(t, "assertion failed", "winner = %q, want enhanced", got.Winner)
	}
}

func TestParseAutoModeRawAlternativesInfersLabels(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(rawAltInput), Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Benchmark != "BenchmarkEnhance" || got.Winner != "enhanced" {
		require.Failf(t, "assertion failed", "verdict = %+v, want BenchmarkEnhance enhanced winner", got)
	}
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
	if got.BaselineLabel != "base" || got.CandidateLabel != labelCandidate || got.Winner != labelCandidate {
		require.Failf(t, "assertion failed", "verdict = %+v, want base/candidate labels with candidate winner", got)
	}
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
	if got.Outcome != Inconclusive || got.ReasonCode != "ambiguous-alternatives" {
		require.Failf(t, "assertion failed", "verdict = %+v, want ambiguous-alternatives", got)
	}
}

func TestCompareRawFilesDifferentBenchmarkNames(t *testing.T) {
	t.Parallel()

	fast := strings.NewReader(strings.Repeat("BenchmarkExampleFast-10 100 1 ns/op\n", 10))
	slow := strings.NewReader(strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10))

	report, err := CompareRawFiles(fast, slow, Options{})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Benchmark != "BenchmarkExampleFast_vs_BenchmarkExampleSlow" ||
		got.Winner != "BenchmarkExampleFast" ||
		got.Outcome != OldWins {
		require.Failf(t, "assertion failed", "verdict = %+v, want BenchmarkExampleFast winner", got)
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
		{count: RawComparisonMinSamples, wantOutcome: OldWins},
		{count: 8, wantOutcome: OldWins},
		{count: RecommendedRawSamples - 1, wantOutcome: OldWins},
		{count: RecommendedRawSamples, wantOutcome: OldWins},
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
			if got.Outcome != test.wantOutcome {
				require.Failf(t, "assertion failed", "outcome = %s, want %s", got.Outcome, test.wantOutcome)
			}

			if got.ReasonCode != test.wantReason {
				require.Failf(t, "assertion failed", "reason = %q, want %q", got.ReasonCode, test.wantReason)
			}
		})
	}
}

type rawFileInconclusiveCase struct {
	name   string
	aInput string
	bInput string
	reason string
}

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

	if _, err := CompareRawFiles(failingReader{}, strings.NewReader(""), Options{}); err == nil {
		require.FailNow(t, "expected a reader error")
	}

	if _, err := CompareRawFiles(strings.NewReader("PASS\n"), failingReader{}, Options{}); err == nil {
		require.FailNow(t, "expected b reader error")
	}

	longLine := strings.NewReader(strings.Repeat("x", 70*1024))

	_, err := CompareRawFiles(longLine, strings.NewReader(""), Options{})
	require.Error(t, err, "expected scanner error")

	if !strings.Contains(err.Error(), "scanning raw benchmark file input") {
		require.Failf(t, "assertion failed", "error = %q, want raw file scanner context", err.Error())
	}
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
	if got.ReasonCode != "benchmark-set-mismatch" ||
		got.BaselineLabel != "./fast.txt" ||
		got.CandidateLabel != "./slow.txt" {
		require.Failf(t, "assertion failed", "verdict = %+v, want benchmark-set-mismatch labels", got)
	}
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
		if _, _, ok := parseRawFileBenchmarkLine(line); ok {
			require.Failf(t, "assertion failed", "line %q parsed, want false", line)
		}
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
		Mode:      altMode,
		Baseline:  "base",
		Candidate: labelCandidate,
	})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Benchmark != "BenchmarkEnhance/group" || got.Outcome != NewWins {
		require.Failf(t, "assertion failed", "verdict = %+v, want nested new-wins", got)
	}
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

	if report.Verdicts[0].Outcome != TradeOff {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", report.Verdicts[0].Outcome, TradeOff)
	}
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

	if report.Verdicts[0].Outcome != Tie {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", report.Verdicts[0].Outcome, Tie)
	}
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

	if report.Verdicts[0].Outcome != OldWins {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", report.Verdicts[0].Outcome, OldWins)
	}
}

func TestParseAlternativesModeMissingBaseline(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/enhanced-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != "missing-baseline" {
		require.Failf(t, "assertion failed", "verdict = %+v, want missing-baseline", got)
	}
}

func TestParseAlternativesModeMissingCandidate(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/original-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != "missing-candidate" {
		require.Failf(t, "assertion failed", "verdict = %+v, want missing-candidate", got)
	}
}

func TestParseAlternativesModeInsufficientSamples(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 7 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonInsufficient {
		require.Failf(t, "assertion failed", "verdict = %+v, want insufficient-samples", got)
	}
}

func TestParseAlternativesRawSampleBoundaries(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		count       int
		wantReason  string
		wantOutcome Outcome
	}{
		{count: StatisticalMinSamples, wantReason: reasonInsufficient, wantOutcome: Inconclusive},
		{count: RawComparisonMinSamples, wantOutcome: NewWins},
		{count: 8, wantOutcome: NewWins},
		{count: RecommendedRawSamples - 1, wantOutcome: NewWins},
		{count: RecommendedRawSamples, wantOutcome: NewWins},
	} {
		t.Run(fmt.Sprintf("count_%d", test.count), func(t *testing.T) {
			t.Parallel()

			report, err := Parse(strings.NewReader(rawAlternativeSamples(test.count)), Options{Mode: altMode})
			require.NoError(t, err)

			got := report.Verdicts[0]
			if got.Outcome != test.wantOutcome {
				require.Failf(t, "assertion failed", "outcome = %s, want %s", got.Outcome, test.wantOutcome)
			}

			if got.ReasonCode != test.wantReason {
				require.Failf(t, "assertion failed", "reason = %q, want %q", got.ReasonCode, test.wantReason)
			}
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
	if got.Outcome != Inconclusive || got.ReasonCode != "unsupported-metric" {
		require.Failf(t, "assertion failed", "verdict = %+v, want unsupported-metric", got)
	}
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
	if got.Outcome != Inconclusive || got.ReasonCode != "unsupported-metric" {
		require.Failf(t, "assertion failed", "verdict = %+v, want unsupported-metric", got)
	}
}

func TestParseAlternativesModeMalformedBenchmark(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance 100 8 ns/op\n"), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonMalformedBenchmark {
		require.Failf(t, "assertion failed", "verdict = %+v, want malformed-benchmark", got)
	}
}

func TestParseAlternativesModeMalformedShortRow(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance/original-10 100\n"), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonMalformedBenchmark {
		require.Failf(t, "assertion failed", "verdict = %+v, want malformed-benchmark", got)
	}
}

func TestParseAlternativesModeMalformedIteration(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance/original-10 nope 8 ns/op\n"), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonMalformedBenchmark {
		require.Failf(t, "assertion failed", "verdict = %+v, want malformed-benchmark", got)
	}
}

func TestParseAlternativesModeNoBenchmarkRows(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("PASS\n"), Options{Mode: altMode})
	require.NoError(t, err)

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonMalformedBenchmark {
		require.Failf(t, "assertion failed", "verdict = %+v, want malformed-benchmark", got)
	}
}

func TestParseAlternativesModeSkipsUnrequestedLabels(t *testing.T) {
	t.Parallel()

	input := rawAltInput + "BenchmarkEnhance/control-10 100 1 ns/op\n"

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	require.NoError(t, err)

	if report.Verdicts[0].Outcome != NewWins {
		require.Failf(t, "assertion failed", "outcome = %s, want %s", report.Verdicts[0].Outcome, NewWins)
	}
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

	if report.Verdicts[0].Benchmark != "BenchmarkA" || report.Verdicts[1].Benchmark != "BenchmarkZ" {
		require.Failf(t, "assertion failed", "verdicts = %+v, want sorted mixed verdicts", report.Verdicts)
	}
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
		require.Failf(t, "assertion failed", "p-value = %f, want normal approximation between 0 and 1", got)
	}
}

func TestPrivateAlternativeBranches(t *testing.T) {
	t.Parallel()

	if got := trimCPUSuffix("BenchmarkFoo/original"); got != "BenchmarkFoo/original" {
		require.Failf(t, "assertion failed", "name = %q, want no trim", got)
	}

	if got := trimCPUSuffix("BenchmarkFoo/original-fast"); got != "BenchmarkFoo/original-fast" {
		require.Failf(t, "assertion failed", "name = %q, want no trim for non-numeric suffix", got)
	}

	if _, _, ok := splitRawBenchmarkName("BenchmarkFoo-10"); ok {
		require.FailNow(t, "benchmark without sub-benchmark should not split")
	}

	if metric, ok := normalizeRawMetric("MB/s"); ok || metric != "" {
		require.Failf(t, "assertion failed", "metric = %q, ok = %v; want unsupported", metric, ok)
	}
}

func TestPrivateAlternativeMathBranches(t *testing.T) {
	t.Parallel()

	metrics := parseRawMetrics([]string{"bad", metricNanosecondsPerOp, "10", metricNanosecondsPerOp})
	if metrics[metricSecPerOp] != 10 {
		require.Failf(t, "assertion failed", "metrics = %+v, want valid metric after bad value", metrics)
	}

	if got := variance([]float64{1}, 1); got != 0 {
		require.Failf(t, "assertion failed", "variance = %f, want 0 for one sample", got)
	}

	if got := deltaPercent(0, 10); got != 0 {
		require.Failf(t, "assertion failed", "delta = %f, want 0 for zero baseline", got)
	}
}

func TestPrivateAlternativeEmptyReportBranches(t *testing.T) {
	t.Parallel()

	insufficientState := alternativeParseState{hasInsufficientRows: true}
	if got := insufficientState.emptyAlternativeReport(); got.Verdicts[0].ReasonCode != reasonInsufficient {
		require.Failf(t, "assertion failed", "report = %+v, want insufficient-samples", got)
	}

	emptyState := alternativeParseState{}
	if got := emptyState.emptyAlternativeReport(); got.Verdicts[0].ReasonCode != reasonMalformedBenchmark {
		require.Failf(t, "assertion failed", "report = %+v, want malformed-benchmark", got)
	}
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

	if got := emptyTextState.rawBaselineLabel(); got != labelOld {
		require.Failf(t, "assertion failed", "raw baseline label = %q, want old", got)
	}

	if got := emptyTextState.rawCandidateLabel(); got != labelNew {
		require.Failf(t, "assertion failed", "raw candidate label = %q, want new", got)
	}
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

	if got := displayLabel(""); got != "" {
		require.Failf(t, "assertion failed", "empty label = %q, want empty", got)
	}

	if got := displayLabel("."); got != "." {
		require.Failf(t, "assertion failed", "dot label = %q, want dot", got)
	}

	baselineLabel, candidateLabel := comparisonLabels([]Comparison{{}})
	if baselineLabel != labelOld || candidateLabel != labelNew {
		require.Failf(t, "assertion failed", "blank comparison labels = %q/%q, want old/new", baselineLabel, candidateLabel)
	}

	baselineLabel, candidateLabel = comparisonLabels(nil)
	if baselineLabel != labelOld || candidateLabel != labelNew {
		require.Failf(t, "assertion failed", "empty comparison labels = %q/%q, want old/new", baselineLabel, candidateLabel)
	}

	if got := winnerLabel(Outcome("unknown"), labelOld, labelNew); got != "" {
		require.Failf(t, "assertion failed", "unknown outcome winner = %q, want empty", got)
	}
}

func TestWriteVerboseTextHeaderErrorContainsContext(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	err := report.WriteVerboseText(failingWriter{})
	require.Error(t, err, "expected header write error")

	if !strings.Contains(err.Error(), "writing text report") {
		require.Failf(t, "assertion failed", "error = %q, want text output context", err.Error())
	}
}

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

var _ io.Writer = (*failAfterWriter)(nil)
