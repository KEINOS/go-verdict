package rawbench

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser"
	"github.com/stretchr/testify/require"
)

const longLineSize = 70 * 1024

var errTestRead = errors.New("test read error")

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, errTestRead
}

var _ io.Reader = failingReader{}

func TestParseAlternatives(t *testing.T) {
	t.Parallel()

	result, err := ParseAlternatives("BenchmarkFoo/original-10 100 10 ns/op 1 MB/s\n")
	require.NoError(t, err)
	require.True(t, result.HasBenchmarkRows)
	require.Equal(t, Samples{
		"BenchmarkFoo": {
			"original": {
				benchparser.MetricSecPerOp: {10},
			},
		},
	}, result.Samples)
}

func TestParseFile(t *testing.T) {
	t.Parallel()

	result, err := ParseFile(strings.NewReader("BenchmarkFast-10 100 10 ns/op\n"))
	require.NoError(t, err)
	require.Equal(t, "BenchmarkFast", result.Name)
	require.Equal(t, map[string][]float64{benchparser.MetricSecPerOp: {10}}, result.Metrics)
}

func TestLooksLikeAlternatives(t *testing.T) {
	t.Parallel()

	require.False(t, LooksLikeAlternatives("PASS\nBenchmarkFoo 100 1 ns/op\n"))
	require.True(t, LooksLikeAlternatives("PASS\nBenchmarkFoo/original-10 100 1 ns/op\n"))
}

func TestParseAlternativesBranches(t *testing.T) {
	t.Parallel()

	result, err := ParseAlternatives("PASS\nBenchmarkFoo/original-10 100\nBenchmarkFoo/enhanced-10 100 1 MB/s\n")
	require.NoError(t, err)
	require.True(t, result.HasMalformedRows)
	require.True(t, result.HasUnsupportedRows)

	_, err = ParseAlternatives("BenchmarkFoo/original-10 100 " + strings.Repeat("1", longLineSize) + " ns/op\n")
	require.Error(t, err)
}

func TestParseFileBranches(t *testing.T) {
	t.Parallel()

	result, err := ParseFile(strings.NewReader("PASS\nBenchmarkFast-10 100\nBenchmarkFast-10 100 1 MB/s\n"))
	require.NoError(t, err)
	require.True(t, result.HasMalformedRows)
	require.True(t, result.HasUnsupportedRows)

	result, err = ParseFile(strings.NewReader("BenchmarkFast-10 100 1 ns/op\nBenchmarkSlow-10 100 2 ns/op\n"))
	require.NoError(t, err)
	require.True(t, result.HasMultipleSeries)

	_, err = ParseFile(failingReader{})
	require.Error(t, err)

	_, err = ParseFile(strings.NewReader(strings.Repeat("x", longLineSize)))
	require.Error(t, err)
}

func TestDecodeLineBranches(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"BenchmarkFoo/original-10",
		"BenchmarkFoo/original-10 nope 10 ns/op",
		"BenchmarkFoo/original-10 100 10",
		"BenchmarkFoo/original-10 100 10 ns/op 1",
		"BenchmarkFoo/original-10 100 bad ns/op",
	} {
		_, ok := decodeLine(line)
		require.False(t, ok)
	}

	metrics, ok := parseMetrics([]string{"bad", "MB/s", "10", "ns/op"})
	require.True(t, ok)
	require.Equal(t, map[string]float64{benchparser.MetricSecPerOp: 10}, metrics)

	_, ok = parseMetrics([]string{"bad", "ns/op"})
	require.False(t, ok)
}

func TestNameBranches(t *testing.T) {
	t.Parallel()

	require.Equal(t, "BenchmarkFoo/original", trimCPUSuffix("BenchmarkFoo/original"))
	require.Equal(t, "BenchmarkFoo/original-fast", trimCPUSuffix("BenchmarkFoo/original-fast"))
	require.Equal(t, "BenchmarkFoo/original", trimCPUSuffix("BenchmarkFoo/original-10"))

	_, _, ok := splitName("BenchmarkFoo-10")
	require.False(t, ok)
	_, _, ok = splitName("/-10")
	require.False(t, ok)

	_, _, ok = parseFileLine("-10 100 1 ns/op")
	require.False(t, ok)
}
