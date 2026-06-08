package rawbench

import (
	"errors"
	"strings"
	"testing"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser"
	"github.com/stretchr/testify/require"
)

const maxRawFuzzInputSize = 80 * 1024

const (
	rawValidGoTestBenchSeed = "BenchmarkFoo/original-10 100 10 ns/op 1 MB/s\nBenchmarkFoo/enhanced-10 100 8 ns/op 1 MB/s\n"
	rawNestedGoTestBenchSeed = "BenchmarkFoo/group/original-10 100 12 ns/op\nBenchmarkFoo/group/enhanced-10 100 9 ns/op\n"
	rawMalformedSeed = "BenchmarkFoo/original-10 nope 10 ns/op\n"
	rawUnsupportedSeed = "BenchmarkFoo/original-10 100 1 MB/s\n"
	rawTopLevelSeed = "BenchmarkFoo-10 100 10 ns/op\n"
	rawNoiseSeed = "PASS\nnot a benchmark\ntext contains BenchmarkFoo/original-10 100 1 ns/op\n"
	rawOddMetricFieldSeed = "BenchmarkFoo/original-10 100 10 ns/op 1\n"
	rawFileMultipleSeriesSeed = "BenchmarkFast-10 100 1 ns/op\nBenchmarkSlow-10 100 2 ns/op\n"
	rawNoCPUSuffixSubBenchmarkSeed = "BenchmarkFoo/original-fast 100 10 ns/op\n"
	rawFileValidSeed = "BenchmarkFast-10 100 10 ns/op\n"
	rawFileNoCPUSuffixSeed = "BenchmarkFast 100 10 ns/op\n"
)

func FuzzParseGoTestBench(f *testing.F) {
	longGoTestBenchSeed := "BenchmarkFoo/original-10 100 " + strings.Repeat("1", longLineSize) + " ns/op\n"

	f.Add(rawValidGoTestBenchSeed)
	f.Add(rawNestedGoTestBenchSeed)
	f.Add(rawMalformedSeed)
	f.Add(rawUnsupportedSeed)
	f.Add(rawTopLevelSeed)
	f.Add(rawNoiseSeed)
	f.Add(rawOddMetricFieldSeed)
	f.Add(longGoTestBenchSeed)

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxRawFuzzInputSize {
			t.Skip("keeping parser fuzz input bounded")
		}

		result, err := ParseGoTestBench(input)
		if err != nil {
			require.ErrorIs(t, err, errScanningGoTestBench)

			return
		}

		if LooksLikeGoTestBench(input) {
			require.True(t, result.HasBenchmarkRows)
		}

		assertGoTestBenchSeed(t, input, result)
	})
}

func FuzzLooksLikeGoTestBench(f *testing.F) {
	f.Add(rawValidGoTestBenchSeed)
	f.Add(rawTopLevelSeed)
	f.Add(rawNoiseSeed)
	f.Add(rawNestedGoTestBenchSeed)
	f.Add(rawNoCPUSuffixSubBenchmarkSeed)

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxRawFuzzInputSize {
			t.Skip("keeping parser fuzz input bounded")
		}

		looksLike := LooksLikeGoTestBench(input)
		switch input {
		case rawValidGoTestBenchSeed, rawNestedGoTestBenchSeed, rawNoCPUSuffixSubBenchmarkSeed:
			require.True(t, looksLike)
		case rawTopLevelSeed, rawNoiseSeed:
			require.False(t, looksLike)
		}
	})
}

func FuzzParseFile(f *testing.F) {
	longFileSeed := "BenchmarkFast-10 100 " + strings.Repeat("1", longLineSize) + " ns/op\n"

	f.Add(rawFileValidSeed)
	f.Add(rawMalformedSeed)
	f.Add(rawUnsupportedSeed)
	f.Add(rawNoiseSeed)
	f.Add(rawFileMultipleSeriesSeed)
	f.Add(rawFileNoCPUSuffixSeed)
	f.Add(longFileSeed)

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxRawFuzzInputSize {
			t.Skip("keeping parser fuzz input bounded")
		}

		result, err := ParseFile(strings.NewReader(input))
		if err != nil {
			require.True(t, errors.Is(err, errScanningFile) || errors.Is(err, errReadingInput))

			return
		}

		assertRawFileSeed(t, input, result)
	})
}

func assertGoTestBenchSeed(t *testing.T, input string, result GoTestBench) {
	t.Helper()

	switch input {
	case rawValidGoTestBenchSeed:
		require.True(t, result.HasBenchmarkRows)
		require.False(t, result.HasMalformedRows)
		require.False(t, result.HasUnsupportedRows)
		require.Equal(t, []float64{10}, result.Samples["BenchmarkFoo"]["original"][benchparser.MetricSecPerOp])
		require.Equal(t, []float64{8}, result.Samples["BenchmarkFoo"]["enhanced"][benchparser.MetricSecPerOp])
	case rawNestedGoTestBenchSeed:
		require.True(t, result.HasBenchmarkRows)
		require.False(t, result.HasMalformedRows)
		require.False(t, result.HasUnsupportedRows)
		require.Equal(t, []float64{12}, result.Samples["BenchmarkFoo/group"]["original"][benchparser.MetricSecPerOp])
	case rawMalformedSeed, rawOddMetricFieldSeed, rawTopLevelSeed:
		require.True(t, result.HasBenchmarkRows)
		require.True(t, result.HasMalformedRows)
		require.False(t, result.HasUnsupportedRows)
	case rawUnsupportedSeed:
		require.True(t, result.HasBenchmarkRows)
		require.False(t, result.HasMalformedRows)
		require.True(t, result.HasUnsupportedRows)
	case rawNoiseSeed:
		require.False(t, result.HasBenchmarkRows)
		require.False(t, result.HasMalformedRows)
		require.False(t, result.HasUnsupportedRows)
	}
}

func assertRawFileSeed(t *testing.T, input string, result File) {
	t.Helper()

	switch input {
	case rawFileValidSeed:
		require.Equal(t, "BenchmarkFast", result.Name)
		require.True(t, result.HasBenchmarkRows)
		require.False(t, result.HasMalformedRows)
		require.False(t, result.HasUnsupportedRows)
		require.False(t, result.HasMultipleSeries)
		require.Equal(t, []float64{10}, result.Metrics[benchparser.MetricSecPerOp])
	case rawMalformedSeed:
		require.True(t, result.HasBenchmarkRows)
		require.True(t, result.HasMalformedRows)
		require.False(t, result.HasUnsupportedRows)
	case rawUnsupportedSeed:
		require.True(t, result.HasBenchmarkRows)
		require.False(t, result.HasMalformedRows)
		require.True(t, result.HasUnsupportedRows)
	case rawNoiseSeed:
		require.False(t, result.HasBenchmarkRows)
		require.False(t, result.HasMalformedRows)
		require.False(t, result.HasUnsupportedRows)
		require.False(t, result.HasMultipleSeries)
	case rawFileMultipleSeriesSeed:
		require.Equal(t, "BenchmarkFast", result.Name)
		require.True(t, result.HasBenchmarkRows)
		require.False(t, result.HasMalformedRows)
		require.False(t, result.HasUnsupportedRows)
		require.True(t, result.HasMultipleSeries)
		require.Equal(t, []float64{1}, result.Metrics[benchparser.MetricSecPerOp])
	case rawFileNoCPUSuffixSeed:
		require.Equal(t, "BenchmarkFast", result.Name)
		require.True(t, result.HasBenchmarkRows)
		require.False(t, result.HasMalformedRows)
		require.False(t, result.HasUnsupportedRows)
		require.False(t, result.HasMultipleSeries)
		require.Equal(t, []float64{10}, result.Metrics[benchparser.MetricSecPerOp])
	}
}
