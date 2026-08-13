package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KEINOS/go-verdict/verdict"
	"github.com/stretchr/testify/require"
)

const (
	argExtra        = "extra"
	topicBenchstat  = "benchstat"
	flagFormat      = "--format"
	flagAlpha       = "--alpha"
	flagMinDelta    = "--min-delta"
	flagMode        = "--mode"
	flagRequire     = "--require"
	formatText      = "text"
	removedMode     = "alternatives"
	optionAlpha     = "alpha"
	optionMinDelta  = "min-delta"
	winningInput    = "name          old time/op  new time/op  delta\n" +
		"Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=0.001 n=10+10)\n"
	goTestBenchInput = "BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/original-10 100 10 ns/op 8 B/op 1 allocs/op\n" +
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n"
)

var (
	errTestWrite  = errors.New("test write error")
	errTestSubcmd = errors.New("test subcommand error")
)

// ----------------------------------------------------------------------------
//  Data Providers
// ----------------------------------------------------------------------------

// Data Provider for consistent **sample count** wording.
//
//nolint:gochecknoglobals // required for shared test data provider
var sampleCountWording = []string{
	fmt.Sprintf("at least %d samples", verdict.RawComparisonMinSamples),
	fmt.Sprintf("-count=%d or more", verdict.RecommendedRawSamples),
}

// ----------------------------------------------------------------------------
//  Unit Tests
// ----------------------------------------------------------------------------

//nolint:paralleltest // Uses process-wide mocks (os.Args and osExit).
func Test_main_fail(t *testing.T) {
	// Backup and restore
	oldOsExit := osExit
	oldOsArgs := os.Args

	t.Cleanup(func() {
		osExit = oldOsExit
		os.Args = oldOsArgs
	})

	// Mock os.Exit to panic instead of os.Exit
	osExit = func(code int) {
		if code != 0 {
			//nolint:err113 // allow dynamic err for test
			panic(fmt.Errorf("exit with code %d", code))
		}
	}

	// Mock command args
	os.Args = []string{
		t.Name(),
		"--invalid-flag",
	}

	// Require panic due to invalid flag
	expectContains := "exit with code 1" // mocked err msg
	require.PanicsWithError(t, expectContains, func() { main() },
		"invalid flag should error with msg")
}

func TestRunCLITextFormat(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI(
		[]string{flagFormat, formatText},
		strings.NewReader(winningInput),
		&out,
	)
	require.NoError(t, err,
		"failed to run CLI in text format")

	require.Contains(t, out.String(), "Foo-8: new wins",
		"output should include benchmark verdict")
}

func TestRunCLIVerboseTextFormat(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI(
		[]string{"--verbose"},
		strings.NewReader(winningInput),
		&out,
	)
	require.NoError(t, err,
		"failed to run CLI in verbose mode")

	got := out.String()

	for _, want := range []string{
		"Foo-8: new wins",
		"Pareto-superior",
		"+ sec/op",
	} {
		require.Contains(t, got, want,
			"verbose output should contain expected snippet")
	}
}

func TestRunCLIJSONFormat(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI(
		[]string{flagFormat, formatJSON},
		strings.NewReader(winningInput),
		&out,
	)
	require.NoError(t, err,
		"failed to run CLI in json format")

	require.Contains(t, out.String(), `"outcome": "new-wins"`,
		"json output should include new-wins outcome")
}

func TestRunCLIRequireAllowsMatchingOutcomes(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagRequire, string(verdict.NewWins)}, strings.NewReader(winningInput), &out)
	require.NoError(t, err,
		"matching required outcome should succeed")
	require.Contains(t, out.String(), "Foo-8: new wins",
		"report should still be written before require gate succeeds")
}

func TestRunCLIRequireRejectsNonMatchingOutcomesAfterReport(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagRequire, string(verdict.OldWins)}, strings.NewReader(winningInput), &out)
	require.ErrorIs(t, err, errRequiredOutcome,
		"non-matching required outcome should fail")
	require.ErrorContains(t, err, "Foo-8: new-wins",
		"error should name the benchmark and outcome that missed the gate")
	require.Contains(t, out.String(), "Foo-8: new wins",
		"report should be written before require gate fails")
}

func TestRunCLIRequireRejectsUnknownOutcome(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagRequire, "faster"}, strings.NewReader(winningInput), &strings.Builder{})
	require.ErrorIs(t, err, errUnknownRequired,
		"unknown required outcome should fail during flag initialization")
	require.ErrorContains(t, err, "new-wins",
		"error should list valid outcomes")
}

func TestRunCLIMinDeltaDefaultAndExplicitZero(t *testing.T) {
	t.Parallel()

	input := "name          old time/op  new time/op  delta\n" +
		"Foo-8         10.0ns ± 1%   9.9ns ± 1%  -1.00% (p=0.001 n=10+10)\n"

	var defaultOut strings.Builder

	err := runCLI(nil, strings.NewReader(input), &defaultOut)
	require.NoError(t, err,
		"default min-delta should parse successfully")
	require.Equal(t, "Foo-8: tie\n", defaultOut.String(),
		"default min-delta should treat a 1% change as tie")

	var zeroOut strings.Builder

	err = runCLI([]string{flagMinDelta, "0"}, strings.NewReader(input), &zeroOut)
	require.NoError(t, err,
		"explicit zero min-delta should parse successfully")
	require.Equal(t, "Foo-8: new wins\n", zeroOut.String(),
		"explicit zero min-delta should count the 1% significant change")
}

func TestRunCLIHelpWritesSamplePolicy(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI(
		[]string{flagHelpLong},
		strings.NewReader(""),
		&out,
	)
	require.NoError(t, err,
		"failed to print help output")

	require.Contains(t, out.String(),
		AppDescription,
		"help opening sentence should match app description")

	for _, want := range sampleCountWording {
		require.Contains(t, out.String(), want,
			"help output should include shared sample policy wording")
	}

	for _, want := range []string{
		"Usage:\n  verdict [command] [options]",
		"Commands:\n  help [topic]",
		"  hotspot <package>",
		"  skill",
		"  version",
		"Options:\n  --format text|json",
		"Hotspot options:\n  --bench regexp",
		"  -v, --version",
	} {
		require.Contains(t, out.String(), want,
			"help output should include command and option sections")
	}
}

func TestRunCLIHelpExplainsInputModes(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagHelpLong}, strings.NewReader(""), &out)
	require.NoError(t, err,
		"failed to print help output")

	for _, want := range []string{
		"auto: detect benchstat output or raw go test -bench output.",
		"benchstat: read already-compared benchstat text or CSV.",
		"gotestbench: compare raw go test -bench sub-benchmarks, such as original vs enhanced.",
	} {
		require.Contains(t, out.String(), want,
			"help output should explain each input mode")
	}
}

func TestRunCLIHotspotHelp(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{cmdHotspot, flagHelpLong}, strings.NewReader("ignored"), &out)
	require.NoError(t, err,
		"hotspot help should succeed")
	require.Contains(t, out.String(), "Usage:\n  verdict hotspot",
		"hotspot help should include subcommand usage")
}

func TestRunCLIHotspotWrapsCommandErrors(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{cmdHotspot, "--no-such-flag", "./pkg"}, strings.NewReader("ignored"), &strings.Builder{})
	require.ErrorContains(t, err, "running hotspot command",
		"the CLI names the command that failed")
	require.ErrorContains(t, err, "parsing hotspot flags")
}

func TestRunCLIHelpCommandListsTopics(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{cmdHelp}, strings.NewReader("ignored"), &out)
	require.NoError(t, err,
		"plain help command should succeed")

	for _, want := range []string{
		"Workflow help topics:",
		"bootstrap",
		"hotspot",
		topicBenchstat,
		"gotestbench",
		"results",
		"verdict help <topic>",
	} {
		require.Contains(t, out.String(), want,
			"help index should list workflow topics")
	}
}

func TestRunCLIHelpCommandPrintsTopic(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{cmdHelp, topicBenchstat}, strings.NewReader("ignored"), &out)
	require.NoError(t, err,
		"help topic command should succeed")
	require.Contains(t, out.String(), "benchstat old.txt new.txt | verdict",
		"benchstat topic should include the before/after pipeline")
}

func TestRunCLIHelpCommandUnknownTopic(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{cmdHelp, "no-such-topic"}, strings.NewReader("ignored"), &strings.Builder{})
	require.Error(t, err,
		"unknown topic should return an error")
	require.ErrorContains(t, err, "unknown help topic",
		"unknown topic error should name the failure")
	require.ErrorContains(t, err, "bootstrap",
		"unknown topic error should list available topics")
}

func TestRunCLIHelpCommandRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{cmdHelp, topicBenchstat, argExtra}, strings.NewReader("ignored"), &strings.Builder{})
	require.ErrorIs(t, err, errUnexpectedCommandArgs,
		"extra args should return 'errUnexpectedCommandArgs' error")
}

func TestRunCLIVersionRequests(t *testing.T) {
	t.Parallel()

	var commandOut strings.Builder

	err := runCLI([]string{cmdVersion}, strings.NewReader("ignored"), &commandOut)
	require.NoError(t, err,
		"version command should succeed")
	require.NotEmpty(t, commandOut.String(),
		"version output should not be empty")

	for _, args := range [][]string{
		{flagVersionLong},
		{flagVersionShort},
	} {
		var out strings.Builder

		err = runCLI(args, strings.NewReader("ignored"), &out)
		require.NoError(t, err,
			"version request should succeed")
		require.Equal(t, commandOut.String(), out.String(),
			"version command and flags should use the same subcommand output")
	}
}

func TestRunCLIVersionRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{cmdVersion, argExtra}, strings.NewReader("ignored"), &strings.Builder{})
	require.ErrorIs(t, err, errUnexpectedCommandArgs,
		"extra args should return 'errUnexpectedCommandArgs' error")
}

func TestRunSubcmdErrorBranches(t *testing.T) {
	t.Parallel()

	err := runSubcmd(nil, &strings.Builder{}, false)
	require.ErrorIs(t, err, errUnknownCommand,
		"nil subcommand should return unknown command")

	err = runSubcmd(failingSubcmd{}, &strings.Builder{}, false)
	require.ErrorIs(t, err, errTestSubcmd,
		"subcommand errors should pass through")
}

func TestRunCLIHelpWriteErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagHelpLong}, strings.NewReader(""), failingWriter{})
	require.ErrorIs(t, err, errWritingOutput,
		"help write errors should be wrapped with output context")
}

func TestRunCLISkillMatchesCanonicalSkill(t *testing.T) {
	t.Parallel()

	want, err := os.ReadFile(filepath.Join("internal", "skill", "SKILL.md"))
	require.NoError(t, err,
		"failed to read canonical skill file")

	var out strings.Builder

	err = runCLI([]string{cmdSkill}, strings.NewReader("ignored"), &out)
	require.NoError(t, err,
		"failed to run skill command")
	require.Equal(t, string(want), out.String(),
		"skill output does not match canonical SKILL.md")
}

func TestRunCLISkillNilOutputContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{cmdSkill}, strings.NewReader("ignored"), nil)
	require.ErrorIs(t, err, errWritingOutput,
		"nil output should return 'errWritingOutput' error")
}

func TestRunCLISkillRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{cmdSkill, argExtra}, strings.NewReader("ignored"), &strings.Builder{})
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
		want string
		args []string
	}{
		{name: "zero alpha", args: []string{flagAlpha, "0"}, want: optionAlpha},
		{name: "negative alpha", args: []string{flagAlpha, "-0.1"}, want: optionAlpha},
		{name: "alpha above one", args: []string{flagAlpha, "1.1"}, want: optionAlpha},
		{name: "NaN alpha", args: []string{flagAlpha, "NaN"}, want: optionAlpha},
		{name: "Inf alpha", args: []string{flagAlpha, "Inf"}, want: optionAlpha},
		{name: "negative Inf alpha", args: []string{flagAlpha, "-Inf"}, want: optionAlpha},
		{name: "negative min delta", args: []string{flagMinDelta, "-1"}, want: optionMinDelta},
		{name: "NaN min delta", args: []string{flagMinDelta, "NaN"}, want: optionMinDelta},
		{name: "Inf min delta", args: []string{flagMinDelta, "Inf"}, want: optionMinDelta},
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

func TestRunCLIGoTestBenchScannerErrorIsParseError(t *testing.T) {
	t.Parallel()

	input := "BenchmarkEnhance/original-10 100 " + strings.Repeat("1", 70*1024) + " ns/op\n"

	err := runCLI([]string{flagMode, verdict.ModeGoTestBench}, strings.NewReader(input), &strings.Builder{})
	require.Error(t, err,
		"scanner failure should return parse error")

	for _, want := range []string{"parsing input", "scanning raw go test -bench input"} {
		require.ErrorContains(t, err, want,
			"scanner error should include expected context")
	}
}

func TestRunCLIEmptyStdinReturnsGuidance(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", " \n\t\n"} {
		err := runCLI(nil, strings.NewReader(input), &strings.Builder{})
		require.ErrorIs(t, err, errNoStdinInput,
			"blank stdin should return the no-input error")

		for _, want := range []string{
			"benchstat old.txt new.txt | verdict",
			"verdict help",
		} {
			require.ErrorContains(t, err, want,
				"no-input error should include actionable guidance")
		}
	}
}

func TestRunCLITerminalStdinReturnsGuidance(t *testing.T) {
	t.Parallel()

	// /dev/null is a character device, like an interactive terminal, so the
	// CLI must fail fast instead of blocking on a read that never ends.
	devNull, err := os.Open(os.DevNull)
	require.NoError(t, err,
		"failed to open the null device")

	t.Cleanup(func() { _ = devNull.Close() })

	err = runCLI(nil, devNull, &strings.Builder{})
	require.ErrorIs(t, err, errNoStdinInput,
		"character-device stdin should return the no-input error")
}

func TestIsInteractiveTerminal(t *testing.T) {
	t.Parallel()

	require.False(t, isInteractiveTerminal(strings.NewReader("data")),
		"plain readers should not count as interactive terminals")

	regular, err := os.CreateTemp(t.TempDir(), "stdin-*.txt")
	require.NoError(t, err,
		"failed to create temp file")

	t.Cleanup(func() { _ = regular.Close() })

	require.False(t, isInteractiveTerminal(regular),
		"regular files should not count as interactive terminals")

	devNull, err := os.Open(os.DevNull)
	require.NoError(t, err,
		"failed to open the null device")

	t.Cleanup(func() { _ = devNull.Close() })

	require.True(t, isInteractiveTerminal(devNull),
		"character devices should count as interactive terminals")

	require.False(t, isInteractiveTerminal(failingStatter{}),
		"stat failures should fall back to non-interactive")
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

	tests := map[string]error{
		verdict.ReasonBenchmarkSetMismatch: errBenchmarkSetMismatch,
		verdict.ReasonInsufficientSamples:  errInsufficientSamples,
	}

	for _, handler := range reasonCodeErrorHandlers() {
		wantErr, ok := tests[handler.reasonCode]
		require.True(t, ok,
			"mapped reason code should have an explicit CLI expectation")

		test := struct {
			wantErr    error
			reasonCode string
		}{
			reasonCode: handler.reasonCode,
			wantErr:    wantErr,
		}

		t.Run(test.reasonCode, func(t *testing.T) {
			t.Parallel()

			err := reportError(verdictReport(test.reasonCode))
			require.ErrorIs(t, err, test.wantErr,
				"reason code should map to expected CLI error")
		})
	}

	require.Len(t, tests, len(reasonCodeErrorHandlers()),
		"all expected CLI reason codes should have handlers")

	//nolint:exhaustruct // Zero-value BenchmarkVerdict checks fallback labels.
	err := benchmarkSetMismatchError(verdict.BenchmarkVerdict{})
	require.ErrorContains(t, err, "verdict -a ./a.txt -b ./b.txt",
		"fallback labels should be used in mismatch guidance")
}

func TestSampleCountWordingUsesSharedPolicy(t *testing.T) {
	t.Parallel()

	err := insufficientSamplesError()
	for _, want := range sampleCountWording {
		require.ErrorContains(t, err, want,
			"insufficient sample error should use shared wording")
	}

	help := flagHelpText()
	for _, want := range sampleCountWording {
		require.Contains(t, help, want,
			"help text should use shared sample policy wording")
	}

	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	require.NoError(t, err,
		"failed to read README for wording check")

	for _, want := range sampleCountWording {
		require.Contains(t, string(readme), want,
			"README should include shared sample policy wording")
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

func TestRunCLIGoTestBenchMode(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagMode, verdict.ModeGoTestBench}, strings.NewReader(goTestBenchInput), &out)
	require.NoError(t, err,
		"failed to run gotestbench mode")
	require.Contains(t, out.String(), "BenchmarkEnhance: enhanced wins",
		"gotestbench mode should report enhanced winner")
}

func TestRunCLIAutoModeGoTestBench(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI(nil, strings.NewReader(goTestBenchInput), &out)
	require.NoError(t, err,
		"failed to run auto mode with raw go test -bench input")
	require.Equal(t, "BenchmarkEnhance: enhanced wins\n", out.String(),
		"auto mode should emit concise alternative verdict")
}

func TestRunCLIGoTestBenchModeWithCustomLabels(t *testing.T) {
	t.Parallel()

	input := strings.ReplaceAll(
		strings.ReplaceAll(goTestBenchInput, "original", "base"),
		"enhanced",
		"candidate",
	)

	var out strings.Builder

	err := runCLI(
		[]string{flagMode, verdict.ModeGoTestBench, "--baseline", "base", "--candidate", "candidate"},
		strings.NewReader(input),
		&out,
	)
	require.NoError(t, err,
		"failed to run gotestbench mode with custom labels")
	require.Contains(t, out.String(), "BenchmarkEnhance: candidate wins",
		"custom labels should be reflected in winner output")
}

func TestRunCLIUnknownMode(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagMode, "sideways"}, strings.NewReader(winningInput), &strings.Builder{})
	require.ErrorIs(t, err, errUnknownMode,
		"unknown mode should return errUnknownMode")
}

func TestRunCLIRemovedMode(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagMode, removedMode}, strings.NewReader(goTestBenchInput), &strings.Builder{})
	require.Equal(t, errUnknownMode, err,
		"removed alternatives mode should return the exact unknown mode error")
}

// ----------------------------------------------------------------------------
//  Helper functions and test data
// ----------------------------------------------------------------------------

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errTestWrite
}

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, errTestWrite
}

type failingStatter struct{}

func (failingStatter) Read(_ []byte) (int, error) {
	return 0, errTestWrite
}

func (failingStatter) Stat() (os.FileInfo, error) {
	return nil, errTestWrite
}

type failingSubcmd struct{}

func (failingSubcmd) Run() (string, error) {
	return "", errTestSubcmd
}

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
