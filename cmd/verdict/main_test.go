package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

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
		"BenchmarkEnhance/enhanced-10 100 8 ns/op 8 B/op 1 allocs/op\n"
)

var errTestWrite = errors.New("test write error")

type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
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

	if !strings.Contains(out.String(), "Foo-8: new-wins") {
		t.Fatalf("output = %q, want benchmark verdict", out.String())
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

	if !strings.Contains(out.String(), "BenchmarkEnhance: new-wins") {
		t.Fatalf("output = %q, want alternative verdict", out.String())
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

	if !strings.Contains(out.String(), "BenchmarkEnhance: new-wins") {
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
