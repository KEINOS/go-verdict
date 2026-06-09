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
)

func TestParseArgsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr error
	}{
		{
			name: "defaults",
			args: []string{testPkgArg},
			want: options{
				bench:     defaultBench,
				benchtime: defaultBenchtime,
				count:     defaultCount,
				format:    defaultFormat,
				pkg:       testPkgArg,
			},
			wantErr: nil,
		},
		{
			name: "custom values",
			args: []string{"--bench", "BenchmarkFoo", "--benchtime", "25x", "--count", "3", "--format", "json", testPkgArg},
			want: options{bench: "BenchmarkFoo", benchtime: "25x", count: 3, format: formatJSON, pkg: testPkgArg},
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

	for _, test := range tests {
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

	base := Result{
		SchemaVersion:  schemaVersion,
		Package:        testPkgArg,
		ImportPath:     testImportPath,
		Benchmark:      ".",
		Classification: classNoClearHotspot,
		Reason:         classNoClearHotspot,
		Function:       "",
		CPU:            zeroMetric(),
		Alloc:          zeroMetric(),
		Secondary:      nil,
		Caveat:         "",
		Next:           "next",
	}

	got := classify(base, profileSet{
		CPU: map[string]pprofRow{
			testMixedFunc:                  {Function: testMixedFunc, Flat: 10, FlatPct: 10, Cum: 20, CumPct: 20},
			"example.com/project/pkg.CPU": {Function: "example.com/project/pkg.CPU", Flat: 50, FlatPct: 50, Cum: 50, CumPct: 50},
		},
		Alloc: map[string]pprofRow{
			testMixedFunc: {Function: testMixedFunc, Flat: bytesPerKB, FlatPct: 10, Cum: bytesPerKB, CumPct: 10},
		},
	})
	require.Equal(t, classMixedHotspot, got.Classification)
	require.Equal(t, testMixedFunc, got.Function)
	require.Nil(t, got.Secondary)

	got = classify(base, profileSet{
		CPU: map[string]pprofRow{
			"example.com/project/pkg.Beta": {Function: "example.com/project/pkg.Beta", Flat: 0, FlatPct: 10, Cum: 0, CumPct: 10},
			"example.com/project/pkg.Alfa": {Function: "example.com/project/pkg.Alfa", Flat: 0, FlatPct: 10, Cum: 0, CumPct: 10},
		},
		Alloc: map[string]pprofRow{
			"example.com/project/pkg.Alloc": {
				Function: "example.com/project/pkg.Alloc",
				Flat:     0,
				FlatPct:  60,
				Cum:      0,
				CumPct:   60,
			},
		},
	})
	require.Equal(t, classCPUHotspot, got.Classification)
	require.Equal(t, "example.com/project/pkg.Alfa", got.Function)
	require.NotNil(t, got.Secondary)
	require.Equal(t, "example.com/project/pkg.Beta", got.Secondary.Function)
}

func TestClassifyNoClear(t *testing.T) {
	t.Parallel()

	got := classify(Result{
		SchemaVersion:  schemaVersion,
		Package:        testPkgArg,
		ImportPath:     testImportPath,
		Benchmark:      ".",
		Classification: "",
		Reason:         "",
		Function:       "",
		CPU:            zeroMetric(),
		Alloc:          zeroMetric(),
		Secondary:      nil,
		Caveat:         "",
		Next:           "next",
	}, profileSet{CPU: map[string]pprofRow{}, Alloc: map[string]pprofRow{}})

	require.Equal(t, classNoClearHotspot, got.Classification)
	require.Contains(t, got.Caveat, "No clear")
}

func TestFormatResultTextAndJSON(t *testing.T) {
	t.Parallel()

	result := Result{
		SchemaVersion:  schemaVersion,
		Package:        testPkgArg,
		ImportPath:     testImportPath,
		Benchmark:      ".",
		Classification: classCPUHotspot,
		Reason:         classCPUHotspot,
		Function:       "example.com/project/pkg.Work",
		CPU:            Metric{FlatMS: 10, FlatBytes: 0, FlatPct: 20, CumMS: 30, CumBytes: 0, CumPct: 40},
		Alloc:          zeroMetric(),
		Secondary:      nil,
		Caveat:         "",
		Next:           "Judge candidate changes with verdict.",
	}

	text, err := formatResult(result, defaultFormat)
	require.NoError(t, err)
	require.Contains(t, text, "inspect example.com/project/pkg.Work")
	require.Contains(t, text, "cpu flat 20.0%")

	jsonText, err := formatResult(result, formatJSON)
	require.NoError(t, err)
	require.Contains(t, jsonText, `"schema_version": 1`)
	require.Contains(t, jsonText, `"reason": "cpu-hotspot"`)

	_, err = formatResult(result, "yaml")
	require.ErrorIs(t, err, errInvalidFormat)
}

func TestCommandRunSuccessNoBenchmarkAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		runner := newFakeRunner()
		runner.outputs = []fakeOutput{
			{out: goListJSON(testImportPath, "/repo/pkg"), err: nil},
			{out: []byte("compiled"), err: nil},
			{out: []byte("BenchmarkWork-10 1 100 ns/op\nPASS\n"), err: nil},
			{out: []byte(cpuTop("example.com/project/pkg.Work", "20ms", "20.00%", "30ms", "30.00%")), err: nil},
			{out: []byte(allocTop("example.com/project/pkg.Work", "10kB", "10.00%", "20kB", "20.00%")), err: nil},
		}

		var out strings.Builder

		err := (Command{runner: runner}).Run([]string{"--bench", "BenchmarkWork", testPkgArg}, &out)
		require.NoError(t, err)
		require.Contains(t, out.String(), classMixedHotspot)
		require.Len(t, runner.calls, 5)
		require.Equal(t, "/repo/pkg", runner.calls[2].Dir)
		require.Contains(t, runner.calls[2].Args, "-test.bench=BenchmarkWork")
		require.Contains(t, runner.calls[3].Args, "-nodecount=50")
		require.Contains(t, runner.calls[4].Args, "-alloc_space")
	})

	t.Run("no benchmark", func(t *testing.T) {
		t.Parallel()

		runner := newFakeRunner()
		runner.outputs = []fakeOutput{
			{out: goListJSON(testImportPath, "/repo/pkg"), err: nil},
			{out: []byte("compiled"), err: nil},
			{out: []byte("PASS\n"), err: nil},
		}

		var out strings.Builder

		err := (Command{runner: runner}).Run([]string{testPkgArg}, &out)
		require.NoError(t, err)
		require.Contains(t, out.String(), "no benchmark workload")
		require.Len(t, runner.calls, 3)
	})

	t.Run("pprof error", func(t *testing.T) {
		t.Parallel()

		runner := newFakeRunner()
		runner.outputs = []fakeOutput{
			{out: goListJSON(testImportPath, "/repo/pkg"), err: nil},
			{out: []byte("compiled"), err: nil},
			{out: []byte("BenchmarkWork-10 1 100 ns/op\nPASS\n"), err: nil},
			{out: nil, err: errFakeRun},
		}

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
	runner.outputs = []fakeOutput{
		{out: goListJSON(testImportPath, "/repo/pkg"), err: nil},
		{out: []byte("compiled"), err: nil},
		{out: []byte("BenchmarkWork-10 1 100 ns/op\nPASS\n"), err: nil},
		{out: []byte(cpuTop("example.com/project/pkg.Work", "20ms", "20.00%", "30ms", "30.00%")), err: nil},
		{out: []byte(allocTop("example.com/project/pkg.Work", "10kB", "10.00%", "20kB", "20.00%")), err: nil},
	}

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
	outputs []fakeOutput
	want    string
} {
	return []struct {
		name    string
		outputs []fakeOutput
		want    string
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
		{out: goListJSON(testImportPath, "/repo/pkg"), err: nil},
		{out: nil, err: errFakeRun},
	}
}

func fakeOutputsUntilBenchmarkError() []fakeOutput {
	return []fakeOutput{
		{out: goListJSON(testImportPath, "/repo/pkg"), err: nil},
		{out: []byte("compiled"), err: nil},
		{out: nil, err: errFakeRun},
	}
}

func fakeOutputsUntilAllocProfileError() []fakeOutput {
	return []fakeOutput{
		{out: goListJSON(testImportPath, "/repo/pkg"), err: nil},
		{out: []byte("compiled"), err: nil},
		{out: []byte("BenchmarkWork-10 1 100 ns/op\nPASS\n"), err: nil},
		{out: []byte(cpuTop("example.com/project/pkg.Work", "20ms", "20.00%", "30ms", "30.00%")), err: nil},
		{out: nil, err: errFakeRun},
	}
}

func fakeOutputsWithMalformedCPUProfile() []fakeOutput {
	return []fakeOutput{
		{out: goListJSON(testImportPath, "/repo/pkg"), err: nil},
		{out: []byte("compiled"), err: nil},
		{out: []byte("BenchmarkWork-10 1 100 ns/op\nPASS\n"), err: nil},
		{out: []byte("not pprof"), err: nil},
		{out: []byte(allocTop("example.com/project/pkg.Work", "10kB", "10.00%", "20kB", "20.00%")), err: nil},
	}
}

func TestBaseResultFallbackCaveatAndNoClearText(t *testing.T) {
	t.Parallel()

	result := baseResult(options{
		bench:     defaultBench,
		benchtime: defaultBenchtime,
		count:     defaultCount,
		format:    defaultFormat,
		pkg:       testPkgArg,
	}, packageInfo{ImportPath: testImportPath, Dir: "/repo/pkg", Module: nil})

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
	out []byte
	err error
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
	return options{bench: "", benchtime: "", count: 0, format: "", pkg: ""}
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

func cpuTop(function string, flat string, flatPct string, cum string, cumPct string) string {
	return "      flat  flat%   sum%        cum   cum%\n" +
		"     " + flat + " " + flatPct + " " + flatPct + " " + cum + " " + cumPct + "  " + function + "\n"
}

func allocTop(function string, flat string, flatPct string, cum string, cumPct string) string {
	return "      flat  flat%   sum%        cum   cum%\n" +
		"     " + flat + " " + flatPct + " " + flatPct + " " + cum + " " + cumPct + "  " + function + "\n"
}
