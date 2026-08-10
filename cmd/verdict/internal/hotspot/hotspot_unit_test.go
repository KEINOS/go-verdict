package hotspot

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var errFakeRun = errors.New("fake run error")

const (
	testPkgArg     = "./pkg"
	testImportPath = "example.com/project/pkg"
	testModulePath = "example.com/project"
	testMixedFunc  = "example.com/project/pkg.Mixed"
	testWorkFunc   = "example.com/project/pkg.Work"
	testAllocFunc  = "example.com/project/pkg.Alloc"
	testRetainFunc = "example.com/project/pkg.Retain"
	testPkgDir     = "/repo/pkg"
	testSampleDir  = "testdata/sample"
	testFlagFormat = "--format"
)

func TestParseArgsValidation(t *testing.T) {
	t.Parallel()

	for _, test := range parseArgsCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseArgs(test.args)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

type parseArgsCase struct {
	wantErr error
	name    string
	args    []string
	want    options
}

func parseArgsCases() []parseArgsCase {
	return []parseArgsCase{
		{
			name:    "defaults",
			args:    []string{testPkgArg},
			want:    defaultOptions(testPkgArg),
			wantErr: nil,
		},
		{
			name: "custom values",
			args: []string{"--bench", "BenchmarkFoo", "--benchtime", "25x", "--count", "3", testFlagFormat, "json", testPkgArg},
			want: options{
				bench:     "BenchmarkFoo",
				benchtime: "25x",
				count:     3,
				format:    formatJSON,
				pkg:       testPkgArg,
				fast:      false,
			},
			wantErr: nil,
		},
		{
			name:    "fast single pass",
			args:    []string{"--fast", testPkgArg},
			want:    fastOptions(testPkgArg),
			wantErr: nil,
		},
		{name: "missing package", args: nil, want: zeroOptions(), wantErr: errMissingPackage},
		{name: "extra package", args: []string{"./a", "./b"}, want: zeroOptions(), wantErr: errMissingPackage},
		{name: "bad count", args: []string{"--count", "0", testPkgArg}, want: zeroOptions(), wantErr: errInvalidCount},
		{
			name:    "bad benchtime",
			args:    []string{"--benchtime", "zero", testPkgArg},
			want:    zeroOptions(),
			wantErr: errInvalidBenchtime,
		},
		{name: "bad format", args: []string{"--format", "yaml", testPkgArg}, want: zeroOptions(), wantErr: errInvalidFormat},
	}
}

func defaultOptions(pkg string) options {
	return options{
		bench:     defaultBench,
		benchtime: defaultBenchtime,
		count:     defaultCount,
		format:    defaultFormat,
		pkg:       pkg,
		fast:      false,
	}
}

func fastOptions(pkg string) options {
	opts := defaultOptions(pkg)
	opts.fast = true

	return opts
}

func TestParseTopNormalizesAndConvertsUnits(t *testing.T) {
	t.Parallel()

	output := []byte(`
      flat  flat%   sum%        cum   cum%
     100ms  50.0% 50.0%      200ms 100.0%  example.com/project/pkg.(*Worker).Run[go.shape.int] (inline)
      20us  10.0% 60.0%       30us  15.0%  example.com/project/pkg.Work.func1
`)

	rows, err := parseTop(output, profileCPU)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "example.com/project/pkg.(*Worker).Run[go.shape.int]", rows[0].Function)
	require.InDelta(t, 100.0, rows[0].Flat, 0.001)
	require.InDelta(t, 0.02, rows[1].Flat, 0.001)
}

func TestRowsByFunctionAggregatesNormalizedDuplicates(t *testing.T) {
	t.Parallel()

	rows := rowsByFunction([]pprofRow{
		{Function: testWorkFunc, Flat: 10, FlatPct: 10, Cum: 20, CumPct: 20},
		{Function: testWorkFunc, Flat: 5, FlatPct: 5, Cum: 15, CumPct: 15},
	})

	require.InDelta(t, 15.0, rows[testWorkFunc].Flat, 0.001)
	require.InDelta(t, 15.0, rows[testWorkFunc].FlatPct, 0.001)
	require.InDelta(t, 20.0, rows[testWorkFunc].Cum, 0.001)
	require.InDelta(t, 20.0, rows[testWorkFunc].CumPct, 0.001)
}

func TestParseTopAllocAndMalformed(t *testing.T) {
	t.Parallel()

	rows, err := parseTop([]byte(`
      flat  flat%   sum%        cum   cum%
 1152.30kB 47.24% 47.24%  2.00MB 50.00%  example.com/project/pkg.allocBytes
`), profileAlloc)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.InDelta(t, 1152.30*bytesPerKB, rows[0].Flat, 0.01)
	require.InDelta(t, 2*bytesPerKB*bytesPerKB, rows[0].Cum, 0.01)

	_, err = parseTop([]byte("not pprof"), profileAlloc)
	require.ErrorIs(t, err, errNoPprofRows)

	_, err = parseTop([]byte(""), profileKind(99))
	require.ErrorIs(t, err, errUnsupportedProfile)
}

func TestUserRowsFiltersAndNormalizesVendor(t *testing.T) {
	t.Parallel()

	rows := rowsByFunction([]pprofRow{
		{Function: normalizeSymbol("example.com/project/pkg.Work (inline)"), Flat: 0, FlatPct: 20, Cum: 0, CumPct: 30},
		{
			Function: normalizeSymbol("example.com/project/vendor/example.com/dep.Helper"),
			Flat:     0,
			FlatPct:  99,
			Cum:      0,
			CumPct:   99,
		},
		{Function: "runtime.mallocgc", Flat: 0, FlatPct: 99, Cum: 0, CumPct: 99},
	})

	got := userRows(rows, []string{testModulePath})
	require.Contains(t, got, "example.com/project/pkg.Work")
	require.NotContains(t, got, "example.com/dep.Helper")
	require.NotContains(t, got, "runtime.mallocgc")
}

func TestClassifyMixedAndTieBreaking(t *testing.T) {
	t.Parallel()

	base := testResult()

	got := classify(base, profileSet{
		CPU: map[string]pprofRow{
			testMixedFunc:                 {Function: testMixedFunc, Flat: 10, FlatPct: 10, Cum: 20, CumPct: 20},
			"example.com/project/pkg.CPU": {Function: "example.com/project/pkg.CPU", Flat: 50, FlatPct: 50, Cum: 50, CumPct: 50},
		},
		Alloc: map[string]pprofRow{
			testMixedFunc: {Function: testMixedFunc, Flat: bytesPerKB, FlatPct: 10, Cum: bytesPerKB, CumPct: 10},
		},
		AllocObjects: nil,
		Inuse:        nil,
	}, nil)
	require.Equal(t, classMixedHotspot, got.Classification)
	require.Equal(t, testMixedFunc, got.Function)
	require.Nil(t, got.Secondary)

	got = classify(base, profileSet{
		CPU: map[string]pprofRow{
			"example.com/project/pkg.Beta": {Function: "example.com/project/pkg.Beta", Flat: 0, FlatPct: 10, Cum: 0, CumPct: 10},
			"example.com/project/pkg.Alfa": {Function: "example.com/project/pkg.Alfa", Flat: 0, FlatPct: 10, Cum: 0, CumPct: 10},
		},
		Alloc: map[string]pprofRow{
			testAllocFunc: {Function: testAllocFunc, Flat: 0, FlatPct: 60, Cum: 0, CumPct: 60},
		},
		AllocObjects: nil,
		Inuse:        nil,
	}, nil)
	require.Equal(t, classCPUHotspot, got.Classification)
	require.Equal(t, "example.com/project/pkg.Alfa", got.Function)
	require.NotNil(t, got.Secondary)
	require.Equal(t, "example.com/project/pkg.Beta", got.Secondary.Function)
}

func TestClassifyNoClear(t *testing.T) {
	t.Parallel()

	base := testResult()
	base.Classification = ""
	base.Reason = ""

	got := classify(base, profileSet{
		CPU:          map[string]pprofRow{},
		Alloc:        map[string]pprofRow{},
		AllocObjects: map[string]pprofRow{},
		Inuse:        map[string]pprofRow{},
	}, nil)

	require.Equal(t, classNoClearHotspot, got.Classification)
	require.Contains(t, got.Caveat, "No clear")
}

func TestFormatResultTextAndJSON(t *testing.T) {
	t.Parallel()

	result := testResult()
	result.Classification = classCPUHotspot
	result.Reason = classCPUHotspot
	result.Function = testWorkFunc
	result.CPU = Metric{FlatMS: 10, FlatBytes: 0, FlatPct: 20, CumMS: 30, CumBytes: 0, CumPct: 40}
	result.Next = "Judge candidate changes with verdict."

	text, err := formatResult(result, defaultFormat)
	require.NoError(t, err)
	require.Contains(t, text, "inspect example.com/project/pkg.Work")
	require.Contains(t, text, "cpu flat 20.0%")

	jsonText, err := formatResult(result, formatJSON)
	require.NoError(t, err)
	require.Contains(t, jsonText, `"schema_version": 2`)
	require.Contains(t, jsonText, `"reason": "cpu-hotspot"`)

	_, err = formatResult(result, "yaml")
	require.ErrorIs(t, err, errInvalidFormat)
}

func TestCommandRunSuccessNoBenchmarkAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		runner := newFakeRunner()
		runner.outputs = fullRunOutputs()

		var out strings.Builder

		err := (Command{runner: runner}).Run([]string{"--bench", "BenchmarkWork", testPkgArg}, &out)
		require.NoError(t, err)
		require.Contains(t, out.String(), classMixedHotspot)
		require.Len(t, runner.calls, fullRunCallCount)
		require.Equal(t, testPkgDir, runner.calls[3].Dir)
		require.Contains(t, runner.calls[3].Args, "-test.bench=BenchmarkWork")
		require.Contains(t, runner.calls[5].Args, "-nodecount=50")
		require.Contains(t, runner.calls[6].Args, "-alloc_space")
	})

	t.Run("no benchmark", func(t *testing.T) {
		t.Parallel()

		runner := newFakeRunner()
		runner.outputs = noBenchmarkOutputs()

		var out strings.Builder

		err := (Command{runner: runner}).Run([]string{testPkgArg}, &out)
		require.NoError(t, err)
		require.Contains(t, out.String(), "No benchmark workload ran")
		require.Len(t, runner.calls, noBenchmarkCallCount)
	})

	t.Run("pprof error", func(t *testing.T) {
		t.Parallel()

		runner := newFakeRunner()

		runner.outputs = append(benchmarkRunOutputs(), fakeOutput{out: nil, err: errFakeRun})

		err := (Command{runner: runner}).Run([]string{testPkgArg}, &strings.Builder{})
		require.ErrorContains(t, err, "reading CPU profile")
	})
}

func TestCommandRunInputOutputErrors(t *testing.T) {
	t.Parallel()

	var help strings.Builder

	err := (Command{runner: newFakeRunner()}).Run([]string{"--help"}, &help)
	require.NoError(t, err)
	require.Contains(t, help.String(), "Usage:\n  verdict hotspot")

	err = (Command{runner: newFakeRunner()}).Run([]string{testPkgArg}, nil)
	require.ErrorIs(t, err, errNilOutput)

	err = (Command{runner: newFakeRunner()}).Run([]string{"--bad"}, &strings.Builder{})
	require.ErrorContains(t, err, "parsing hotspot flags")

	runner := newFakeRunner()
	runner.outputs = fullRunOutputs()

	err = (Command{runner: runner}).Run([]string{testPkgArg}, failingWriter{})
	require.ErrorContains(t, err, "writing output")
}

func TestCommandRunHardErrorBranches(t *testing.T) {
	t.Parallel()

	for _, test := range hardErrorCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runner := newFakeRunner()
			runner.outputs = test.outputs

			err := (Command{runner: runner}).Run([]string{testPkgArg}, &strings.Builder{})
			require.ErrorContains(t, err, test.want)
		})
	}
}

func hardErrorCases() []struct {
	name    string
	want    string
	outputs []fakeOutput
} {
	return []struct {
		name    string
		want    string
		outputs []fakeOutput
	}{
		{name: "resolve", outputs: []fakeOutput{{out: nil, err: errFakeRun}}, want: "resolving package"},
		{name: "compile", outputs: fakeOutputsUntilCompileError(), want: "compiling benchmark binary"},
		{name: "benchmark", outputs: fakeOutputsUntilBenchmarkError(), want: "running benchmark workload"},
		{name: "allocation pprof", outputs: fakeOutputsUntilAllocProfileError(), want: "reading allocation profile"},
		{name: "malformed pprof", outputs: fakeOutputsWithMalformedCPUProfile(), want: "parsing CPU profile"},
	}
}

func fakeOutputsUntilCompileError() []fakeOutput {
	return []fakeOutput{
		{out: goListJSON(testImportPath, testPkgDir), err: nil},
		{out: goListDepsJSON(), err: nil},
		{out: nil, err: errFakeRun},
	}
}

func fakeOutputsUntilBenchmarkError() []fakeOutput {
	return []fakeOutput{
		{out: goListJSON(testImportPath, testPkgDir), err: nil},
		{out: goListDepsJSON(), err: nil},
		{out: []byte("compiled"), err: nil},
		{out: nil, err: errFakeRun},
	}
}

func fakeOutputsUntilAllocProfileError() []fakeOutput {
	return append(
		benchmarkRunOutputs(),
		fakeOutput{out: []byte(topOutput(testWorkFunc, "20ms", "20.00%", "30ms", "30.00%")), err: nil},
		fakeOutput{out: nil, err: errFakeRun},
	)
}

func fakeOutputsWithMalformedCPUProfile() []fakeOutput {
	return append(
		benchmarkRunOutputs(),
		fakeOutput{out: []byte("not pprof"), err: nil},
		fakeOutput{out: []byte(topOutput(testWorkFunc, "10kB", "10.00%", "20kB", "20.00%")), err: nil},
	)
}

func TestBaseResultFallbackCaveatAndNoClearText(t *testing.T) {
	t.Parallel()

	result := baseResult(
		defaultOptions(testPkgArg),
		packageInfo{ImportPath: testImportPath, Dir: testPkgDir, Module: nil, GoFiles: nil},
	)

	require.Contains(t, result.Caveat, "Module path was unavailable")

	text, err := formatResult(result, defaultFormat)
	require.NoError(t, err)
	require.Contains(t, text, "no clear user-code hotspot")
	require.Contains(t, text, "Caveat:")
}

func TestResolvePackageRejectsMultiPackage(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{{
		out: append(
			goListJSON("example.com/project/a", "/repo/a"),
			goListJSON("example.com/project/b", "/repo/b")...,
		),
		err: nil,
	}}

	_, err := (Command{runner: runner}).resolvePackage("./...")
	require.ErrorIs(t, err, errMultiplePackages)
}

type fakeOutput struct {
	err error
	out []byte
}

type fakeRunner struct {
	outputs []fakeOutput
	calls   []invocation
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errFakeRun
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: nil, calls: nil}
}

func zeroOptions() options {
	return options{bench: "", benchtime: "", count: 0, format: "", pkg: "", fast: false}
}

// testResult is a fully specified baseline report for classification and
// formatting tests.
func testResult() Result {
	return Result{
		SchemaVersion:  schemaVersion,
		Package:        testPkgArg,
		ImportPath:     testImportPath,
		Benchmark:      ".",
		Classification: classNoClearHotspot,
		Reason:         classNoClearHotspot,
		Function:       "",
		File:           "",
		Line:           0,
		CPU:            zeroMetric(),
		Alloc:          zeroMetric(),
		Complexity:     Complexity{Cyclomatic: 0, Cognitive: 0},
		Secondary:      nil,
		Caveat:         "",
		Next:           "next",
	}
}

func (runner *fakeRunner) Run(command invocation) ([]byte, error) {
	runner.calls = append(runner.calls, command)
	if len(runner.outputs) == 0 {
		return nil, errFakeRun
	}

	output := runner.outputs[0]
	runner.outputs = runner.outputs[1:]

	return output.out, output.err
}

func goListJSON(importPath string, dir string) []byte {
	return []byte(
		`{"ImportPath": "` + importPath + `", "Dir": "` + dir + `", "Module": {"Path": "` + testModulePath + `"}}` + "\n",
	)
}

// topOutput builds one "go tool pprof -top" row for any profile kind.
func topOutput(function string, flat string, flatPct string, cum string, cumPct string) string {
	return "      flat  flat%   sum%        cum   cum%\n" +
		"     " + flat + " " + flatPct + " " + flatPct + " " + cum + " " + cumPct + "  " + function + "\n"
}
