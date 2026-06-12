package main

import (
	"flag"
	"fmt"
	"io"
	"math"

	"github.com/KEINOS/go-verdict/verdict"
)

const (
	flagHelpLong     = "--help"
	flagHelpShort    = "-h"
	flagVersionLong  = "--version"
	flagVersionShort = "-v"
	formatDefault    = "text"
)

type cliOptions struct {
	aPath        string
	bPath        string
	outputFormat string
	verbose      bool
}

func initialize(args []string) (*verdict.Options, cliOptions, error) {
	opts := verdict.NewOptions()

	var cliOpts cliOptions

	cliOpts.outputFormat = formatDefault
	flagSet := flag.NewFlagSet("verdict", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	flagSet.StringVar(&cliOpts.outputFormat,
		"format", formatDefault,
		"output format: text or json")
	flagSet.StringVar(&cliOpts.aPath,
		"a", "",
		"raw benchmark file for side A")
	flagSet.StringVar(&cliOpts.bPath,
		"b", "",
		"raw benchmark file for side B")
	flagSet.StringVar(&opts.Mode,
		"mode", opts.Mode,
		"input mode: auto, benchstat, or gotestbench")
	flagSet.StringVar(&opts.Baseline,
		"baseline", "",
		"baseline sub-benchmark name for gotestbench mode")
	flagSet.StringVar(&opts.Candidate,
		"candidate", "",
		"candidate sub-benchmark name for gotestbench mode")
	flagSet.Float64Var(&opts.Alpha,
		"alpha", opts.Alpha,
		"p-value threshold")
	flagSet.Float64Var(&opts.MinDeltaPct,
		"min-delta", opts.MinDeltaPct,
		"minimum absolute delta percentage treated as practical difference")
	flagSet.BoolVar(&cliOpts.verbose,
		"verbose", false,
		"include verdict reason and metric details in text output")
	flagSet.Usage = func() {
		_, _ = fmt.Fprint(flagSet.Output(), flagHelpText())
	}

	err := flagSet.Parse(args)
	if err != nil {
		return nil, cliOptions{}, fmt.Errorf("%w: %w (run 'verdict --help' for usage)", errParsingFlags, err)
	}

	if opts.Mode != verdict.ModeAuto && opts.Mode != verdict.ModeBenchstat && opts.Mode != verdict.ModeGoTestBench {
		return nil, cliOptions{}, errUnknownMode
	}

	err = validateCLIOptions(opts)
	if err != nil {
		return nil, cliOptions{}, err
	}

	return &opts, cliOpts, nil
}

func validateCLIOptions(opts verdict.Options) error {
	switch {
	case math.IsNaN(opts.Alpha) || math.IsInf(opts.Alpha, 0) || opts.Alpha <= 0 || opts.Alpha > 1:
		return errInvalidAlpha
	case math.IsNaN(opts.MinDeltaPct) || math.IsInf(opts.MinDeltaPct, 0) || opts.MinDeltaPct < 0:
		return errInvalidMinDelta
	default:
		return nil
	}
}

func isHelpRequest(args []string) bool {
	return len(args) == 1 && (args[0] == flagHelpShort || args[0] == flagHelpLong)
}

func isVersionRequest(args []string) bool {
	return len(args) == 1 && (args[0] == flagVersionShort || args[0] == flagVersionLong)
}
