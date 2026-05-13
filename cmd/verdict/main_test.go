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
	formatText   = "text"
	formatJSON   = "json"
	winningInput = "name          old time/op  new time/op  delta\n" +
		"Foo-8         10.0ns ± 1%   8.0ns ± 1%  -20.00% (p=0.001 n=10+10)\n"
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
