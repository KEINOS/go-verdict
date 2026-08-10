package complexity

import (
	"go/ast"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	sampleImportPath = "example.com/project/sample"
	sampleSimple     = sampleImportPath + ".Simple"
	samplePointer    = sampleImportPath + ".(*Worker).Run"
	sampleValue      = sampleImportPath + ".Worker.Name"
	sampleGeneric    = sampleImportPath + ".Map"
	sampleBoxed      = sampleImportPath + ".Box.Get"
	sampleLiteral    = sampleImportPath + ".Handler"
)

func TestAnalyzeNamesFunctionsLikePprof(t *testing.T) {
	t.Parallel()

	index := analyzeSample(t)

	require.Contains(t, index, sampleSimple)
	require.Contains(t, index, samplePointer, "pointer methods must match the pprof symbol format")
	require.Contains(t, index, sampleValue)
	require.Contains(t, index, sampleGeneric)
	require.Contains(t, index, sampleBoxed, "a generic receiver drops its type parameters")
	require.NotContains(t, index, sampleLiteral,
		"the compiler names a package-level literal after the initializer, so no pprof row can match it")
}

func TestAnalyzeScoresBothComplexities(t *testing.T) {
	t.Parallel()

	index := analyzeSample(t)

	simple := index[sampleSimple]
	require.Equal(t, 1, simple.Cyclomatic, "a straight-line function has the base cyclomatic score")
	require.Equal(t, 0, simple.Cognitive)

	branchy := index[samplePointer]
	require.GreaterOrEqual(t, branchy.Cyclomatic, 10)
	require.GreaterOrEqual(t, branchy.Cognitive, 10)
}

func TestAnalyzeRecordsSourcePosition(t *testing.T) {
	t.Parallel()

	index := analyzeSample(t)

	for symbol, stat := range index {
		require.Equal(t, sampleImportPath, stat.ImportPath, symbol)
		require.Equal(t, "sample.go", stat.File, symbol)
		require.Positive(t, stat.Line, symbol)
	}
}

func TestAnalyzeSortsBySymbol(t *testing.T) {
	t.Parallel()

	stats, err := Analyze([]Package{samplePackage()})
	require.NoError(t, err)
	require.Len(t, stats, 8)
	require.Equal(t, samplePointer, stats[0].Symbol, "symbols are sorted for a stable report")
	require.True(t, slices.IsSortedFunc(stats, func(left Stat, right Stat) int {
		return strings.Compare(left.Symbol, right.Symbol)
	}))
}

func TestAnalyzeReportsParseErrors(t *testing.T) {
	t.Parallel()

	_, err := Analyze([]Package{{
		ImportPath: "example.com/project/broken",
		Dir:        filepath.Join("testdata", "broken"),
		Files:      []string{"broken.go"},
	}})
	require.ErrorContains(t, err, "broken.go")
}

func TestAnalyzeAcceptsPackagesWithoutFunctions(t *testing.T) {
	t.Parallel()

	stats, err := Analyze(nil)
	require.NoError(t, err)
	require.Empty(t, stats)

	stats, err = Analyze([]Package{{
		ImportPath: "example.com/project/empty",
		Dir:        filepath.Join("testdata", "sample"),
		Files:      nil,
	}})
	require.NoError(t, err)
	require.Empty(t, stats)
}

func samplePackage() Package {
	return Package{
		ImportPath: sampleImportPath,
		Dir:        filepath.Join("testdata", "sample"),
		Files:      []string{"sample.go"},
	}
}

func analyzeSample(t *testing.T) map[string]Stat {
	t.Helper()

	stats, err := Analyze([]Package{samplePackage()})
	require.NoError(t, err)

	index := make(map[string]Stat, len(stats))
	for _, stat := range stats {
		index[stat.Symbol] = stat
	}

	return index
}

func TestAnalyzeNamesAMultiParameterGenericReceiver(t *testing.T) {
	t.Parallel()

	require.Contains(t, analyzeSample(t), sampleImportPath+".Pair.Key")
}

func TestAnalyzeKeepsInitDeclarationsSeparate(t *testing.T) {
	t.Parallel()

	stats := analyzeSample(t)
	first, firstOK := stats[sampleImportPath+".init.0"]
	second, secondOK := stats[sampleImportPath+".init.1"]

	require.True(t, firstOK, "each init declaration has its own compiler symbol")
	require.True(t, secondOK, "each init declaration has its own compiler symbol")
	require.Equal(t, 98, first.Line)
	require.Equal(t, 1, first.Cyclomatic)
	require.Equal(t, 102, second.Line)
	require.Equal(t, 2, second.Cyclomatic)
}

func TestReceiverNameFallsBackForAnUnnameableType(t *testing.T) {
	t.Parallel()

	require.Equal(t, unknownReceiver, receiverName(&ast.BadExpr{From: 0, To: 0}))
}
