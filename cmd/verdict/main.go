// Package main provides the verdict command-line tool.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/KEINOS/go-verdict/verdict"
)

const (
	alphaDefault       = 0.05
	flagHelpLong       = "--help"
	flagHelpShort      = "-h"
	formatDefault      = "text"
	minDeltaPctDefault = 0.0
	modeDefault        = "auto"
)

// Mockable variables for testing.
//
//nolint:gochecknoglobals // allow global variables for testing purposes
var (
	osExit = os.Exit
)

// Pre-defined errors.
var (
	errBenchmarkSetMismatch = errors.New("inconclusive: benchmark names differ")
	errInsufficientSamples  = errors.New("insufficient samples")
	errUseBothAB            = errors.New("use both -a and -b to compare raw benchmark files")
	errUnknownFormat        = errors.New("unknown format")
	errParsingInput         = errors.New("parsing input")
	errParsingFlags         = errors.New("parsing flags")
	errUnknownMode          = errors.New("unknown mode")
	errWritingOutput        = errors.New("writing output")
)

func main() {
	exitOnError(run())
}

func run() error {
	return runCLI(os.Args[1:], os.Stdin, os.Stdout)
}

func runCLI(args []string, input io.Reader, output io.Writer) error {
	if isHelpRequest(args) {
		_, err := fmt.Fprint(output, flagHelpText())
		if err != nil {
			return fmt.Errorf("%w: %w", errWritingOutput, err)
		}

		return nil
	}

	opts, cliOpts, err := initialize(args)
	if err != nil {
		return err
	}

	report, err := buildReport(input, opts, cliOpts)
	if err != nil {
		return err
	}

	if err := reportError(report); err != nil {
		return err
	}

	return writeReport(report, cliOpts, output)
}

type cliOptions struct {
	aPath        string
	bPath        string
	outputFormat string
	verbose      bool
}

func buildReport(input io.Reader, opts *verdict.Options, cliOpts cliOptions) (verdict.Report, error) {
	if cliOpts.aPath != "" || cliOpts.bPath != "" {
		return buildRawFileReport(opts, cliOpts)
	}

	inputBytes, err := io.ReadAll(input)
	if err != nil {
		return verdict.Report{}, fmt.Errorf("%w: %w", errParsingInput, err)
	}

	report, err := verdict.Parse(bytes.NewReader(inputBytes), *opts)
	if err != nil {
		return verdict.Report{}, fmt.Errorf("%w: %w", errParsingInput, err)
	}

	return report, nil
}

func isHelpRequest(args []string) bool {
	return len(args) == 1 && (args[0] == flagHelpShort || args[0] == flagHelpLong)
}

func buildRawFileReport(opts *verdict.Options, cliOpts cliOptions) (verdict.Report, error) {
	if cliOpts.aPath == "" || cliOpts.bPath == "" {
		return verdict.Report{}, errUseBothAB
	}

	aFile, err := os.Open(cliOpts.aPath)
	if err != nil {
		return verdict.Report{}, fmt.Errorf("reading -a benchmark file: %w", err)
	}
	defer func() { _ = aFile.Close() }()

	bFile, err := os.Open(cliOpts.bPath)
	if err != nil {
		return verdict.Report{}, fmt.Errorf("reading -b benchmark file: %w", err)
	}
	defer func() { _ = bFile.Close() }()

	report, err := verdict.CompareRawFiles(aFile, bFile, *opts)
	if err != nil {
		return verdict.Report{}, fmt.Errorf("%w: %w", errParsingInput, err)
	}

	return report, nil
}

func reportError(report verdict.Report) error {
	if len(report.Verdicts) != 1 || report.Verdicts[0].Outcome != verdict.Inconclusive {
		return nil
	}

	switch report.Verdicts[0].ReasonCode {
	case "benchmark-set-mismatch":
		return benchmarkSetMismatchError(report.Verdicts[0])
	case "insufficient-samples":
		return insufficientSamplesError()
	default:
		return nil
	}
}

func benchmarkSetMismatchError(item verdict.BenchmarkVerdict) error {
	aLabel := item.BaselineLabel
	bLabel := item.CandidateLabel

	if aLabel == "" {
		aLabel = "a.txt"
	}

	if bLabel == "" {
		bLabel = "b.txt"
	}

	return fmt.Errorf(
		"%w\nbenchstat compares the same benchmark before and after a change.\n"+
			"To compare two different benchmark functions as A/B alternatives, pass the raw benchmark files:\n"+
			"  verdict -a %s -b %s",
		errBenchmarkSetMismatch,
		suggestedPath(aLabel),
		suggestedPath(bLabel),
	)
}

func suggestedPath(label string) string {
	if strings.HasPrefix(label, ".") || strings.HasPrefix(label, "/") {
		return label
	}

	return "./" + label
}

func insufficientSamplesError() error {
	return fmt.Errorf(
		"%w: need at least %d samples per benchmark side; recommend -count=%d or more for stable results",
		errInsufficientSamples,
		verdict.RawComparisonMinSamples,
		verdict.RecommendedRawSamples,
	)
}

func writeReport(report verdict.Report, cliOpts cliOptions, output io.Writer) error {
	switch cliOpts.outputFormat {
	case formatDefault:
		write := report.WriteText
		if cliOpts.verbose {
			write = report.WriteVerboseText
		}

		err := write(output)
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
	flagSet.StringVar(&cliOpts.aPath,
		"a", "",
		"raw benchmark file for side A")
	flagSet.StringVar(&cliOpts.bPath,
		"b", "",
		"raw benchmark file for side B")
	flagSet.StringVar(&opts.Mode,
		"mode", modeDefault,
		"input mode: auto, benchstat, or alternatives")
	flagSet.StringVar(&opts.Baseline,
		"baseline", "",
		"baseline sub-benchmark name for alternatives mode")
	flagSet.StringVar(&opts.Candidate,
		"candidate", "",
		"candidate sub-benchmark name for alternatives mode")
	flagSet.Float64Var(&opts.Alpha,
		"alpha", alphaDefault,
		"p-value threshold")
	flagSet.Float64Var(&opts.MinDeltaPct,
		"min-delta", minDeltaPctDefault,
		"minimum absolute delta percentage treated as practical difference")
	flagSet.BoolVar(&cliOpts.verbose,
		"verbose", false,
		"include verdict reason and metric details in text output")
	flagSet.Usage = func() {
		_, _ = fmt.Fprint(flagSet.Output(), flagHelpText())
	}

	err := flagSet.Parse(args)
	if err != nil {
		return nil, cliOptions{}, fmt.Errorf("%w: %w", errParsingFlags, err)
	}

	if opts.Mode != modeDefault && opts.Mode != "benchstat" && opts.Mode != "alternatives" {
		return nil, cliOptions{}, errUnknownMode
	}

	return &opts, cliOpts, nil
}

func flagHelpText() string {
	return fmt.Sprintf(
		`Usage: verdict [options]

Raw benchmark comparisons need at least %d samples per benchmark side.
For stable results, run benchmarks with -count=%d or more.

Options:
  --format text|json
      Output format. Default: text.
  --mode auto|benchstat|alternatives
      Input mode. Default: auto.
  --verbose
      Include verdict reason and metric details in text output.
  -a file
      Raw benchmark file for side A.
  -b file
      Raw benchmark file for side B.
  --baseline name
      Baseline sub-benchmark name for alternatives mode.
  --candidate name
      Candidate sub-benchmark name for alternatives mode.
  --alpha value
      P-value threshold for statistical significance. Default: 0.05.
  --min-delta value
      Minimum absolute delta percentage to treat as a practical difference. Default: 0.0.
`,
		verdict.RawComparisonMinSamples,
		verdict.RecommendedRawSamples,
	)
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		osExit(1)
	}
}
