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
	flagFormat   = "--format"
	flagMode     = "--mode"
	formatText   = "text"
	formatJSON   = "json"
	modeAlt      = "alternatives"
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

//nolint:paralleltest // disable due to mocking during test
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
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "Foo-8: new wins") {
		t.Fatalf("output = %q, want benchmark verdict", out.String())
	}
}

func TestRunCLIVerboseTextFormat(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{"--verbose"}, strings.NewReader(winningInput), &out)
	if err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{"Foo-8: new wins", "Pareto-superior", "+ sec/op"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

func TestRunCLIJSONFormat(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagFormat, formatJSON}, strings.NewReader(winningInput), &out)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), `"outcome": "new-wins"`) {
		t.Fatalf("output = %q, want json outcome", out.String())
	}
}

func TestRunCLIParseErrorContainsContext(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagFormat, formatText}, strings.NewReader("invalid"), &out)
	if err == nil {
		t.Fatal("expected parse error")
	}

	if !strings.Contains(err.Error(), "parsing input") {
		t.Fatalf("error = %q, want parsing input context", err.Error())
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
	if err == nil {
		t.Fatal("expected mismatch guidance error")
	}

	got := err.Error()
	for _, want := range []string{"benchmark names differ", "verdict -a ./fast.txt -b ./slow.txt"} {
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want %q", got, want)
		}
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
	if err != nil {
		t.Fatal(err)
	}

	if out.String() != "BenchmarkExampleFast_vs_BenchmarkExampleSlow: BenchmarkExampleFast wins\n" {
		t.Fatalf("output = %q, want fast winner", out.String())
	}
}

func TestRunCLIABFilesRequireBothSides(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{"-a", "fast.txt"}, strings.NewReader(""), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "use both -a and -b") {
		t.Fatalf("error = %v, want both-side error", err)
	}
}

func TestRunCLIABFilesReadError(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{"-a", "missing-a.txt", "-b", "missing-b.txt"}, strings.NewReader(""), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "reading -a benchmark file") {
		t.Fatalf("error = %v, want file read error", err)
	}
}

func TestRunCLIABFilesBReadError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fastPath := filepath.Join(dir, "fast.txt")
	mustWriteFile(t, fastPath, strings.Repeat("BenchmarkExampleFast-10 100 1 ns/op\n", 10))

	err := runCLI([]string{"-a", fastPath, "-b", "missing-b.txt"}, strings.NewReader(""), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "reading -b benchmark file") {
		t.Fatalf("error = %v, want b file read error", err)
	}
}

func TestRunCLIABFilesParseError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fastPath := filepath.Join(dir, "fast.txt")
	slowPath := filepath.Join(dir, "slow.txt")

	mustWriteFile(t, fastPath, strings.Repeat("x", 70*1024))
	mustWriteFile(t, slowPath, strings.Repeat("BenchmarkExampleSlow-10 100 10 ns/op\n", 10))

	err := runCLI([]string{"-a", fastPath, "-b", slowPath}, strings.NewReader(""), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "parsing input") {
		t.Fatalf("error = %v, want parse error", err)
	}
}

func TestRunCLIReadErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI(nil, failingReader{}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "parsing input") {
		t.Fatalf("error = %v, want parsing input", err)
	}
}

func TestRunCLIRawStdinInsufficientSamples(t *testing.T) {
	t.Parallel()

	input := `BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
BenchmarkEnhance/original-10 100 10 ns/op
BenchmarkEnhance/enhanced-10 100 8 ns/op
`

	err := runCLI(nil, strings.NewReader(input), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "insufficient samples") {
		t.Fatalf("error = %v, want insufficient samples", err)
	}
}

func TestReportErrorBranches(t *testing.T) {
	t.Parallel()

	if err := reportError(verdictReport("other")); err != nil {
		t.Fatalf("error = %v, want nil", err)
	}

	err := reportError(verdictReport("insufficient-samples"))
	if !errors.Is(err, errInsufficientSamples) {
		t.Fatalf("error = %v, want insufficient samples", err)
	}

	err = benchmarkSetMismatchError(verdict.BenchmarkVerdict{})
	if !strings.Contains(err.Error(), "verdict -a ./a.txt -b ./b.txt") {
		t.Fatalf("error = %q, want fallback labels", err.Error())
	}

	if hasTooFewRawSamples("BenchmarkFoo/original-10\nnot benchmark\n") {
		t.Fatal("short raw row should be ignored")
	}
}

func TestRunCLIUnknownFormat(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagFormat, "xml"}, strings.NewReader(winningInput), &strings.Builder{})
	if !errors.Is(err, errUnknownFormat) {
		t.Fatalf("error = %v, want %v", err, errUnknownFormat)
	}
}

func TestRunCLITextWriteErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagFormat, formatText}, strings.NewReader(winningInput), failingWriter{})
	if !errors.Is(err, errWritingOutput) {
		t.Fatalf("error = %v, want %v", err, errWritingOutput)
	}
}

func TestRunCLIJSONWriteErrorContainsContext(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagFormat, formatJSON}, strings.NewReader(winningInput), failingWriter{})
	if !errors.Is(err, errWritingOutput) {
		t.Fatalf("error = %v, want %v", err, errWritingOutput)
	}
}

func TestRunCLIAlternativesMode(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI([]string{flagMode, modeAlt}, strings.NewReader(alternativesInput), &out)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "BenchmarkEnhance: enhanced wins") {
		t.Fatalf("output = %q, want alternative verdict", out.String())
	}
}

func TestRunCLIAutoModeAlternatives(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	err := runCLI(nil, strings.NewReader(alternativesInput), &out)
	if err != nil {
		t.Fatal(err)
	}

	if out.String() != "BenchmarkEnhance: enhanced wins\n" {
		t.Fatalf("output = %q, want auto alternative verdict", out.String())
	}
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
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "BenchmarkEnhance: candidate wins") {
		t.Fatalf("output = %q, want custom-label alternative verdict", out.String())
	}
}

func TestRunCLIUnknownMode(t *testing.T) {
	t.Parallel()

	err := runCLI([]string{flagMode, "sideways"}, strings.NewReader(winningInput), &strings.Builder{})
	if !errors.Is(err, errUnknownMode) {
		t.Fatalf("error = %v, want %v", err, errUnknownMode)
	}
}

func mustWriteFile(t *testing.T, path, data string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func verdictReport(reasonCode string) verdict.Report {
	return verdict.Report{
		Verdicts: []verdict.BenchmarkVerdict{{
			Benchmark:  "all",
			Outcome:    verdict.Inconclusive,
			ReasonCode: reasonCode,
		}},
	}
}
