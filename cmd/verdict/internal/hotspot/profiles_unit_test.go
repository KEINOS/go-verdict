package hotspot

// This file covers the profiling signals: profile kinds, value units, and the
// separate CPU and memory profiling passes.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	benchmarkRunLine = "BenchmarkWork-10 1 100 ns/op\nPASS\n"

	// Call counts for one hotspot run: two package listings, one compile, the
	// benchmark passes, and one pprof read per signal.
	fullRunCallCount     = 9
	fastRunCallCount     = 8
	noBenchmarkCallCount = 4
)

func TestPprofInvocationSelectsProfileKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantArg string
		kind    profileKind
	}{
		{name: signalCPU, kind: profileCPU, wantArg: ""},
		{name: "allocated bytes", kind: profileAlloc, wantArg: "-alloc_space"},
		{name: "allocated objects", kind: profileAllocObjects, wantArg: "-alloc_objects"},
		{name: "retained bytes", kind: profileInuse, wantArg: "-inuse_space"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := pprofInvocation("/tmp/hotspot.test", "/tmp/profile.out", test.kind)
			require.Equal(t, "go", got.Name)
			require.Contains(t, got.Args, "-nodecount=50")

			if test.wantArg == "" {
				joined := strings.Join(got.Args, " ")
				require.NotContains(t, joined, "_space")
				require.NotContains(t, joined, "_objects")

				return
			}

			require.Contains(t, got.Args, test.wantArg)
		})
	}
}

func TestParseTopParsesObjectCountsAndRetainedBytes(t *testing.T) {
	t.Parallel()

	counts, err := parseTop([]byte(topOutput(testAllocFunc, "4096", "40.00%", "8192", "80.00%")), profileAllocObjects)
	require.NoError(t, err)
	require.Len(t, counts, 1)
	require.InDelta(t, 4096.0, counts[0].Flat, 0.001, "object counts have no unit suffix")
	require.InDelta(t, 8192.0, counts[0].Cum, 0.001)

	retained, err := parseTop([]byte(topOutput(testRetainFunc, "2.50MB", "50.00%", "5.00MB", "99.00%")), profileInuse)
	require.NoError(t, err)
	require.Len(t, retained, 1)
	require.InDelta(t, 2.5*bytesPerKB*bytesPerKB, retained[0].Flat, 0.01)
	require.InDelta(t, 5*bytesPerKB*bytesPerKB, retained[0].Cum, 0.01)
}

func TestReadProfilesCollectsEverySignal(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = profileTopOutputs()

	got, err := newCommand(t, runner).readProfiles("/tmp/hotspot.test", "/tmp/cpu.out", "/tmp/mem.out")
	require.NoError(t, err)
	require.Contains(t, got.CPU, testWorkFunc)
	require.Contains(t, got.Alloc, testWorkFunc)
	require.Contains(t, got.AllocObjects, testAllocFunc)
	require.Contains(t, got.Inuse, testRetainFunc)
	require.InDelta(t, 4096.0, got.AllocObjects[testAllocFunc].Flat, 0.001)
	require.InDelta(t, bytesPerKB*bytesPerKB, got.Inuse[testRetainFunc].Flat, 0.01)

	require.Len(t, runner.calls, 4, "every memory view is read from the same memory profile")
	require.Contains(t, runner.calls[1].Args, "-alloc_space")
	require.Contains(t, runner.calls[2].Args, "-alloc_objects")
	require.Contains(t, runner.calls[3].Args, "-inuse_space")
}

func TestCommandRunUsesSeparateProfilingPasses(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = fullRunOutputs()

	err := newCommand(t, runner).Run([]string{testPkgArg}, &strings.Builder{})
	require.NoError(t, err)
	require.Len(t, runner.calls, fullRunCallCount)

	cpuPass := strings.Join(runner.calls[3].Args, " ")
	require.Contains(t, cpuPass, "-test.cpuprofile=")
	require.NotContains(t, cpuPass, "-test.memprofile", "allocation profiling biases the CPU profile")

	memoryPass := strings.Join(runner.calls[4].Args, " ")
	require.Contains(t, memoryPass, "-test.memprofile=")
	require.Contains(t, memoryPass, "-test.memprofilerate=1")
	require.NotContains(t, memoryPass, "-test.cpuprofile")
}

func TestCommandRunFastCombinesProfilingIntoOnePass(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = fastRunOutputs()

	var out strings.Builder

	err := newCommand(t, runner).Run([]string{testFlagFast, testPkgArg}, &out)
	require.NoError(t, err)
	require.Len(t, runner.calls, fastRunCallCount)

	onePass := strings.Join(runner.calls[3].Args, " ")
	require.Contains(t, onePass, "-test.cpuprofile=")
	require.Contains(t, onePass, "-test.memprofile=")
	require.Contains(t, out.String(), "--fast", "the caveat must name the flag that lowered CPU accuracy")
}

func TestCommandRunFastOmitsTheCaveatWithoutMeasurement(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = noBenchmarkOutputs()

	var out strings.Builder

	err := newCommand(t, runner).Run([]string{testFlagFast, testPkgArg}, &out)
	require.NoError(t, err)
	require.Contains(t, out.String(), "static estimate")
	require.NotContains(t, out.String(), "--fast",
		"nothing was measured, so there is no CPU accuracy to caveat")
}

func TestCommandRunRejectsAnInconsistentMemoryPass(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{
		{out: goListJSON(testImportPath, testPkgDir), err: nil},
		{out: goListDepsJSON(), err: nil},
		{out: []byte("compiled"), err: nil},
		{out: []byte(benchmarkRunLine), err: nil},
		{out: []byte("PASS\n"), err: nil},
	}

	err := newCommand(t, runner).Run([]string{testPkgArg}, &strings.Builder{})
	require.ErrorIs(t, err, errInconsistentPass,
		"memory samples from a run without the workload would misreport the benchmark")
	require.ErrorContains(t, err, "--fast")
}

func TestHelpTextDocumentsFast(t *testing.T) {
	t.Parallel()

	require.Contains(t, HelpText(), "--fast")
}

func benchmarkRunOutputs() []fakeOutput {
	return []fakeOutput{
		{out: goListJSON(testImportPath, testPkgDir), err: nil},
		{out: goListDepsJSON(), err: nil},
		{out: []byte("compiled"), err: nil},
		{out: []byte(benchmarkRunLine), err: nil},
		{out: []byte(benchmarkRunLine), err: nil},
	}
}

func profileTopOutputs() []fakeOutput {
	return []fakeOutput{
		{out: []byte(topOutput(testWorkFunc, "20ms", "20.00%", "30ms", "30.00%")), err: nil},
		{out: []byte(topOutput(testWorkFunc, "10kB", "10.00%", "20kB", "20.00%")), err: nil},
		{out: []byte(topOutput(testAllocFunc, "4096", "40.00%", "8192", "80.00%")), err: nil},
		{out: []byte(topOutput(testRetainFunc, "1.00MB", "60.00%", "2.00MB", "90.00%")), err: nil},
	}
}

func fullRunOutputs() []fakeOutput {
	return append(benchmarkRunOutputs(), profileTopOutputs()...)
}

// fastRunOutputs drops the second benchmark run because --fast profiles once.
func fastRunOutputs() []fakeOutput {
	outputs := benchmarkRunOutputs()

	return append(outputs[:len(outputs)-1], profileTopOutputs()...)
}

func TestParseTopLineRejectsMalformedRows(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"too few fields":     "10ms 50% 50% 20ms 100%",
		"flat percent sign":  "10ms 50 50% 20ms 100% pkg.Fn",
		"cum percent sign":   "10ms 50% 50% 20ms 100 pkg.Fn",
		"unparsable flat":    "abcms 50% 50% 20ms 100% pkg.Fn",
		"unparsable cum":     "10ms 50% 50% abcms 100% pkg.Fn",
		"unparsable percent": "10ms x% 50% 20ms 100% pkg.Fn",
		"blank":              "",
	}

	for name, line := range tests {
		_, ok := parseTopLine(line, profileCPU)
		require.False(t, ok, name)
	}

	_, ok := parseTopLine("10ms 50% 50% 20ms 100% pkg.Fn", profileCPU)
	require.True(t, ok, "a well-formed row still parses")
}

func TestParseUnitValueRejectsABadNumberWithAKnownUnit(t *testing.T) {
	t.Parallel()

	_, ok := parseByteValue("abckB")
	require.False(t, ok)

	_, ok = parseCountValue("not-a-number")
	require.False(t, ok)
}

func TestParseValueAndSampleFlagRejectUnknownKinds(t *testing.T) {
	t.Parallel()

	unknown := profileKind(99)

	_, ok := parseValue("10", unknown)
	require.False(t, ok)
	require.Empty(t, sampleFlag(unknown))
	require.False(t, unknown.supported())
}

func TestCommandRunFastReportsABenchmarkFailure(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = []fakeOutput{
		{out: goListJSON(testImportPath, testPkgDir), err: nil},
		{out: goListDepsJSON(), err: nil},
		{out: []byte("compiled"), err: nil},
		{out: nil, err: errFakeRun},
	}

	err := newCommand(t, runner).Run([]string{testFlagFast, testPkgArg}, &strings.Builder{})
	require.ErrorContains(t, err, "running benchmark workload")
}

func TestCommandRunReportsAMemoryPassFailure(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()

	outputs := benchmarkRunOutputs()
	outputs[len(outputs)-1] = fakeOutput{out: nil, err: errFakeRun}
	runner.outputs = outputs

	err := newCommand(t, runner).Run([]string{testPkgArg}, &strings.Builder{})
	require.ErrorContains(t, err, "running benchmark workload")
}

func TestParseUnitValueFallsBackToABareNumber(t *testing.T) {
	t.Parallel()

	got, ok := parseByteValue("1024")
	require.True(t, ok, "pprof drops the unit when the value needs no scaling")
	require.InDelta(t, 1024.0, got, 0.001)
}
