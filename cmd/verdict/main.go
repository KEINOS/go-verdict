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
	formatDefault      = "text"
	minDeltaPctDefault = 0.0
	modeDefault        = "auto"
	rawCLIMinFields    = 2
	rawCLIMinSamples   = 3
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

	if hasTooFewRawSamples(string(inputBytes)) {
		return verdict.Report{}, insufficientSamplesError()
	}

	report, err := verdict.Parse(bytes.NewReader(inputBytes), *opts)
	if err != nil {
		return verdict.Report{}, fmt.Errorf("%w: %w", errParsingInput, err)
	}

	return report, nil
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
	return fmt.Errorf("%w: run benchmarks with -count=10 or more", errInsufficientSamples)
}

func hasTooFewRawSamples(input string) bool {
	counts := map[string]int{}

	for rawLine := range strings.SplitSeq(input, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "Benchmark") || !strings.Contains(line, "/") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < rawCLIMinFields {
			continue
		}

		counts[fields[0]]++
	}

	if len(counts) == 0 {
		return false
	}

	for _, count := range counts {
		if count < rawCLIMinSamples {
			return true
		}
	}

	return false
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

	err := flagSet.Parse(args)
	if err != nil {
		return nil, cliOptions{}, fmt.Errorf("%w: %w", errParsingFlags, err)
	}

	if opts.Mode != modeDefault && opts.Mode != "benchstat" && opts.Mode != "alternatives" {
		return nil, cliOptions{}, errUnknownMode
	}

	return &opts, cliOpts, nil
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		osExit(1)
	}
}
