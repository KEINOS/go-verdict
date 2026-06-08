package benchstat

import (
	"errors"
	"strings"
	"testing"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser"
	"github.com/stretchr/testify/require"
)

const maxBenchstatFuzzInputSize = 80 * 1024

const benchstatTextSeed = `goos: darwin
goarch: arm64
pkg: example.com/foo
cpu: Apple M4
name          old time/op  new time/op  delta
Foo-8         10.0ns       8.0ns        -20.00% (p=0.001 n=10+10)
`

const benchstatCSVSeed = `,old.txt,,new.txt,,,
,sec/op,CI,sec/op,CI,vs base,P
Foo-8,1.0e-08,1%,8.0e-09,1%,-20.00%,p=0.001 n=10
`

const benchstatMissingPTextSeed = `               │ old.txt │ new.txt │
               │ sec/op  │ sec/op  vs base │
Foo-8          1.00n      1.10n
`

const benchstatMismatchTextSeed = `│ old │ new │
benchmark set differs
`

const (
	benchstatMalformedCSVSeed = ",vs base,P\n\"Foo"
	benchstatCSVHeuristicTextSeed = "plain text that mentions ,vs base,P but has no rows"
	benchstatCSVWithoutMarkerSeed = ",old.txt,,new.txt,,,\n,sec/op,CI,sec/op,CI,delta,P\n"
)

func FuzzParse(f *testing.F) {
	f.Add(benchstatTextSeed)
	f.Add(benchstatCSVSeed)
	f.Add(benchstatMissingPTextSeed)
	f.Add(benchstatMismatchTextSeed)
	f.Add(benchstatMalformedCSVSeed)
	f.Add(benchstatCSVHeuristicTextSeed)
	f.Add(benchstatCSVWithoutMarkerSeed)
	f.Add("")
	f.Add(strings.Repeat("x", longLineSize))

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxBenchstatFuzzInputSize {
			t.Skip("keeping parser fuzz input bounded")
		}

		result, err := Parse(input)
		assertBenchstatSeed(t, input, result, err)
	})
}

func assertBenchstatSeed(t *testing.T, input string, result Result, err error) {
	t.Helper()

	switch input {
	case benchstatTextSeed:
		require.NoError(t, err)
		require.Len(t, result.Comparisons, 1)
		require.Equal(t, benchparser.Comparison{
			Benchmark:      testFoo8,
			Metric:         benchparser.MetricSecPerOp,
			DeltaPct:       -20,
			PValue:         0.001,
			BaselineLabel:  "",
			CandidateLabel: "",
			ApproxEqual:    false,
		}, result.Comparisons[0])
	case benchstatCSVSeed:
		require.NoError(t, err)
		require.Len(t, result.Comparisons, 1)
		require.Equal(t, testFoo8, result.Comparisons[0].Benchmark)
		require.Equal(t, "old.txt", result.Comparisons[0].BaselineLabel)
		require.Equal(t, "new.txt", result.Comparisons[0].CandidateLabel)
		require.Equal(t, benchparser.MetricSecPerOp, result.Comparisons[0].Metric)
		require.InDelta(t, -20.0, result.Comparisons[0].DeltaPct, 0.000001)
		require.InDelta(t, 0.001, result.Comparisons[0].PValue, 0.000001)
		require.False(t, result.Comparisons[0].ApproxEqual)
	case benchstatMissingPTextSeed:
		require.NoError(t, err)
		require.Equal(t, ReasonMissingPValue, result.InconclusiveReason)
	case benchstatMismatchTextSeed:
		require.NoError(t, err)
		require.Equal(t, ReasonBenchmarkSetMismatch, result.InconclusiveReason)
	case benchstatMalformedCSVSeed:
		require.ErrorContains(t, err, errReadingCSVInput.Error())
	case benchstatCSVHeuristicTextSeed, benchstatCSVWithoutMarkerSeed, "":
		require.ErrorIs(t, err, ErrNoComparisonRows)
	case strings.Repeat("x", longLineSize):
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrNoComparisonRows) || strings.Contains(err.Error(), errScanningTextInput.Error()))
	}
}
