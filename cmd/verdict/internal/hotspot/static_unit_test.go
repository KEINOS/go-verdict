package hotspot

// This file covers the static complexity signal: symbol joining, module-local
// package discovery, and the no-benchmark fallback.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/complexity"
)

func TestComplexityOnlyRanksAndScopesCandidates(t *testing.T) {
	t.Parallel()

	got := classify(testResult(), emptyProfiles(), map[string]complexity.Stat{
		testWorkFunc:                     statOf(testWorkFunc, 12, 20),
		testAllocFunc:                    statOf(testAllocFunc, 30, 16),
		testRetainFunc:                   statOf(testRetainFunc, 2, 3),
		"example.com/project/other.Huge": statOf("example.com/project/other.Huge", 99, 99),
	}, defaultTop)

	require.Equal(t, classComplexityHotspot, got.Classification)
	require.Equal(t, testAllocFunc, got.Function, "the highest normalized score wins")
	require.Contains(t, got.Caveat, "static estimate")

	for _, choice := range got.Candidates {
		require.NotEqual(t, testRetainFunc, choice.Function, "code below both thresholds is not a candidate")
		require.NotEqual(t, "example.com/project/other.Huge", choice.Function,
			"only the target package can qualify on complexity alone")
	}
}

func TestComplexityOnlyIgnoresAPackageWithASharedDotPrefix(t *testing.T) {
	t.Parallel()

	// A module may hold both "example.com/project/pkg" and a directory whose
	// name only starts with it, such as a versioned suffix. Prefix matching
	// would treat the second one as the target package.
	sibling := testImportPath + ".v2.Huge"

	base := testResult()
	got := classify(base, emptyProfiles(), map[string]complexity.Stat{
		sibling: statOf(sibling, 99, 99),
	}, defaultTop)

	require.Equal(t, classNoClearHotspot, got.Classification,
		"only the target package can qualify on complexity alone")
	require.Empty(t, got.Function)
}

func TestComplexityOnlyBreaksTiesBySymbol(t *testing.T) {
	t.Parallel()

	first := testImportPath + ".Alfa"
	second := testImportPath + ".Beta"

	got := classify(testResult(), emptyProfiles(), map[string]complexity.Stat{
		second: statOf(second, 20, 30),
		first:  statOf(first, 20, 30),
	}, defaultTop)

	require.Equal(t, first, got.Function)
}

func TestWithoutBenchmarkKeepsTheBootstrapAdviceWithoutStatic(t *testing.T) {
	t.Parallel()

	got := withoutBenchmark(testResult(), nil, defaultTop)
	require.Equal(t, classNoBenchmark, got.Classification)
	require.Contains(t, got.Caveat, "verdict help bootstrap")
	require.Empty(t, got.Function)
}

func TestWithoutBenchmarkFallsBackToStatic(t *testing.T) {
	t.Parallel()

	got := withoutBenchmark(testResult(), map[string]complexity.Stat{
		testWorkFunc: statOf(testWorkFunc, 12, 20),
	}, defaultTop)
	require.Equal(t, classComplexityHotspot, got.Classification)
	require.Equal(t, testWorkFunc, got.Function)
	require.Contains(t, got.Caveat, "static estimate")
	require.Contains(t, got.Caveat, "verdict help bootstrap")
}

func TestStaticComplexityReportsFailures(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{{out: nil, err: errFakeRun}}

	_, err := newCommand(t, runner).resolveModulePackages(testPkgArg, testModulePath)
	require.ErrorContains(t, err, "listing module packages")

	runner = newFakeRunner()
	runner.outputs = []fakeOutput{{out: []byte("{"), err: nil}}

	_, err = newCommand(t, runner).resolveModulePackages(testPkgArg, testModulePath)
	require.ErrorContains(t, err, "decoding go list output")

	runner = newFakeRunner()
	runner.outputs = []fakeOutput{{out: goListBrokenJSON(), err: nil}}

	_, err = newCommand(t, runner).staticComplexity(testPkgArg, samplePackageInfo())
	require.ErrorContains(t, err, "analyzing complexity")
}

// statOf builds a static result for a symbol. No test symbol has a method
// receiver, so the declaring package is everything before the last dot.
func statOf(symbol string, cyclomatic int, cognitive int) complexity.Stat {
	return complexity.Stat{
		ImportPath: symbol[:strings.LastIndex(symbol, ".")],
		Symbol:     symbol,
		File:       testSampleFile,
		Line:       1,
		Cyclomatic: cyclomatic,
		Cognitive:  cognitive,
	}
}

func samplePackageInfo() packageInfo {
	info := packageInfo{
		ImportPath: testImportPath,
		Dir:        testSampleDir,
		Module:     nil,
		GoFiles:    nil,
		CgoFiles:   nil,
	}
	info.Module = &struct {
		Path string `json:"Path"`
	}{Path: testModulePath}

	return info
}

func goListBrokenJSON() []byte {
	return []byte(`{"ImportPath": "` + testImportPath + `", "Dir": "testdata/broken",` +
		` "GoFiles": ["broken.go"], "Module": {"Path": "` + testModulePath + `"}}` + "\n")
}

func TestStaticKeyStripsGenericsAndClosures(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		testWorkFunc:                        testWorkFunc,
		testWorkFunc + "[go.shape.int]":      testWorkFunc,
		testWorkFunc + ".func1":              testWorkFunc,
		testWorkFunc + ".func1.2":            testWorkFunc,
		testImportPath + ".(*Worker).Run":    testImportPath + ".(*Worker).Run",
		testImportPath + ".(*Worker).Run[T]": testImportPath + ".(*Worker).Run",
	}

	for symbol, want := range tests {
		require.Equal(t, want, staticKey(symbol), symbol)
	}
}

func TestResolveModulePackagesKeepsModuleLocalPackages(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{{out: goListDepsJSON(), err: nil}}

	got, err := newCommand(t, runner).resolveModulePackages(testPkgArg, testModulePath)
	require.NoError(t, err)
	require.Len(t, got, 1, "standard library and other modules are not user code")
	require.Equal(t, testImportPath, got[0].ImportPath)
	require.Equal(t, []string{"sample.go"}, got[0].Files)
	require.Contains(t, runner.calls[0].Args, "-deps")
}

func TestResolveModulePackagesWithoutModulePath(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{{out: goListDepsJSON(), err: nil}}

	got, err := newCommand(t, runner).resolveModulePackages(testPkgArg, "")
	require.NoError(t, err)
	require.Empty(t, got, "without a module path there is no user-code boundary to trust")
	require.Empty(t, runner.calls, "no package listing is needed")
}

func TestResolveModulePackagesIncludesCgoSources(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{{out: goListCgoJSON(), err: nil}}

	got, err := newCommand(t, runner).resolveModulePackages(testPkgArg, testModulePath)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []string{"sample.go", "bridge.go"}, got[0].Files,
		"a cgo package keeps its Go-side sources in CgoFiles")
}

func TestCommandRunFallsBackToComplexityWithoutBenchmark(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = noBenchmarkOutputs()

	var out strings.Builder

	err := newCommand(t, runner).Run([]string{testPkgArg}, &out)
	require.NoError(t, err)
	require.Len(t, runner.calls, noBenchmarkCallCount, "the memory pass is skipped when no benchmark ran")

	text := out.String()
	require.Contains(t, text, classComplexityHotspot)
	require.Contains(t, text, testWorkFunc)
	require.Contains(t, text, "sample.go:")
	require.Contains(t, text, "static estimate")
	require.Contains(t, text, "verdict help bootstrap")
}

func TestCommandRunEnrichesHotFunctionWithComplexity(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = fullRunOutputs()

	var out strings.Builder

	err := newCommand(t, runner).Run([]string{testFlagFormat, formatJSON, testPkgArg}, &out)
	require.NoError(t, err)

	text := out.String()
	require.Contains(t, text, `"schema_version": 2`)
	require.Contains(t, text, `"file": "sample.go"`)
	require.Contains(t, text, `"cyclomatic": 10`)
	require.Contains(t, text, `"line":`)
}

// goListDepsJSON mimics "go list -deps -json" with one module-local package,
// one standard library package, and one package from another module.
func goListDepsJSON() []byte {
	stdlib := `{"ImportPath": "strings", "Dir": "/usr/local/go/src/strings", "GoFiles": ["strings.go"]}`
	external := `{"ImportPath": "example.com/dep", "Dir": "/gopath/dep", "GoFiles": ["dep.go"],` +
		` "Module": {"Path": "example.com/dep"}}`
	local := `{"ImportPath": "` + testImportPath + `", "Dir": "` + testSampleDir + `",` +
		` "GoFiles": ["sample.go"], "Module": {"Path": "` + testModulePath + `"}}`

	return []byte(stdlib + "\n" + external + "\n" + local + "\n")
}

func goListCgoJSON() []byte {
	return []byte(`{"ImportPath": "` + testImportPath + `", "Dir": "` + testSampleDir + `",` +
		` "GoFiles": ["sample.go"], "CgoFiles": ["bridge.go"],` +
		` "Module": {"Path": "` + testModulePath + `"}}` + "\n")
}

func noBenchmarkOutputs() []fakeOutput {
	return []fakeOutput{
		{out: goListJSON(testImportPath, testSampleDir), err: nil},
		{out: goListDepsJSON(), err: nil},
		{out: []byte("compiled"), err: nil},
		{out: []byte("PASS\n"), err: nil},
	}
}

func TestResolvePackageRejectsAnEmptyListing(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{{out: []byte(""), err: nil}}

	_, err := newCommand(t, runner).resolvePackage(testPkgArg)
	require.ErrorIs(t, err, errMissingPackage)
}

func TestModulePathIsEmptyOutsideAModule(t *testing.T) {
	t.Parallel()

	outside := packageInfo{
		ImportPath: testImportPath,
		Dir:        testSampleDir,
		Module:     nil,
		GoFiles:    nil,
		CgoFiles:   nil,
	}
	require.Empty(t, outside.modulePath())
	require.Equal(t, testModulePath, samplePackageInfo().modulePath())
}

func TestCommandRunReportsAStaticAnalysisFailure(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{
		{out: goListJSON(testImportPath, testSampleDir), err: nil},
		{out: goListBrokenJSON(), err: nil},
	}

	err := newCommand(t, runner).Run([]string{testPkgArg}, &strings.Builder{})
	require.ErrorContains(t, err, "analyzing complexity")
}

func TestIsBenchmarkFunctionNeedsAQualifiedName(t *testing.T) {
	t.Parallel()

	require.True(t, isBenchmarkFunction(testImportPath+".BenchmarkWork"))
	require.False(t, isBenchmarkFunction("BenchmarkWork"), "an unqualified name is not a profile symbol")
	require.False(t, isBenchmarkFunction(testImportPath+"."), "a trailing dot has no function name")
	require.False(t, isBenchmarkFunction(testWorkFunc))
}

func TestResolvePackageReportsMalformedJSON(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{{out: []byte("{"), err: nil}}

	_, err := newCommand(t, runner).resolvePackage(testPkgArg)
	require.ErrorContains(t, err, "decoding go list output")
}

func TestCommandRunReportsAModulePackageListingFailure(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{
		{out: goListJSON(testImportPath, testSampleDir), err: nil},
		{out: nil, err: errFakeRun},
	}

	err := newCommand(t, runner).Run([]string{testPkgArg}, &strings.Builder{})
	require.ErrorContains(t, err, "listing module packages")
}

func TestStaticKeyStripsNestedGenericShapes(t *testing.T) {
	t.Parallel()

	require.Equal(t, testWorkFunc, staticKey(testWorkFunc+"[go.shape.[]int]"),
		"a shape may itself be a composite type")
	require.Equal(t, testImportPath+".(*Box).Get",
		staticKey(testImportPath+".(*Box[go.shape.map[string]int]).Get"))
	require.Equal(t, testWorkFunc, staticKey(testWorkFunc+"[go.shape.[]int].func1"))
}
