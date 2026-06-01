//nolint:exhaustruct // Tests intentionally use partial struct literals.
package benchstat

import (
	"strings"
	"testing"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser"
	"github.com/stretchr/testify/require"
)

const (
	labelNew     = "new"
	longLineSize = 70 * 1024
	rawBad       = "bad"
	testFoo      = "Foo"
)

func TestParseCSV(t *testing.T) {
	t.Parallel()

	input := `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,1.0e-08,1%,8.0e-09,1%,-20.00%,p=0.001 n=10
`

	result, err := Parse(input)
	require.NoError(t, err)
	require.Equal(t, []benchparser.Comparison{{
		Benchmark:      "Foo-8",
		Metric:         benchparser.MetricSecPerOp,
		DeltaPct:       -20,
		PValue:         0.001,
		BaselineLabel:  "old.txt",
		CandidateLabel: "new.txt",
		ApproxEqual:    false,
	}}, result.Comparisons)
}

func TestParseTextMissingPValue(t *testing.T) {
	t.Parallel()

	input := `               │ old.txt │ new.txt │
               │ sec/op  │ sec/op  vs base │
Foo-8          1.00n      1.10n
`

	result, err := Parse(input)
	require.NoError(t, err)
	require.Equal(t, ReasonMissingPValue, result.InconclusiveReason)
}

func TestParseCSVBranches(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		input      string
		wantReason string
		wantError  bool
	}{
		{
			name: "benchmark mismatch",
			input: `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,,1%,,1%,?,?
`,
			wantReason: ReasonBenchmarkSetMismatch,
		},
		{
			name: "missing p-value",
			input: `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,1,1%,2,1%,+100%,?
`,
			wantReason: ReasonMissingPValue,
		},
		{
			name: "only headers",
			input: `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
`,
			wantError: true,
		},
		{
			name:      "malformed CSV",
			input:     ",vs base,P\n\"Foo",
			wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := Parse(test.input)
			if test.wantError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.wantReason, result.InconclusiveReason)
		})
	}
}

func TestParseTextBranches(t *testing.T) {
	t.Parallel()

	valid := `goos: darwin
goarch: arm64
pkg: example.com/foo
cpu: Apple M4
name          old time/op  new time/op  delta
Foo-8         10.0ns       8.0ns        -20.00% (p=0.001 n=10+10)
Bar-8         10.0ns       10.0ns          ~     (p=1.000 n=10+10)
`
	result, err := Parse(valid)
	require.NoError(t, err)
	require.Len(t, result.Comparisons, 2)
	require.True(t, result.Comparisons[1].ApproxEqual)

	_, err = Parse("")
	require.ErrorIs(t, err, ErrNoComparisonRows)

	_, err = Parse("name old time/op new time/op delta\nFoo-8 1ns 2ns changed (p=0.1 n=1+1)\n")
	require.ErrorIs(t, err, ErrNoComparisonRows)

	_, err = Parse("name old time/op new time/op delta\nFoo-8 1ns 2ns +1.0% (p=n/a n=1+1)\n")
	require.ErrorIs(t, err, ErrNoComparisonRows)

	result, err = Parse("│ old │ new │\n│ sec/op │ sec/op vs base │\nFoo-8 1ns 2ns\n")
	require.NoError(t, err)
	require.Equal(t, ReasonMissingPValue, result.InconclusiveReason)

	result, err = Parse("│ old │ new │\nbenchmark set differs\n")
	require.NoError(t, err)
	require.Equal(t, ReasonBenchmarkSetMismatch, result.InconclusiveReason)

	_, err = Parse(strings.Repeat("x", longLineSize))
	require.Error(t, err)
}

func TestCSVPrivateBranches(t *testing.T) {
	t.Parallel()

	state := newCSVParseState()
	state.handleRecord(nil)
	state.updateBenchmarkSetMismatch([]string{testFoo})
	require.False(t, state.hasBenchmarkSetMismatch)

	state.metric = benchparser.MetricSecPerOp
	state.pValueIndex = 1

	_, ok := state.parseComparison([]string{testFoo, "0.001"})
	require.False(t, ok)

	state.deltaIndex = 1
	state.pValueIndex = 2
	_, ok = state.parseComparison([]string{testFoo, rawBad, "0.001"})
	require.False(t, ok)
	_, ok = state.parseComparison([]string{testFoo, "-1%", rawBad})
	require.False(t, ok)

	state.captureLabels([]string{testFoo, "old", "", labelNew})
	state.captureLabels([]string{"", "", "", labelNew})
	state.captureLabels([]string{"", "sec/op", "", "sec/op", "", "vs base", "P"})
	require.Empty(t, state.baselineLabel)

	require.Equal(t, -1, findFieldIndex([]string{"foo"}, "bar"))

	for _, raw := range []string{"", "~", "?", rawBad} {
		_, ok = parseDeltaPercent(raw)
		require.False(t, ok)
	}

	_, ok = parsePValue("p=n/a")
	require.False(t, ok)
}

func TestTextPrivateBranches(t *testing.T) {
	t.Parallel()

	state := textParseState{baselineLabel: "old", candidateLabel: labelNew}
	state.captureLabels("│ ignored │ ignored │")
	require.Equal(t, "old", state.baselineLabel)

	emptyState := textParseState{}
	emptyState.captureLabels("│ sec/op │ sec/op vs base │")
	require.Empty(t, emptyState.baselineLabel)

	_, ok := parseTextLabels("│")
	require.False(t, ok)
	_, ok = parseTextLabels("│ sec/op │ sec/op vs base │")
	require.False(t, ok)
	require.False(t, looksLikeComparisonLine("Foo"))

	for _, test := range []struct {
		line   string
		metric string
	}{
		{line: "Foo-8 1ns 2ns -1.0%", metric: benchparser.MetricSecPerOp},
		{line: "Foo-8 1ns 2ns -1.0% (p=n/a)", metric: benchparser.MetricSecPerOp},
		{line: "Foo-8 1ns 2ns changed (p=0.1)", metric: benchparser.MetricSecPerOp},
		{line: "Foo-8 1ns 2ns -1.0% (p=0.1)", metric: ""},
	} {
		_, ok = parseComparisonLine(test.line, test.metric)
		require.False(t, ok)
	}
}
