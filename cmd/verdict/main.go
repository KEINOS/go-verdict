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
	baselineDefault    = "original"
	candidateDefault   = "enhanced"
	formatDefault      = "text"
	minDeltaPctDefault = 0.0
	modeDefault        = "benchstat"
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
	errUnknownMode   = errors.New("unknown mode")
	errWritingOutput = errors.New("writing output")
)

func main() {
	exitOnError(run())
}

func run() error {
	return runCLI(os.Args[1:], os.Stdin, os.Stdout)
}

func runCLI(args []string, input io.Reader, output io.Writer) error {
	opts, cliOpts, err := initialize(args)
	if err != nil {
		return err
	}

	report, err := verdict.Parse(input, *opts)
	if err != nil {
		return fmt.Errorf("%w: %w", errParsingInput, err)
	}

	return writeReport(report, cliOpts.outputFormat, output)
}

type cliOptions struct {
	outputFormat string
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

func initialize(args []string) (*verdict.Options, cliOptions, error) {
	var opts verdict.Options

	var cliOpts cliOptions

	cliOpts.outputFormat = formatDefault
	flagSet := flag.NewFlagSet("verdict", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	flagSet.StringVar(&cliOpts.outputFormat,
		"format", formatDefault,
		"output format: text or json")
	flagSet.StringVar(&opts.Mode,
		"mode", modeDefault,
		"input mode: benchstat or alternatives")
	flagSet.StringVar(&opts.Baseline,
		"baseline", baselineDefault,
		"baseline sub-benchmark name for alternatives mode")
	flagSet.StringVar(&opts.Candidate,
		"candidate", candidateDefault,
		"candidate sub-benchmark name for alternatives mode")
	flagSet.Float64Var(&opts.Alpha,
		"alpha", alphaDefault,
		"p-value threshold")
	flagSet.Float64Var(&opts.MinDeltaPct,
		"min-delta", minDeltaPctDefault,
		"minimum absolute delta percentage treated as practical difference")

	err := flagSet.Parse(args)
	if err != nil {
		return nil, cliOptions{}, fmt.Errorf("%w: %w", errParsingFlags, err)
	}

	if opts.Mode != modeDefault && opts.Mode != "alternatives" {
		return nil, cliOptions{}, errUnknownMode
	}

	return &opts, cliOpts, nil
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		osExit(1)
	}
}
