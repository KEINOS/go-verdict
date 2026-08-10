package hotspot

// This file covers candidate fusion and the Pareto ranking that decides which
// function the report suggests first.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/complexity"
)

func TestClassifyPrefersHotAndComplex(t *testing.T) {
	t.Parallel()

	got := classify(testResult(), profileSet{
		CPU: map[string]pprofRow{
			testWorkFunc:  row(testWorkFunc, 20, 30),
			testMixedFunc: row(testMixedFunc, 40, 60),
		},
		Alloc:        map[string]pprofRow{},
		AllocObjects: map[string]pprofRow{},
		Inuse:        map[string]pprofRow{},
	}, map[string]complexity.Stat{
		testWorkFunc: statOf(testWorkFunc, 24, 31),
	}, defaultTop)

	require.Equal(t, classHotAndComplex, got.Classification)
	require.Equal(t, testWorkFunc, got.Function,
		"a function that is hot and complex outranks one that is only hotter")
	require.Equal(t, []string{"cpu", "complexity"}, got.Signals)
	require.Len(t, got.Candidates, 1)
	require.Equal(t, testMixedFunc, got.Candidates[0].Function)
}

func TestClassifyNamesEachMemorySignal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		profiles profileSet
		name     string
		want     string
	}{
		{
			name:     "allocated bytes",
			want:     classAllocHotspot,
			profiles: memoryProfiles(rowsOf(testAllocFunc, 30, 40), zeroRows(), zeroRows()),
		},
		{
			name:     "allocation count",
			want:     classAllocRateHotspot,
			profiles: memoryProfiles(zeroRows(), rowsOf(testAllocFunc, 30, 40), zeroRows()),
		},
		{
			name:     "retained heap",
			want:     classRetentionHotspot,
			profiles: memoryProfiles(zeroRows(), zeroRows(), rowsOf(testRetainFunc, 30, 40)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := classify(testResult(), test.profiles, nil, defaultTop)
			require.Equal(t, test.want, got.Classification)
		})
	}
}

func TestClassifyKeepsParetoOptimalCandidatesFirst(t *testing.T) {
	t.Parallel()

	// Beta is worse than Alfa on every signal, so Alfa dominates it and drops
	// behind the front. Gamma wins on allocations alone, so nothing dominates
	// it, and its stronger single signal puts it first.
	got := classify(testResult(), profileSet{
		CPU: map[string]pprofRow{
			testImportPath + ".Alfa": row(testImportPath+".Alfa", 40, 60),
			testImportPath + ".Beta": row(testImportPath+".Beta", 20, 30),
		},
		Alloc: map[string]pprofRow{
			testImportPath + ".Gamma": row(testImportPath+".Gamma", 50, 70),
		},
		AllocObjects: map[string]pprofRow{},
		Inuse:        map[string]pprofRow{},
	}, nil, defaultTop)

	require.Equal(t, testImportPath+".Gamma", got.Function)
	require.Len(t, got.Candidates, 2)
	require.Equal(t, testImportPath+".Alfa", got.Candidates[0].Function)
	require.Equal(t, testImportPath+".Beta", got.Candidates[1].Function,
		"a dominated candidate ranks below every candidate in the Pareto front")
}

func TestClassifyRanksMeasuredCostAboveStaticEstimate(t *testing.T) {
	t.Parallel()

	got := classify(testResult(), profileSet{
		CPU:          map[string]pprofRow{testWorkFunc: row(testWorkFunc, 20, 30)},
		Alloc:        map[string]pprofRow{},
		AllocObjects: map[string]pprofRow{},
		Inuse:        map[string]pprofRow{},
	}, map[string]complexity.Stat{
		testAllocFunc: statOf(testAllocFunc, 90, 90),
	}, defaultTop)

	require.Equal(t, testWorkFunc, got.Function, "a measured hotspot outranks a static estimate")
	require.Equal(t, classCPUHotspot, got.Classification)
	require.Empty(t, got.Caveat, "a measured suggestion needs no static-estimate caveat")
	require.Len(t, got.Candidates, 1)
	require.Equal(t, testAllocFunc, got.Candidates[0].Function)
}

func TestClassifyCapsTheCandidateList(t *testing.T) {
	t.Parallel()

	profiles := profileSet{
		CPU: map[string]pprofRow{
			testImportPath + ".Alfa":  row(testImportPath+".Alfa", 40, 60),
			testImportPath + ".Beta":  row(testImportPath+".Beta", 30, 50),
			testImportPath + ".Gamma": row(testImportPath+".Gamma", 20, 40),
			testImportPath + ".Delta": row(testImportPath+".Delta", 10, 30),
		},
		Alloc:        map[string]pprofRow{},
		AllocObjects: map[string]pprofRow{},
		Inuse:        map[string]pprofRow{},
	}

	require.Len(t, classify(testResult(), profiles, nil, defaultTop).Candidates, defaultTop-1)
	require.Empty(t, classify(testResult(), profiles, nil, 1).Candidates)
	require.Len(t, classify(testResult(), profiles, nil, 99).Candidates, 3)
}

func TestClassifyReportsNoClearHotspot(t *testing.T) {
	t.Parallel()

	got := classify(testResult(), emptyProfiles(), nil, defaultTop)

	require.Equal(t, classNoClearHotspot, got.Classification)
	require.Contains(t, got.Caveat, "The cost may be spread")
	require.Empty(t, got.Function)
}

func TestClassifyScoresComplexityForEveryMeasuredPackage(t *testing.T) {
	t.Parallel()

	hot := "example.com/project/other.Helper"
	cold := "example.com/project/other.Untouched"

	got := classify(testResult(), profileSet{
		CPU:          map[string]pprofRow{hot: row(hot, 40, 60)},
		Alloc:        map[string]pprofRow{},
		AllocObjects: map[string]pprofRow{},
		Inuse:        map[string]pprofRow{},
	}, map[string]complexity.Stat{
		hot:  statOf(hot, 40, 40),
		cold: statOf(cold, 99, 99),
	}, defaultTop)

	require.Equal(t, hot, got.Function)
	require.Equal(t, testSampleFile, got.File, "a hot function outside the package still gets its position")
	require.Equal(t, 40, got.Complexity.Cyclomatic)
	require.Equal(t, classHotAndComplex, got.Classification,
		"the profiles already proved this function hot, so its complexity is real evidence")
	require.Empty(t, got.Candidates,
		"complex but cold code outside the target package is never a candidate")
}

func TestStaticKeyJoinsGenericAndClosureRows(t *testing.T) {
	t.Parallel()

	got := classify(testResult(), profileSet{
		CPU:          map[string]pprofRow{testWorkFunc + ".func1": row(testWorkFunc+".func1", 40, 60)},
		Alloc:        map[string]pprofRow{},
		AllocObjects: map[string]pprofRow{},
		Inuse:        map[string]pprofRow{},
	}, map[string]complexity.Stat{
		testWorkFunc: statOf(testWorkFunc, 24, 31),
	}, defaultTop)

	require.Equal(t, classHotAndComplex, got.Classification,
		"a closure carries the complexity of the function that declares it")
	require.Equal(t, testSampleFile, got.File)
}

func TestClassifyReportsAGenericFunctionOnce(t *testing.T) {
	t.Parallel()

	generic := testImportPath + ".Map"

	got := classify(testResult(), profileSet{
		CPU:          map[string]pprofRow{generic + "[go.shape.int]": row(generic+"[go.shape.int]", 40, 60)},
		Alloc:        map[string]pprofRow{},
		AllocObjects: map[string]pprofRow{},
		Inuse:        map[string]pprofRow{},
	}, map[string]complexity.Stat{
		generic: statOf(generic, 24, 31),
	}, defaultTop)

	require.Equal(t, classHotAndComplex, got.Classification)

	for _, choice := range got.Candidates {
		require.NotEqual(t, generic, choice.Function,
			"the instantiated and the declared symbol are the same function")
	}
}

func TestClassifyOrdersDominatorsBeforeWhatTheyDominate(t *testing.T) {
	t.Parallel()

	// Charlie dominates both others. Bravo dominates Alfa, but the two tie on
	// signal count and top score, so only dominance can order them, and Alfa
	// would win a name comparison.
	got := classify(testResult(), profileSet{
		CPU: map[string]pprofRow{
			testImportPath + ".Charlie": row(testImportPath+".Charlie", 40, 60),
			testImportPath + ".Bravo":   row(testImportPath+".Bravo", 20, 20),
			testImportPath + ".Alfa":    row(testImportPath+".Alfa", 20, 20),
		},
		Alloc: map[string]pprofRow{
			testImportPath + ".Charlie": row(testImportPath+".Charlie", 50, 70),
			testImportPath + ".Bravo":   row(testImportPath+".Bravo", 20, 20),
			testImportPath + ".Alfa":    row(testImportPath+".Alfa", 15, 15),
		},
		AllocObjects: map[string]pprofRow{},
		Inuse:        map[string]pprofRow{},
	}, nil, 99)

	require.Equal(t, testImportPath+".Charlie", got.Function)
	require.Len(t, got.Candidates, 2)
	require.Equal(t, testImportPath+".Bravo", got.Candidates[0].Function,
		"a candidate must never rank behind one it dominates")
	require.Equal(t, testImportPath+".Alfa", got.Candidates[1].Function)
}

func row(function string, flatPct float64, cumPct float64) pprofRow {
	return pprofRow{Function: function, Flat: flatPct, FlatPct: flatPct, Cum: cumPct, CumPct: cumPct}
}

func rowsOf(function string, flatPct float64, cumPct float64) map[string]pprofRow {
	return map[string]pprofRow{function: row(function, flatPct, cumPct)}
}

func zeroRows() map[string]pprofRow {
	return map[string]pprofRow{}
}

func memoryProfiles(bytes map[string]pprofRow, objects map[string]pprofRow, retained map[string]pprofRow) profileSet {
	return profileSet{
		CPU:          map[string]pprofRow{},
		Alloc:        bytes,
		AllocObjects: objects,
		Inuse:        retained,
	}
}
