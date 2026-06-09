package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/KEINOS/go-verdict/verdict"
)

// Pre-defined errors.
var (
	errBenchmarkSetMismatch  = errors.New("inconclusive: benchmark names differ")
	errInsufficientSamples   = errors.New("insufficient samples")
	errUnexpectedCommandArgs = errors.New("command does not accept extra arguments")
	errUnknownCommand        = errors.New("unknown command")
	errUseBothAB             = errors.New("use both -a and -b to compare raw benchmark files")
	errUnknownFormat         = errors.New("unknown format")
	errParsingInput          = errors.New("parsing input")
	errParsingFlags          = errors.New("parsing flags")
	errInvalidAlpha          = errors.New("alpha must be greater than 0 and at most 1")
	errInvalidMinDelta       = errors.New("min-delta must be finite and non-negative")
	errUnknownMode           = errors.New("unknown mode")
	errWritingOutput         = errors.New("writing output")
)

type reasonCodeErrorHandler struct {
	handle     func(verdict.BenchmarkVerdict) error
	reasonCode string
}

func reasonCodeErrorHandlers() []reasonCodeErrorHandler {
	return []reasonCodeErrorHandler{
		{
			reasonCode: verdict.ReasonBenchmarkSetMismatch,
			handle:     benchmarkSetMismatchError,
		},
		{
			reasonCode: verdict.ReasonInsufficientSamples,
			handle: func(verdict.BenchmarkVerdict) error {
				return insufficientSamplesError()
			},
		},
	}
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

	for _, handler := range reasonCodeErrorHandlers() {
		if handler.reasonCode == report.Verdicts[0].ReasonCode {
			return handler.handle(report.Verdicts[0])
		}
	}

	return nil
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

func insufficientSamplesError() error {
	return fmt.Errorf(
		"%w: need at least %d samples per benchmark side; recommend -count=%d or more for stable results",
		errInsufficientSamples,
		verdict.RawComparisonMinSamples,
		verdict.RecommendedRawSamples,
	)
}

func suggestedPath(label string) string {
	if strings.HasPrefix(label, ".") || strings.HasPrefix(label, "/") {
		return label
	}

	return "./" + label
}
