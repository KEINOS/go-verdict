package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/KEINOS/go-verdict/verdict"
	"github.com/stretchr/testify/require"
)

const (
	flagFormat   = "--format"
	flagAlpha    = "--alpha"
	flagMinDelta = "--min-delta"
	flagMode     = "--mode"
	formatText   = "text"
	formatJSON   = "json"
	modeAlt      = "alternatives"
	optionAlpha  = "alpha"
	testRevision = "0123456789abcdef"
	testVersion  = "v1.2.3"
	winningInput = "name          old time/op  new time/op  delta\n" +
		"Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=0.001 n=10+10)\n"
	alternativesInput = "BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n"
)

var errTestWrite = errors.New("test write error")

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errTestWrite
}

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, errTestWrite
}

//nolint:paralleltest // Uses process-wide mocks (os.Args and osExit).
func Test_main_fail(t *testing.T) {
	// Backup and restore
	oldOsExit := osExit
	oldOsArgs := os.Args

	t.Cleanup(func() {
		osExit = oldOsExit
		os.Args = oldOsArgs
	})

	// Mock os.Exit
	osExit = func(code int) {
		// panic instead of os.Exit to capture the exit code
		if code != 0 {
			panic(fmt.Sprintf("exit with code %d", code))
		}
	}

	// Mock args
	os.Args = []string{
		t.Name(),
		"--invalid-flag",
	}

	// Require panic due to invalid flag
	require.Panics(t, func() { main() },
		"invalid flag should error")
}

func TestRunCLITextFormat(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagFormat, formatText}, strings.NewReader(winningInput), &out)
	require.NoError(t, err,
		"failed to run CLI in text format")
	require.Contains(t, out.String(), "Foo-8: new wins",
		"output should include benchmark verdict")
}

func TestRunCLIVerboseTextFormat(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{"--verbose"}, strings.NewReader(winningInput), &out)
	require.NoError(t, err,
		"failed to run CLI in verbose mode")

	got := out.String()
	for _, want := range []string{"Foo-8: new wins", "Pareto-superior", "+ sec/op"} {
		require.Contains(t, got, want,
			"verbose output should contain expected snippet")
	}
}

func TestRunCLIJSONFormat(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagFormat, formatJSON}, strings.NewReader(winningInput), &out)
	require.NoError(t, err,
		"failed to run CLI in json format")
	require.Contains(t, out.String(), `"outcome": "new-wins"`,
		"json output should include new-wins outcome")
}

func TestRunCLIHelpWritesSamplePolicy(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagHelpLong}, strings.NewReader(""), &out)
	require.NoError(t, err,
		"failed to print help output")
	require.Contains(t, out.String(),
		"Turn Go benchmark results into a winner, tie, or trade-off.",
		"help opening sentence should match the approved Phase 27 wording")

	for _, want := range sampleCountWording() {
		require.Contains(t, out.String(), want,
			"help output should include shared sample policy wording")
	}

	for _, want := range []string{
		"Usage:\n  verdict [command] [options]",
		"Commands:\n  skill",
		"  version",
		"Options:\n  --format text|json",
		"  -v, --version",
	} {
		require.Contains(t, out.String(), want,
			"help output should include command and option sections")
	}
}

//nolint:paralleltest // Uses process-wide debugReadBuildInfo mock.
func TestGetAppVersion(t *testing.T) {
	tests := []struct {
		name      string
		buildInfo *debug.BuildInfo
		ok        bool
		want      string
	}{
		{
			name:      "build info unavailable",
			buildInfo: nil,
			ok:        false,
			want:      "unknown (devel)",
		},
		{
			name:      "uses module version and short revision",
			buildInfo: testBuildInfo(testVersion, testRevision),
			ok:        true,
			want:      "v1.2.3 (0123456)",
		},
		{
			name:      "keeps short revision as-is",
			buildInfo: testBuildInfo(testVersion, "abc123"),
			ok:        true,
			want:      "v1.2.3 (abc123)",
		},
		{
			name:      "uses unknown revision when version exists and revision is missing",
			buildInfo: testBuildInfo("v2.0.0", ""),
			ok:        true,
			want:      "v2.0.0 (unknown)",
		},
		{
			name:      "uses revision with devel fallback when version is missing",
			buildInfo: testBuildInfo("", testRevision),
			ok:        true,
			want:      "0123456 (devel)",
		},
		{
			name:      "treats devel version as missing release version",
			buildInfo: testBuildInfo(defaultVersion, testRevision),
			ok:        true,
			want:      "0123456 (devel)",
		},
		{
			name:      "falls back when version and revision are empty",
			buildInfo: testBuildInfo("", ""),
			ok:        true,
			want:      "unknown (devel)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			restoreDebugReadBuildInfo := mockDebugReadBuildInfo(test.buildInfo, test.ok)
			t.Cleanup(restoreDebugReadBuildInfo)

			require.Equal(t, test.want, GetAppVersion(),
				"app version should match build info policy")
		})
	}
}

//nolint:paralleltest // Uses process-wide debugReadBuildInfo mock.
func TestRunCLIVersionRequests(t *testing.T) {
	restoreDebugReadBuildInfo := mockDebugReadBuildInfo(testBuildInfo(testVersion, testRevision), true)
	t.Cleanup(restoreDebugReadBuildInfo)

	for _, args := range [][]string{
		{commandVersion},
		{flagVersionLong},
		{flagVersionShort},
	} {
		var out strings.Builder

		err := runCLI(args, strings.NewReader("ignored"), &out)
		require.NoError(t, err,
			"version request should succeed")
		require.Equal(t, "v1.2.3 (0123456)\n", out.String(),
			"version output should be deterministic")
	}
}

func TestRunCLIVersionRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{commandVersion, "extra"}, strings.NewReader("ignored"), &strings.Builder{})
	require.ErrorIs(t, err, errUnexpectedCommandArgs,
		"extra args should return 'errUnexpectedCommandArgs' error")
}

func TestRunCLIHelpWriteErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagHelpLong}, strings.NewReader(""), failingWriter{})
	require.ErrorIs(t, err, errWritingOutput,
		"help write errors should be wrapped with output context")
}

func TestRunCLISkillMatchesCanonicalSkill(t *testing.T) {
	t.Parallel()

	want, err := os.ReadFile(filepath.Join("..", "..", "skill", "verdict", "SKILL.md"))
	require.NoError(t, err,
		"failed to read canonical skill file")

	var out strings.Builder

	err = runCLI([]string{commandSkill}, strings.NewReader("ignored"), &out)
	require.NoError(t, err,
		"failed to run skill command")
	require.Equal(t, string(want), out.String(),
		"skill output does not match canonical SKILL.md")
}

func TestRunCLISkillNilOutputContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{commandSkill}, strings.NewReader("ignored"), nil)
	require.ErrorIs(t, err, errWritingOutput,
		"nil output should return 'errWritingOutput' error")
}

func TestRunCLISkillRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{commandSkill, "extra"}, strings.NewReader("ignored"), &strings.Builder{})
	require.ErrorIs(t, err, errUnexpectedCommandArgs,
		"extra args should return 'errUnexpectedCommandArgs' error")
}

func TestRunCLIUnknownCommand(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{"export-skill"}, strings.NewReader("ignored"), &strings.Builder{})
	require.ErrorIs(t, err, errUnknownCommand,
		"unknown command should return 'errUnknownCommand' error")
}

func TestRunCLIParseErrorContainsContext(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagFormat, formatText}, strings.NewReader("invalid"), &out)
	require.Error(t, err,
		"invalid input should return parse error")
	require.ErrorContains(t, err, "parsing input",
		"parse errors should include parsing input context")
}

func TestRunCLIRejectsInvalidStatisticalOptions(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "zero alpha", args: []string{flagAlpha, "0"}, want: optionAlpha},
		{name: "negative alpha", args: []string{flagAlpha, "-0.1"}, want: optionAlpha},
		{name: "alpha above one", args: []string{flagAlpha, "1.1"}, want: optionAlpha},
		{name: "negative min delta", args: []string{flagMinDelta, "-1"}, want: "min-delta"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := runCLI(test.args, strings.NewReader(winningInput), &strings.Builder{})
			require.Error(t, err)
			require.ErrorContains(t, err, test.want,
				"invalid statistical option should be mentioned in error")
		})
	}
}

func TestRunCLIBenchmarkSetMismatchGuidance(t *testing.T) {
	t.Parallel()

	input := `goos: darwin
goarch: arm64
pkg: example.com/foo
cpu: Apple M4
               │ ./fast.txt │ ./slow.txt │
               │   sec/op   │ sec/op vs base │
Fast-10               1.0n
Slow-10                         10.0n
geomean               1.0n      10.0n ? ¹ ²
¹ benchmark set differs from baseline; geomeans may not be comparable
`

	err := runCLI(nil, strings.NewReader(input), &strings.Builder{})
	require.Error(t, err,
		"benchmark mismatch should return guidance error")

	for _, want := range []string{"benchmark names differ", "verdict -a ./fast.txt -b ./slow.txt"} {
		require.ErrorContains(t, err, want,
			"mismatch guidance should include expected hint")
	}
}

func TestRunCLIABFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fastPath := filepath.Join(dir, "fast.txt")
	slowPath := filepath.Join(dir, "slow.txt")

	mustWriteFile(t, fastPath, strings.Repeat("BenchmarkExampleFast-10 100 1 ns/op\n", 10))
	mustWriteFile(t, slowPath, strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10))

	var out strings.Builder

	err := runCLI([]string{"-a", fastPath, "-b", slowPath}, strings.NewReader("ignored"), &out)
	require.NoError(t, err,
		"failed to compare raw benchmark files")
	require.Equal(t,
		"BenchmarkExampleFast_vs_BenchmarkExampleSlow: BenchmarkExampleFast wins\n",
		out.String(),
		"A/B comparison should pick fast benchmark as winner")
}

func TestRunCLIABFilesRequireBothSides(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{"-a", "fast.txt"}, strings.NewReader(""), &strings.Builder{})
	require.Error(t, err,
		"missing -b should return usage error")
	require.ErrorContains(t, err, "use both -a and -b",
		"missing side error should mention both flags")
}

func TestRunCLIABFilesReadError(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{"-a", "missing-a.txt", "-b", "missing-b.txt"}, strings.NewReader(""), &strings.Builder{})
	require.Error(t, err,
		"missing -a file should return read error")
	require.ErrorContains(t, err, "reading -a benchmark file",
		"error should mention -a read failure")
}

func TestRunCLIABFilesBReadError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fastPath := filepath.Join(dir, "fast.txt")
	mustWriteFile(t, fastPath, strings.Repeat("BenchmarkExampleFast-10 100 1 ns/op\n", 10))

	err := runCLI([]string{"-a", fastPath, "-b", "missing-b.txt"}, strings.NewReader(""), &strings.Builder{})
	require.Error(t, err,
		"missing -b file should return read error")
	require.ErrorContains(t, err, "reading -b benchmark file",
		"error should mention -b read failure")
}

func TestRunCLIABFilesParseError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fastPath := filepath.Join(dir, "fast.txt")
	slowPath := filepath.Join(dir, "slow.txt")

	mustWriteFile(t, fastPath, strings.Repeat("x", 70*1024))
	mustWriteFile(t, slowPath, strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10))

	err := runCLI([]string{"-a", fastPath, "-b", slowPath}, strings.NewReader(""), &strings.Builder{})
	require.Error(t, err,
		"invalid raw benchmark file should return parse error")

	for _, want := range []string{"parsing input", "scanning raw benchmark file input"} {
		require.ErrorContains(t, err, want,
			"parse error should include expected context")
	}
}

func TestRunCLIAlternativesScannerErrorIsParseError(t *testing.T) {
	t.Parallel()

	input := "BenchmarkEnhance/original-10 100 " + strings.Repeat("1", 70*1024) + " ns/op\n"

	err := runCLI([]string{flagMode, modeAlt}, strings.NewReader(input), &strings.Builder{})
	require.Error(t, err,
		"scanner failure should return parse error")

	for _, want := range []string{"parsing input", "scanning raw alternatives input"} {
		require.ErrorContains(t, err, want,
			"scanner error should include expected context")
	}
}

func TestRunCLIReadErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI(nil, failingReader{}, &strings.Builder{})
	require.Error(t, err,
		"failing reader should return parse error")
	require.ErrorContains(t, err, "parsing input",
		"read error should include parsing context")
}

func TestRunCLIRawStdinInsufficientSamples(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
`

	err := runCLI(nil, strings.NewReader(input), &strings.Builder{})
	require.Error(t, err,
		"insufficient samples should return CLI error")

	for _, want := range []string{
		"insufficient samples",
		fmt.Sprintf("at least %d samples", verdict.RawComparisonMinSamples),
		fmt.Sprintf("-count=%d or more", verdict.RecommendedRawSamples),
	} {
		require.ErrorContains(t, err, want,
			"insufficient sample message should include policy wording")
	}
}

func TestRunCLIRawSampleBoundaryAcceptsThreeSamples(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI(nil, strings.NewReader(strings.Repeat(
		"BenchmarkEnhance/original-10 100 10 ns/op\n"+
			"BenchmarkEnhance/enhanced-10 100 8 ns/op\n",
		verdict.RawComparisonMinSamples,
	)), &out)
	require.NoError(t, err,
		"minimum accepted raw samples should succeed")
	require.Contains(t, out.String(), "BenchmarkEnhance: enhanced wins",
		"output should report enhanced as winner")
}

func TestReportErrorBranches(t *testing.T) {
	t.Parallel()

	require.NoError(t, reportError(verdictReport("other")),
		"non-mapped reason code should not return error")

	err := reportError(verdictReport("insufficient-samples"))
	require.ErrorIs(t, err, errInsufficientSamples,
		"insufficient-samples reason should map to CLI error")

	//nolint:exhaustruct // Zero-value BenchmarkVerdict checks fallback labels.
	err = benchmarkSetMismatchError(verdict.BenchmarkVerdict{})
	require.ErrorContains(t, err, "verdict -a ./a.txt -b ./b.txt",
		"fallback labels should be used in mismatch guidance")
}

func TestSampleCountWordingUsesSharedPolicy(t *testing.T) {
	t.Parallel()

	err := insufficientSamplesError()
	for _, want := range sampleCountWording() {
		require.ErrorContains(t, err, want,
			"insufficient sample error should use shared wording")
	}

	help := flagHelpText()
	for _, want := range sampleCountWording() {
		require.Contains(t, help, want,
			"help text should use shared sample policy wording")
	}

	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	require.NoError(t, err,
		"failed to read README for wording check")

	for _, want := range sampleCountWording() {
		require.Contains(t, string(readme), want,
			"README should include shared sample policy wording")
	}
}

func sampleCountWording() []string {
	return []string{
		fmt.Sprintf("at least %d samples", verdict.RawComparisonMinSamples),
		fmt.Sprintf("-count=%d or more", verdict.RecommendedRawSamples),
	}
}

func TestRunCLIUnknownFormat(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagFormat, "xml"}, strings.NewReader(winningInput), &strings.Builder{})
	require.ErrorIs(t, err, errUnknownFormat,
		"unknown output format should return errUnknownFormat")
}

func TestRunCLITextWriteErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagFormat, formatText}, strings.NewReader(winningInput), failingWriter{})
	require.ErrorIs(t, err, errWritingOutput,
		"text writer failure should return output context")
}

func TestRunCLIJSONWriteErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagFormat, formatJSON}, strings.NewReader(winningInput), failingWriter{})
	require.ErrorIs(t, err, errWritingOutput,
		"json writer failure should return output context")
}

func TestRunCLIAlternativesMode(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagMode, modeAlt}, strings.NewReader(alternativesInput), &out)
	require.NoError(t, err,
		"failed to run alternatives mode")
	require.Contains(t, out.String(), "BenchmarkEnhance: enhanced wins",
		"alternatives mode should report enhanced winner")
}

func TestRunCLIAutoModeAlternatives(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI(nil, strings.NewReader(alternativesInput), &out)
	require.NoError(t, err,
		"failed to run auto mode with alternatives input")
	require.Equal(t, "BenchmarkEnhance: enhanced wins\n", out.String(),
		"auto mode should emit concise alternative verdict")
}

func TestRunCLIAlternativesModeWithCustomLabels(t *testing.T) {
	t.Parallel()

	input := strings.ReplaceAll(
		strings.ReplaceAll(alternativesInput, "original", "base"),
		"enhanced",
		"candidate",
	)

	var out strings.Builder

	err := runCLI(
		[]string{flagMode, modeAlt, "--baseline", "base", "--candidate", "candidate"},
		strings.NewReader(input),
		&out,
	)
	require.NoError(t, err,
		"failed to run alternatives mode with custom labels")
	require.Contains(t, out.String(), "BenchmarkEnhance: candidate wins",
		"custom labels should be reflected in winner output")
}

func TestRunCLIUnknownMode(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagMode, "sideways"}, strings.NewReader(winningInput), &strings.Builder{})
	require.ErrorIs(t, err, errUnknownMode,
		"unknown mode should return errUnknownMode")
}

// ----------------------------------------------------------------------------
//  Helper functions and test data
// ----------------------------------------------------------------------------

func mustWriteFile(t *testing.T, path, data string) {
	t.Helper()

	err := os.WriteFile(path, []byte(data), 0o600)
	if err != nil {
		require.NoError(t, err,
			"failed to write fixture file")
	}
}

func verdictReport(reasonCode string) verdict.Report {
	var zeroBenchmarkVerdict verdict.BenchmarkVerdict

	zeroBenchmarkVerdict.Benchmark = "all"
	zeroBenchmarkVerdict.Outcome = verdict.Inconclusive
	zeroBenchmarkVerdict.ReasonCode = reasonCode

	return verdict.Report{
		Verdicts: []verdict.BenchmarkVerdict{zeroBenchmarkVerdict},
	}
}

func mockDebugReadBuildInfo(info *debug.BuildInfo, ok bool) func() {
	oldDebugReadBuildInfo := debugReadBuildInfo

	debugReadBuildInfo = func() (*debug.BuildInfo, bool) {
		return info, ok
	}

	return func() {
		debugReadBuildInfo = oldDebugReadBuildInfo
	}
}

func testBuildInfo(version, revision string) *debug.BuildInfo {
	info := new(debug.BuildInfo)
	info.Main.Version = version

	if revision != "" {
		info.Settings = []debug.BuildSetting{
			{Key: vcsRevisionKey, Value: revision},
		}
	}

	return info
}
