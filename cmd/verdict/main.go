// Package main provides the verdict command-line tool.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/KEINOS/go-verdict/verdict"
)

const (
	alphaDefault       = 0.05
	formatDefault      = "text"
	minDeltaPctDefault = 0.0
)

// Mockable variables for testing.
//
//nolint:gochecknoglobals // allow global variables for testing purposes
var (
	osExit = os.Exit
)

// Pre-defined errors.
var (
	errUnknownFormat = errors.New("unknown format")
	errParsingInput  = errors.New("parsing input")
	errParsingFlags  = errors.New("parsing flags")
	errWritingOutput = errors.New("writing output")
)

func main() {
	exitOnError(run())
}

func run() error {
	return runCLI(os.Args[1:], os.Stdin, os.Stdout)
}

func runCLI(args []string, input io.Reader, output io.Writer) error {
	opts, outputFormat, err := initialize(args)
	if err != nil {
		return err
	}

	report, err := verdict.Parse(input, *opts)
	if err != nil {
		return fmt.Errorf("%w: %w", errParsingInput, err)
	}

	return writeReport(report, outputFormat, output)
}

func writeReport(report verdict.Report, outputFormat string, output io.Writer) error {
	switch outputFormat {
	case formatDefault:
		err := report.WriteText(output)
		if err != nil {
			return fmt.Errorf("%w: %w", errWritingOutput, err)
		}
	case "json":
		err := report.WriteJSON(output)
		if err != nil {
			return fmt.Errorf("%w: %w", errWritingOutput, err)
		}
	default:
		return errUnknownFormat
	}

	return nil
}

func initialize(args []string) (*verdict.Options, string, error) {
	var opts verdict.Options

	outputFormat := formatDefault
	flagSet := flag.NewFlagSet("verdict", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	flagSet.StringVar(&outputFormat,
		"format", formatDefault,
		"output format: text or json")
	flagSet.Float64Var(&opts.Alpha,
		"alpha", alphaDefault,
		"p-value threshold")
	flagSet.Float64Var(&opts.MinDeltaPct,
		"min-delta", minDeltaPctDefault,
		"minimum absolute delta percentage treated as practical difference")

	err := flagSet.Parse(args)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", errParsingFlags, err)
	}

	return &opts, outputFormat, nil
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		osExit(1)
	}
}
