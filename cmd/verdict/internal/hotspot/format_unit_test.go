package hotspot

// This file covers the report formatting helpers, which are pure functions of
// one Result.

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendCaveatJoinsSentences(t *testing.T) {
	t.Parallel()

	require.Equal(t, "second.", appendCaveat("", "second."))
	require.Equal(t, "first. second.", appendCaveat("first.", "second."))
}

func TestWithCaveatOnlyAddsALineWhenThereIsOne(t *testing.T) {
	t.Parallel()

	require.Equal(t, "text\n", withCaveat("text\n", ""))
	require.Equal(t, "text\nCaveat: why\n", withCaveat("text\n", "why"))
}

func TestSourcePositionNeedsBothHalves(t *testing.T) {
	t.Parallel()

	require.Empty(t, sourcePosition("", 12))
	require.Empty(t, sourcePosition(testSampleFile, 0))
	require.Empty(t, sourcePosition(testSampleFile, -1))
	require.Equal(t, " at sample.go:12", sourcePosition(testSampleFile, 12))
}

func TestFormatTextCoversEveryReportShape(t *testing.T) {
	t.Parallel()

	noBenchmark := testResult()
	noBenchmark.Classification = classNoBenchmark
	noBenchmark.Caveat = caveatNoBenchmark
	require.Contains(t, formatText(noBenchmark), "no benchmark workload ran")
	require.Contains(t, formatText(noBenchmark), "Caveat:")

	noClear := testResult()
	require.Contains(t, formatText(noClear), "no clear user-code hotspot")

	suggestion := testResult()
	suggestion.Classification = classHotAndComplex
	suggestion.Function = testWorkFunc
	suggestion.File = testSampleFile
	suggestion.Line = 12
	suggestion.CPU = Metric{Unit: unitMS, Flat: 10, FlatPct: 20, Cum: 30, CumPct: 40}
	suggestion.AllocBytes = Metric{Unit: unitBytes, Flat: 1, FlatPct: 6, Cum: 2, CumPct: 7}
	suggestion.AllocObjects = Metric{Unit: unitObjects, Flat: 3, FlatPct: 8, Cum: 4, CumPct: 9}
	suggestion.Retained = Metric{Unit: unitBytes, Flat: 5, FlatPct: 11, Cum: 6, CumPct: 12}
	suggestion.Complexity = Complexity{Cyclomatic: 24, Cognitive: 31}

	text := formatText(suggestion)
	require.Contains(t, text, "inspect "+testWorkFunc+" at sample.go:12")
	require.Contains(t, text, "cpu flat 20.0%")
	require.Contains(t, text, "alloc bytes flat 6.0%")
	require.Contains(t, text, "alloc objects flat 8.0%")
	require.Contains(t, text, "retained flat 11.0%")
	require.Contains(t, text, "cyclomatic 24, cognitive 31")
	require.NotContains(t, text, "Also:", "there are no runners-up to list")
}

func TestFormatTextListsRunnersUp(t *testing.T) {
	t.Parallel()

	result := testResult()
	result.Classification = classCPUHotspot
	result.Function = testWorkFunc
	result.Candidates = []Choice{
		{
			Classification: classAllocHotspot,
			Function:       testAllocFunc,
			File:           testSampleFile,
			Line:           7,
			Signals:        []string{signalAllocBytes},
			CPU:            zeroMetric(unitMS),
			AllocBytes:     zeroMetric(unitBytes),
			AllocObjects:   zeroMetric(unitObjects),
			Retained:       zeroMetric(unitBytes),
			Complexity:     Complexity{Cyclomatic: 0, Cognitive: 0},
		},
	}

	require.Contains(t, formatText(result), "Also: "+testAllocFunc+" at sample.go:7 (alloc-hotspot)")
}
