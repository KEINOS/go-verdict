package main

import (
	"fmt"
	"io"

	"github.com/KEINOS/go-verdict/verdict"
)

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

func writeString(output io.Writer, text string) error {
	if output == nil {
		return fmt.Errorf("%w: nil output writer", errWritingOutput)
	}

	_, err := fmt.Fprint(output, text)
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingOutput, err)
	}

	return nil
}

func flagHelpText() string {
	return fmt.Sprintf(
		`Turn Go benchmark results into a winner, tie, or trade-off.

Usage:
  verdict [command] [options]

Note:
  Raw benchmark comparisons need at least %d samples per benchmark side.
  For stable results, run benchmarks with -count=%d or more.

Commands:
  skill
      Print the AI Agent skill text.
  version
      Print the command version.

Options:
  --format text|json
      Output format. Default: text.
  -v, --version
      Print the command version.
  --mode auto|benchstat|alternatives
      Input mode. Default: auto.
      auto: detect benchstat output or raw go test -bench output.
      benchstat: read already-compared benchstat text or CSV.
      alternatives: compare raw sub-benchmarks, such as original vs enhanced.
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
      P-value threshold for statistical significance. Must be greater than 0 and at most 1. Default: 0.05.
  --min-delta value
      Minimum absolute delta percentage to treat as a practical difference. Must be non-negative. Default: 0.0.
 `,
		verdict.RawComparisonMinSamples,
		verdict.RecommendedRawSamples,
	)
}
