package verdict

import (
	"errors"
	"io"
	"strings"
	"testing"
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
	rawAltInput              = "BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
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
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Verdicts) != 1 {
		t.Fatalf("verdict count = %d, want 1", len(report.Verdicts))
	}

	if report.Verdicts[0].Outcome != NewWins {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, NewWins)
	}

	if report.Verdicts[0].Winner != labelNewTxt {
		t.Fatalf("winner = %q, want %q", report.Verdicts[0].Winner, labelNewTxt)
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
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.BaselineLabel != "bench_old.txt" || got.CandidateLabel != "bench_new.txt" {
		t.Fatalf("labels = %q/%q, want bench_old.txt/bench_new.txt", got.BaselineLabel, got.CandidateLabel)
	}

	if got.Winner != "bench_new.txt" {
		t.Fatalf("winner = %q, want bench_new.txt", got.Winner)
	}
}

func TestParseExplicitBenchstatMode(t *testing.T) {
	t.Parallel()

	input := `name          old time/op  new time/op  delta
Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=0.001 n=10+10)
`

	report, err := Parse(strings.NewReader(input), Options{Mode: modeBenchstat})
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdicts[0].Winner != labelNew {
		t.Fatalf("winner = %q, want new", report.Verdicts[0].Winner)
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
	if err := report.WriteVerboseText(&output); err != nil {
		t.Fatal(err)
	}

	got := output.String()
	for _, want := range []string{"Foo-8: trade-off", "reason_code=example", "+ sec/op", "- B/op", "= allocs/op"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
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
		t.Fatal(err)
	}

	want := "Foo-8: new.txt wins\nBar-8: tie\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
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

	err := report.WriteVerboseText(&writer)
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

	err := report.WriteVerboseText(&writer)
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

	err := report.WriteVerboseText(&writer)
	if err == nil {
		t.Fatal("expected metric write error")
	}

	if !strings.Contains(err.Error(), "writing text report") {
		t.Fatalf("error = %q, want text output context", err.Error())
	}
}

func TestParseAlternativesModeNewWins(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(rawAltInput), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Benchmark != "BenchmarkEnhance" || got.Outcome != NewWins {
		t.Fatalf("verdict = %+v, want BenchmarkEnhance new-wins", got)
	}

	if len(got.Metrics) != 3 {
		t.Fatalf("metrics = %d, want 3", len(got.Metrics))
	}

	if got.Winner != "enhanced" {
		t.Fatalf("winner = %q, want enhanced", got.Winner)
	}
}

func TestParseAutoModeRawAlternativesInfersLabels(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader(rawAltInput), Options{})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Benchmark != "BenchmarkEnhance" || got.Winner != "enhanced" {
		t.Fatalf("verdict = %+v, want BenchmarkEnhance enhanced winner", got)
	}
}

func TestParseAutoModeRawAlternativesInfersNonDefaultLabels(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/base-10 100 10 ns/op
BenchmarkEnhance/candidate-10 100 8 ns/op
BenchmarkEnhance/base-10 100 10 ns/op
BenchmarkEnhance/candidate-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.BaselineLabel != "base" || got.CandidateLabel != labelCandidate || got.Winner != labelCandidate {
		t.Fatalf("verdict = %+v, want base/candidate labels with candidate winner", got)
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
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != "ambiguous-alternatives" {
		t.Fatalf("verdict = %+v, want ambiguous-alternatives", got)
	}
}

func TestParseAlternativesModeNestedNameAndCustomLabels(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/group/base-10 100 12 ns/op
BenchmarkEnhance/group/candidate-10 100 10 ns/op
BenchmarkEnhance/group/base-10 100 12 ns/op
BenchmarkEnhance/group/candidate-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{
		Mode:      altMode,
		Baseline:  "base",
		Candidate: labelCandidate,
	})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Benchmark != "BenchmarkEnhance/group" || got.Outcome != NewWins {
		t.Fatalf("verdict = %+v, want nested new-wins", got)
	}
}

func TestParseAlternativesModeTradeOff(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op 8 B/op
BenchmarkEnhance/enhanced-10 100 8 ns/op 16 B/op
BenchmarkEnhance/original-10 100 10 ns/op 8 B/op
BenchmarkEnhance/enhanced-10 100 8 ns/op 16 B/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdicts[0].Outcome != TradeOff {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, TradeOff)
	}
}

func TestParseAlternativesModeTie(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdicts[0].Outcome != Tie {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, Tie)
	}
}

func TestParseAlternativesModeOldWins(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdicts[0].Outcome != OldWins {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, OldWins)
	}
}

func TestParseAlternativesModeMissingBaseline(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/enhanced-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != "missing-baseline" {
		t.Fatalf("verdict = %+v, want missing-baseline", got)
	}
}

func TestParseAlternativesModeMissingCandidate(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/original-10 100 8 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != "missing-candidate" {
		t.Fatalf("verdict = %+v, want missing-candidate", got)
	}
}

func TestParseAlternativesModeInsufficientSamples(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 ns/op
BenchmarkEnhance/enhanced-10 100 7 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonInsufficient {
		t.Fatalf("verdict = %+v, want insufficient-samples", got)
	}
}

func TestParseAlternativesModeUnsupportedMetric(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 8 MB/s
BenchmarkEnhance/enhanced-10 100 9 MB/s
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != "unsupported-metric" {
		t.Fatalf("verdict = %+v, want unsupported-metric", got)
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
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != "unsupported-metric" {
		t.Fatalf("verdict = %+v, want unsupported-metric", got)
	}
}

func TestParseAlternativesModeMalformedBenchmark(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance 100 8 ns/op\n"), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonMalformedBenchmark {
		t.Fatalf("verdict = %+v, want malformed-benchmark", got)
	}
}

func TestParseAlternativesModeMalformedShortRow(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance/original-10 100\n"), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonMalformedBenchmark {
		t.Fatalf("verdict = %+v, want malformed-benchmark", got)
	}
}

func TestParseAlternativesModeMalformedIteration(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("BenchmarkEnhance/original-10 nope 8 ns/op\n"), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonMalformedBenchmark {
		t.Fatalf("verdict = %+v, want malformed-benchmark", got)
	}
}

func TestParseAlternativesModeNoBenchmarkRows(t *testing.T) {
	t.Parallel()

	report, err := Parse(strings.NewReader("PASS\n"), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0]
	if got.Outcome != Inconclusive || got.ReasonCode != reasonMalformedBenchmark {
		t.Fatalf("verdict = %+v, want malformed-benchmark", got)
	}
}

func TestParseAlternativesModeSkipsUnrequestedLabels(t *testing.T) {
	t.Parallel()

	input := rawAltInput + "BenchmarkEnhance/control-10 100 1 ns/op\n"

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdicts[0].Outcome != NewWins {
		t.Fatalf("outcome = %s, want %s", report.Verdicts[0].Outcome, NewWins)
	}
}

func TestParseAlternativesModeSortsMixedVerdicts(t *testing.T) {
	t.Parallel()

	input := `BenchmarkZ/original-10 100 10 ns/op
BenchmarkZ/enhanced-10 100 8 ns/op
BenchmarkZ/original-10 100 10 ns/op
BenchmarkZ/enhanced-10 100 8 ns/op
BenchmarkA/original-10 100 10 ns/op
BenchmarkA/original-10 100 10 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode})
	if err != nil {
		t.Fatal(err)
	}

	if report.Verdicts[0].Benchmark != "BenchmarkA" || report.Verdicts[1].Benchmark != "BenchmarkZ" {
		t.Fatalf("verdicts = %+v, want sorted mixed verdicts", report.Verdicts)
	}
}

func TestParseAlternativesModeVariableSamplesUsePValueApproximation(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
BenchmarkEnhance/original-10 100 12 ns/op
BenchmarkEnhance/enhanced-10 100 9 ns/op
`

	report, err := Parse(strings.NewReader(input), Options{Mode: altMode, Alpha: 1})
	if err != nil {
		t.Fatal(err)
	}

	got := report.Verdicts[0].Metrics[0].PValue
	if got <= 0 || got >= 1 {
		t.Fatalf("p-value = %f, want normal approximation between 0 and 1", got)
	}
}

func TestPrivateAlternativeBranches(t *testing.T) {
	t.Parallel()

	if got := trimCPUSuffix("BenchmarkFoo/original"); got != "BenchmarkFoo/original" {
		t.Fatalf("name = %q, want no trim", got)
	}

	if got := trimCPUSuffix("BenchmarkFoo/original-fast"); got != "BenchmarkFoo/original-fast" {
		t.Fatalf("name = %q, want no trim for non-numeric suffix", got)
	}

	if _, _, ok := splitRawBenchmarkName("BenchmarkFoo-10"); ok {
		t.Fatal("benchmark without sub-benchmark should not split")
	}

	if metric, ok := normalizeRawMetric("MB/s"); ok || metric != "" {
		t.Fatalf("metric = %q, ok = %v; want unsupported", metric, ok)
	}
}

func TestPrivateAlternativeMathBranches(t *testing.T) {
	t.Parallel()

	metrics := parseRawMetrics([]string{"bad", metricNanosecondsPerOp, "10", metricNanosecondsPerOp})
	if metrics[metricSecPerOp] != 10 {
		t.Fatalf("metrics = %+v, want valid metric after bad value", metrics)
	}

	if got := variance([]float64{1}, 1); got != 0 {
		t.Fatalf("variance = %f, want 0 for one sample", got)
	}

	if got := deltaPercent(0, 10); got != 0 {
		t.Fatalf("delta = %f, want 0 for zero baseline", got)
	}
}

func TestPrivateAlternativeEmptyReportBranches(t *testing.T) {
	t.Parallel()

	insufficientState := alternativeParseState{hasInsufficientRows: true}
	if got := insufficientState.emptyAlternativeReport(); got.Verdicts[0].ReasonCode != reasonInsufficient {
		t.Fatalf("report = %+v, want insufficient-samples", got)
	}

	emptyState := alternativeParseState{}
	if got := emptyState.emptyAlternativeReport(); got.Verdicts[0].ReasonCode != reasonMalformedBenchmark {
		t.Fatalf("report = %+v, want malformed-benchmark", got)
	}
}

func TestPrivateTextLabelBranches(t *testing.T) {
	t.Parallel()

	textState := textParseState{baselineLabel: "already", candidateLabel: "set"}
	textState.captureLabels("│ old.txt │ new.txt │")

	if textState.baselineLabel != "already" || textState.candidateLabel != "set" {
		t.Fatalf("labels = %q/%q, want unchanged", textState.baselineLabel, textState.candidateLabel)
	}

	emptyTextState := textParseState{}
	emptyTextState.captureLabels("│ sec/op │ sec/op vs base │")

	if emptyTextState.baselineLabel != "" || emptyTextState.candidateLabel != "" {
		t.Fatalf("metric labels = %q/%q, want empty", emptyTextState.baselineLabel, emptyTextState.candidateLabel)
	}

	if _, ok := parseBenchstatTextLabels("│ sec/op │ sec/op vs base │"); ok {
		t.Fatal("metric header should not parse as labels")
	}
}

func TestPrivateCSVLabelBranches(t *testing.T) {
	t.Parallel()

	csvState := csvParseState{}
	csvState.captureLabels([]string{"", "sec/op", "CI", "sec/op", "CI", "vs base", "P"})
	csvState.captureLabels([]string{"", "", "", labelNewTxt})

	if csvState.baselineLabel != "" || csvState.candidateLabel != "" {
		t.Fatalf("csv labels = %q/%q, want empty", csvState.baselineLabel, csvState.candidateLabel)
	}

	if got := csvState.displayBaselineLabel(); got != labelOld {
		t.Fatalf("baseline label = %q, want old", got)
	}

	if got := csvState.displayCandidateLabel(); got != labelNew {
		t.Fatalf("candidate label = %q, want new", got)
	}
}

func TestPrivateDisplayLabelBranches(t *testing.T) {
	t.Parallel()

	if got := displayLabel(""); got != "" {
		t.Fatalf("empty label = %q, want empty", got)
	}

	if got := displayLabel("."); got != "." {
		t.Fatalf("dot label = %q, want dot", got)
	}

	baselineLabel, candidateLabel := comparisonLabels([]Comparison{{}})
	if baselineLabel != labelOld || candidateLabel != labelNew {
		t.Fatalf("blank comparison labels = %q/%q, want old/new", baselineLabel, candidateLabel)
	}

	baselineLabel, candidateLabel = comparisonLabels(nil)
	if baselineLabel != labelOld || candidateLabel != labelNew {
		t.Fatalf("empty comparison labels = %q/%q, want old/new", baselineLabel, candidateLabel)
	}

	if got := winnerLabel(Outcome("unknown"), labelOld, labelNew); got != "" {
		t.Fatalf("unknown outcome winner = %q, want empty", got)
	}
}

func TestWriteVerboseTextHeaderErrorContainsContext(t *testing.T) {
	t.Parallel()

	report := Report{
		Verdicts: []BenchmarkVerdict{{Benchmark: benchmarkFoo, Outcome: Tie, Reason: reasonSame}},
	}

	err := report.WriteVerboseText(failingWriter{})
	if err == nil {
		t.Fatal("expected header write error")
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
