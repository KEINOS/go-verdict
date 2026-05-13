package verdict

import (
	"errors"
	"io"
	"strings"
	"testing"
)

const (
	benchmarkFoo = "Foo-8"
	reasonSame   = "same"
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
Foo-8,1.0e-08,1%,8.0e-09,1%,-20.00%,0.001
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Verdicts) != 1 {
		t.Fatalf("verdict count = %d, want 1", len(report.Verdicts))
	}

	if report.Verdicts[0].Outcome != NewWins {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, NewWins)
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
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Verdicts) != 1 {
		t.Fatalf("verdict count = %d, want 1", len(report.Verdicts))
	}

	if report.Verdicts[0].Outcome != Inconclusive {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, Inconclusive)
	}

	if report.Verdicts[0].ReasonCode != "missing-pvalue" {
		t.Fatalf("reasonCode = %q, want %q", report.Verdicts[0].ReasonCode, "missing-pvalue")
	}
}

func TestHigherRateMetricTreatsPositiveDeltaAsImproved(t *testing.T) {
	t.Parallel()

	input := `name          old MB/s  new MB/s  delta
Foo-8         100.0    120.0    +20.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdicts[0].Outcome != NewWins {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, NewWins)
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
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Verdicts[0].Metrics) != 2 {
		t.Fatalf("metric count = %d, want 2", len(report.Verdicts[0].Metrics))
	}

	if report.Verdicts[0].Metrics[0].Metric != "alloc/op" {
		t.Fatalf("first metric = %q, want %q", report.Verdicts[0].Metrics[0].Metric, "alloc/op")
	}

	if report.Verdicts[0].Metrics[1].Metric != metricSecPerOp {
		t.Fatalf("second metric = %q, want %q", report.Verdicts[0].Metrics[1].Metric, metricSecPerOp)
	}
}

func TestParseOldBenchstatFormatNewWins(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=0.000 n=10+10)

name          old alloc/op  new alloc/op  delta
Foo-8         2.00 ± 0%     2.00 ± 0%       ~     (p=1.000 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0].Outcome
	if got != NewWins {
		t.Fatalf("outcome = %s, want %s", got, NewWins)
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
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0].Outcome
	if got != TradeOff {
		t.Fatalf("outcome = %s, want %s", got, TradeOff)
	}
}

func TestInsignificantDifferenceIsTie(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   9.9ns ± 1%  -1.00% (p=0.300 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0].Outcome
	if got != Tie {
		t.Fatalf("outcome = %s, want %s", got, Tie)
	}
}

func TestHigherIsBetterMetric(t *testing.T) {
	t.Parallel()

	input := `name          old speed  new speed  delta
Foo-8         100MB/s    120MB/s   +20.00% (p=0.000 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 0})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0].Outcome
	if got != NewWins {
		t.Fatalf("outcome = %s, want %s", got, NewWins)
	}
}

func TestParseReaderErrorContainsContext(t *testing.T) {
	t.Parallel()

	_, err := Parse(failingReader{}, Options{})
	if err == nil {
		t.Fatal("expected read error")
	}

	if !strings.Contains(err.Error(), "reading benchstat input") {
		t.Fatalf("error = %q, want reading context", err.Error())
	}
}

func TestParseTextScannerErrorContainsContext(t *testing.T) {
	t.Parallel()

	longLine := strings.Repeat("x", 70*1024)

	_, err := Parse(strings.NewReader(longLine), Options{})
	if err == nil {
		t.Fatal("expected scanner error")
	}

	if !strings.Contains(err.Error(), "scanning benchstat text input") {
		t.Fatalf("error = %q, want scanner context", err.Error())
	}
}

func TestParseCSVErrorContainsContext(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
"Foo-8,1.0e-08,1%,8.0e-09,1%,-20.00%,0.001
`

	_, err := Parse(strings.NewReader(input), Options{})
	if err == nil {
		t.Fatal("expected csv parse error")
	}

	if !strings.Contains(err.Error(), "reading benchstat csv input") {
		t.Fatalf("error = %q, want csv context", err.Error())
	}
}

func TestParseEmptyInputReturnsError(t *testing.T) {
	t.Parallel()

	_, err := Parse(strings.NewReader(""), Options{})
	if !errors.Is(err, errNoComparisonRows) {
		t.Fatalf("error = %v, want %v", err, errNoComparisonRows)
	}
}

func TestParseCSVBenchmarkSetMismatchReturnsInconclusive(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,,1%,,1%,?,?
`

	report, err := Parse(strings.NewReader(input), Options{})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != "benchmark-set-mismatch" {
		t.Fatalf("verdict = %+v, want benchmark-set-mismatch inconclusive", got)
	}
}

func TestParseCSVMissingPValueReturnsInconclusive(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,1.0,1%,0.9,1%,-10.00%,?
`

	report, err := Parse(strings.NewReader(input), Options{})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != "missing-pvalue" {
		t.Fatalf("verdict = %+v, want missing-pvalue inconclusive", got)
	}
}

func TestParseCSVWithOnlyHeaderReturnsError(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
`

	_, err := Parse(strings.NewReader(input), Options{})
	if !errors.Is(err, errNoComparisonRows) {
		t.Fatalf("error = %v, want %v", err, errNoComparisonRows)
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
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Verdicts) != 1 || report.Verdicts[0].Benchmark != "Good-8" {
		t.Fatalf("verdicts = %+v, want only Good-8", report.Verdicts)
	}
}

func TestParseTextRowsWithoutUsablePValueReturnsError(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=n/a n=10+10)
`

	_, err := Parse(strings.NewReader(input), Options{})
	if !errors.Is(err, errNoComparisonRows) {
		t.Fatalf("error = %v, want %v", err, errNoComparisonRows)
	}
}

func TestParseTextRowsWithoutDeltaReturnsError(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  changed (p=0.001 n=10+10)
`

	_, err := Parse(strings.NewReader(input), Options{})
	if !errors.Is(err, errNoComparisonRows) {
		t.Fatalf("error = %v, want %v", err, errNoComparisonRows)
	}
}

func TestMinDeltaThresholdMakesSignificantSmallChangeTie(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   9.9ns ± 1%  -1.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05, MinDeltaPct: 2})
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdicts[0].Outcome != Tie {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, Tie)
	}
}

func TestOldWinsWhenOnlyRegressionExists(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   12.0ns ± 1%  +20.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Alpha: 0.05})
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdicts[0].Outcome != OldWins {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, OldWins)
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
	if err := report.WriteText(&output); err != nil {
		t.Fatal(err)
	}

	got := output.String()
	for _, want := range []string{"Foo-8: trade-off", "reason_code=example", "+ sec/op", "- B/op", "= allocs/op"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestWriteTextNoVerdictsWritesNothing(t *testing.T) {
	t.Parallel()

	var output strings.Builder
	if err := (Report{}).WriteText(&output); err != nil {
		t.Fatal(err)
	}

	if output.String() != "" {
		t.Fatalf("output = %q, want empty", output.String())
	}
}

func TestWriteTextErrorContainsContext(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	err := report.WriteText(failingWriter{})
	if err == nil {
		t.Fatal("expected write error")
	}

	if !strings.Contains(err.Error(), "writing text report") {
		t.Fatalf("error = %q, want text output context", err.Error())
	}
}

func TestWriteTextReasonErrorContainsContext(t *testing.T) {
	t.Parallel()

	writer := failAfterWriter{limit: 2}
	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	err := report.WriteText(&writer)
	if err == nil {
		t.Fatal("expected reason write error")
	}

	if !strings.Contains(err.Error(), "writing text report") {
		t.Fatalf("error = %q, want text output context", err.Error())
	}
}

func TestWriteJSONSuccess(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	var output strings.Builder
	if err := report.WriteJSON(&output); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output.String(), `"benchmark": "`+benchmarkFoo+`"`) {
		t.Fatalf("output = %q, want benchmark json", output.String())
	}
}

func TestWriteJSONErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := (Report{}).WriteJSON(failingWriter{})
	if err == nil {
		t.Fatal("expected json write error")
	}

	if !strings.Contains(err.Error(), "writing json report") {
		t.Fatalf("error = %q, want json output context", err.Error())
	}
}

func TestDirectionMarkDefault(t *testing.T) {
	t.Parallel()

	if got := directionMark(Direction("unknown")); got != "=" {
		t.Fatalf("mark = %q, want =", got)
	}
}

func TestPrivateEdgeBranches(t *testing.T) {
	t.Parallel()

	state := newCSVParseState()
	state.handleRecord(nil, Options{})

	if len(state.rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(state.rows))
	}

	state.updateBenchmarkSetMismatch([]string{benchmarkFoo})

	if state.hasBenchmarkSetMismatch {
		t.Fatal("benchmark set mismatch should remain false when fields are missing")
	}

	state.metric = metricSecPerOp
	state.pValueIndex = 1

	_, ok := state.parseComparison([]string{benchmarkFoo, "0.001"}, Options{})
	if ok {
		t.Fatal("comparison with missing delta index should not parse")
	}

	if got := findFieldIndex([]string{"foo"}, "bar"); got != -1 {
		t.Fatalf("index = %d, want -1", got)
	}

	for _, rawDelta := range []string{"", "~", "?", "bad"} {
		if _, ok := parseDeltaPercent(rawDelta); ok {
			t.Fatalf("delta %q parsed, want false", rawDelta)
		}
	}

	if looksLikeComparisonLine("Foo") {
		t.Fatal("single-field line should not look like comparison")
	}
}

func TestDecideNoMetricsIsInconclusive(t *testing.T) {
	t.Parallel()

	outcome, reason := decide(0, 0, 0)
	if outcome != Inconclusive || reason == "" {
		t.Fatalf("decide = %s, %q; want inconclusive with reason", outcome, reason)
	}
}

func TestWriteTextMetricErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := writeTextMetric(failingWriter{}, Comparison{Direction: Same})
	if err == nil {
		t.Fatal("expected metric write error")
	}

	if !strings.Contains(err.Error(), "writing text report") {
		t.Fatalf("error = %q, want text output context", err.Error())
	}
}

func TestWriteTextReasonCodeErrorContainsContext(t *testing.T) {
	t.Parallel()

	writer := failAfterWriter{limit: 3}
	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame, ReasonCode: "example"}},
	}

	err := report.WriteText(&writer)
	if err == nil {
		t.Fatal("expected reason code write error")
	}

	if !strings.Contains(err.Error(), "writing text report") {
		t.Fatalf("error = %q, want text output context", err.Error())
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

	err := report.WriteText(&writer)
	if err == nil {
		t.Fatal("expected metric write error")
	}

	if !strings.Contains(err.Error(), "writing text report") {
		t.Fatalf("error = %q, want text output context", err.Error())
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
