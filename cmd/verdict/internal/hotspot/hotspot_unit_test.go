package hotspot

import (
	"errors"
	"os"
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
	testFlagFast   = "--fast"
	testSampleFile = "sample.go"
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
				top:       defaultTop,
				format:    formatJSON,
				pkg:       testPkgArg,
				fast:      false,
			},
			wantErr: nil,
		},
		{
			name:    "fast single pass",
			args:    []string{testFlagFast, testPkgArg},
			want:    fastOptions(testPkgArg),
			wantErr: nil,
		},
		{name: "missing package", args: nil, want: zeroOptions(), wantErr: errMissingPackage},
		{name: "extra package", args: []string{"./a", "./b"}, want: zeroOptions(), wantErr: errMissingPackage},
		{name: "bad count", args: []string{"--count", "0", testPkgArg}, want: zeroOptions(), wantErr: errInvalidCount},
		{name: "bad top", args: []string{"--top", "0", testPkgArg}, want: zeroOptions(), wantErr: errInvalidTop},
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
		top:       defaultTop,
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

func TestFormatResultTextAndJSON(t *testing.T) {
	t.Parallel()

	result := testResult()
	result.Classification = classCPUHotspot
	result.Function = testWorkFunc
	result.Signals = []string{signalCPU}
	result.CPU = Metric{Unit: unitMS, Flat: 10, FlatPct: 20, Cum: 30, CumPct: 40}
	result.Next = "Judge candidate changes with verdict."

	text, err := formatResult(result, defaultFormat)
	require.NoError(t, err)
	require.Contains(t, text, "inspect example.com/project/pkg.Work")
	require.Contains(t, text, "cpu flat 20.0%")

	jsonText, err := formatResult(result, formatJSON)
	require.NoError(t, err)
	require.Contains(t, jsonText, `"schema_version": 2`)
	require.Contains(t, jsonText, `"classification": "cpu-hotspot"`)
	require.Contains(t, jsonText, `"unit": "ms"`)

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

		err := newCommand(t, runner).Run([]string{"--bench", "BenchmarkWork", testPkgArg}, &out)
		require.NoError(t, err)
		require.Contains(t, out.String(), classHotAndComplex, "Work is hot in CPU and allocations and is complex")
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

		err := newCommand(t, runner).Run([]string{testPkgArg}, &out)
		require.NoError(t, err)
		require.Contains(t, out.String(), "No benchmark workload ran")
		require.Len(t, runner.calls, noBenchmarkCallCount)
	})

	t.Run("pprof error", func(t *testing.T) {
		t.Parallel()

		runner := newFakeRunner()

		runner.outputs = append(benchmarkRunOutputs(), fakeOutput{out: nil, err: errFakeRun})

		err := newCommand(t, runner).Run([]string{testPkgArg}, &strings.Builder{})
		require.ErrorContains(t, err, "reading CPU profile")
	})
}

func TestCommandRunInputOutputErrors(t *testing.T) {
	t.Parallel()

	var help strings.Builder

	err := newCommand(t, newFakeRunner()).Run([]string{"--help"}, &help)
	require.NoError(t, err)
	require.Contains(t, help.String(), "Usage:\n  verdict hotspot")

	err = newCommand(t, newFakeRunner()).Run([]string{testPkgArg}, nil)
	require.ErrorIs(t, err, errNilOutput)

	err = newCommand(t, newFakeRunner()).Run([]string{"--bad"}, &strings.Builder{})
	require.ErrorContains(t, err, "parsing hotspot flags")

	runner := newFakeRunner()
	runner.outputs = fullRunOutputs()

	err = newCommand(t, runner).Run([]string{testPkgArg}, failingWriter{})
	require.ErrorContains(t, err, "writing output")
}

func TestCommandRunHardErrorBranches(t *testing.T) {
	t.Parallel()

	for _, test := range hardErrorCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runner := newFakeRunner()
			runner.outputs = test.outputs

			err := newCommand(t, runner).Run([]string{testPkgArg}, &strings.Builder{})
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
		packageInfo{ImportPath: testImportPath, Dir: testPkgDir, Module: nil, GoFiles: nil, CgoFiles: nil},
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

	_, err := newCommand(t, runner).resolvePackage("./...")
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

// newCommand builds a command with fake process execution and a temp directory
// managed by the test, so no test touches the real one.
func newCommand(t *testing.T, runner commandRunner) Command {
	t.Helper()

	dir := t.TempDir()

	return Command{runner: runner, tempDir: func() (string, error) { return dir, nil }}
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: nil, calls: nil}
}

func zeroOptions() options {
	return options{bench: "", benchtime: "", count: 0, top: 0, format: "", pkg: "", fast: false}
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
		Function:       "",
		File:           "",
		Line:           0,
		Signals:        []string{},
		CPU:            zeroMetric(unitMS),
		AllocBytes:     zeroMetric(unitBytes),
		AllocObjects:   zeroMetric(unitObjects),
		Retained:       zeroMetric(unitBytes),
		Complexity:     Complexity{Cyclomatic: 0, Cognitive: 0},
		Candidates:     []Choice{},
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

func TestNewAndDefaultRunnerUseTheProcessRunner(t *testing.T) {
	t.Parallel()

	require.NotNil(t, New().runner, "New must be usable without further wiring")
	require.NotNil(t, New().tempDir)

	filled := Command{runner: nil, tempDir: nil}.withDefaults()
	require.NotNil(t, filled.runner, "a zero Command still runs processes")
	require.NotNil(t, filled.tempDir)
	require.Equal(t, New().runner, filled.runner, "both paths reach the same process runner")

	dir, err := filled.tempDir()
	require.NoError(t, err)
	require.DirExists(t, dir)
	require.NoError(t, os.RemoveAll(dir))
}

func TestScoutReportsATempDirectoryFailure(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{
		{out: goListJSON(testImportPath, testSampleDir), err: nil},
		{out: goListDepsJSON(), err: nil},
	}

	command := Command{
		runner:  runner,
		tempDir: func() (string, error) { return "", errFakeRun },
	}

	err := command.Run([]string{testPkgArg}, &strings.Builder{})
	require.ErrorIs(t, err, errFakeRun)
}

func TestExecRunnerRunsAndReportsRealProcesses(t *testing.T) {
	t.Parallel()

	output, err := execRunner{}.Run(invocation{Dir: "", Name: "go", Args: []string{"version"}})
	require.NoError(t, err)
	require.Contains(t, string(output), "go version")

	_, err = execRunner{}.Run(invocation{Dir: "", Name: "verdict-no-such-binary", Args: nil})
	require.Error(t, err)
	require.ErrorContains(t, err, "verdict-no-such-binary")
}
