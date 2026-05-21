// Package main provides the verdict command-line tool.
package main

import (
	"fmt"
	"io"
	"os"
)

// Mockable variable for testing.
//
//nolint:gochecknoglobals // Package-level hook is required for CLI-exit mocking.
var osExit = os.Exit

func main() {
	exitOnError(runCLI(os.Args[1:], os.Stdin, os.Stdout))
}

func runCLI(args []string, input io.Reader, output io.Writer) error {
	handled, err := runTopLevelCommand(args, output)
	if handled {
		return err
	}

	if isVersionRequest(args) {
		return runVersionCommand(output)
	}

	if isHelpRequest(args) {
		return writeString(output, flagHelpText())
	}

	opts, cliOpts, err := initialize(args)
	if err != nil {
		return err
	}

	report, err := buildReport(input, opts, cliOpts)
	if err != nil {
		return err
	}

	err = reportError(report)
	if err != nil {
		return err
	}

	return writeReport(report, cliOpts, output)
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		osExit(1)
	}
}
