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
	fullRunCallCount = 8
	fastRunCallCount = 7
)

func TestPprofInvocationSelectsProfileKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantArg string
		kind    profileKind
	}{
		{name: "cpu", kind: profileCPU, wantArg: ""},
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

	got, err := (Command{runner: runner}).readProfiles("/tmp/hotspot.test", "/tmp/cpu.out", "/tmp/mem.out")
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

	err := (Command{runner: runner}).Run([]string{testPkgArg}, &strings.Builder{})
	require.NoError(t, err)
	require.Len(t, runner.calls, fullRunCallCount)

	cpuPass := strings.Join(runner.calls[2].Args, " ")
	require.Contains(t, cpuPass, "-test.cpuprofile=")
	require.NotContains(t, cpuPass, "-test.memprofile", "allocation profiling biases the CPU profile")

	memoryPass := strings.Join(runner.calls[3].Args, " ")
	require.Contains(t, memoryPass, "-test.memprofile=")
	require.Contains(t, memoryPass, "-test.memprofilerate=1")
	require.NotContains(t, memoryPass, "-test.cpuprofile")
}

func TestCommandRunFastCombinesProfilingIntoOnePass(t *testing.T) {
	t.Parallel()

	runner := newFakeRunner()
	runner.outputs = fastRunOutputs()

	var out strings.Builder

	err := (Command{runner: runner}).Run([]string{"--fast", testPkgArg}, &out)
	require.NoError(t, err)
	require.Len(t, runner.calls, fastRunCallCount)

	onePass := strings.Join(runner.calls[2].Args, " ")
	require.Contains(t, onePass, "-test.cpuprofile=")
	require.Contains(t, onePass, "-test.memprofile=")
	require.Contains(t, out.String(), "--fast", "the caveat must name the flag that lowered CPU accuracy")
}

func TestHelpTextDocumentsFast(t *testing.T) {
	t.Parallel()

	require.Contains(t, HelpText(), "--fast")
}

func benchmarkRunOutputs() []fakeOutput {
	return []fakeOutput{
		{out: goListJSON(testImportPath, "/repo/pkg"), err: nil},
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
