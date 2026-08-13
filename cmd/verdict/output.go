package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/KEINOS/go-verdict/cmd/verdict/internal/helptopic"
	"github.com/KEINOS/go-verdict/verdict"
)

type complexityJSONReport struct {
	Verdicts []complexityJSONVerdict `json:"verdicts"`
}

type complexityJSONVerdict struct {
	BaselineLabel  string               `json:"baseline_label,omitempty"`
	Benchmark      string               `json:"benchmark"`
	CandidateLabel string               `json:"candidate_label,omitempty"`
	Reason         string               `json:"reason"`
	ReasonCode     string               `json:"reason_code,omitempty"`
	Winner         string               `json:"winner,omitempty"`
	Outcome        verdict.Outcome      `json:"outcome"`
	Metrics        []verdict.Comparison `json:"metrics"`
	Complexity     complexityDetail     `json:"complexity"`
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

func writeComplexityReport(report complexityReport, cliOpts cliOptions, output io.Writer) error {
	switch cliOpts.outputFormat {
	case formatDefault:
		if !cliOpts.verbose {
			return wrapReportWriteError(report.Report.WriteText(output))
		}

		return writeComplexityVerboseText(report, output)
	case formatJSON:
		return writeComplexityJSON(report, output)
	default:
		return errUnknownFormat
	}
}

func writeComplexityVerboseText(report complexityReport, output io.Writer) error {
	for _, item := range report.Report.Verdicts {
		err := verdict.Report{Verdicts: []verdict.BenchmarkVerdict{item}}.WriteVerboseText(output)
		if err != nil {
			return fmt.Errorf("%w: %w", errWritingOutput, err)
		}

		detail := report.Details[item.Benchmark]
		if detail.Status == complexityStatusNotMapped {
			_, err = fmt.Fprintln(output, "  complexity: not-mapped")
			if err != nil {
				return fmt.Errorf("%w: %w", errWritingOutput, err)
			}

			continue
		}

		_, err = fmt.Fprintf(output, "  complexity %s\n", detail.Direction)
		if err != nil {
			return fmt.Errorf("%w: %w", errWritingOutput, err)
		}

		err = writeComplexityMeasurement(output, "baseline", detail.Baseline)
		if err != nil {
			return err
		}

		err = writeComplexityMeasurement(output, "candidate", detail.Candidate)
		if err != nil {
			return err
		}
	}

	return nil
}

func writeComplexityMeasurement(
	output io.Writer,
	side string,
	measurement *complexityMeasurement,
) error {
	if measurement == nil {
		return nil
	}

	source := measurement.Kind
	if measurement.Ref != "" {
		source += " ref=" + measurement.Ref
	}

	if measurement.Root != "" {
		source += " root=" + measurement.Root
	}

	_, err := fmt.Fprintf(
		output,
		"    %s %s file=%s symbol=%s cyclomatic=%d cognitive=%d score=%.6g\n",
		side,
		source,
		measurement.File,
		measurement.Symbol,
		measurement.Cyclomatic,
		measurement.Cognitive,
		measurement.Score,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingOutput, err)
	}

	return nil
}

func writeComplexityJSON(report complexityReport, output io.Writer) error {
	jsonReport := complexityJSONReport{
		Verdicts: make([]complexityJSONVerdict, 0, len(report.Report.Verdicts)),
	}

	for _, item := range report.Report.Verdicts {
		jsonReport.Verdicts = append(jsonReport.Verdicts, complexityJSONVerdict{
			BaselineLabel:  item.BaselineLabel,
			Benchmark:      item.Benchmark,
			CandidateLabel: item.CandidateLabel,
			Reason:         item.Reason,
			ReasonCode:     item.ReasonCode,
			Winner:         item.Winner,
			Outcome:        item.Outcome,
			Metrics:        item.Metrics,
			Complexity:     report.Details[item.Benchmark],
		})
	}

	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(jsonReport)
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingOutput, err)
	}

	return nil
}

func wrapReportWriteError(err error) error {
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingOutput, err)
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

const flagHelpTemplate = `%s

Usage:
  verdict [command] [options]

Note:
  Raw benchmark comparisons need at least %d samples per benchmark side.
  For stable results, run benchmarks with -count=%d or more.

Commands:
  help [topic]
      Print workflow help. Topics: %s.
  hotspot <package>
      Suggest the first function to inspect from benchmark profiles and source complexity.
  skill
      Print the AI Agent skill text.
  version
      Print the command version.

Options:
  --format text|json
      Output format. Default: text.
  -v, --version
      Print the command version.
  --mode auto|benchstat|gotestbench
      Input mode. Default: auto.
      auto: detect benchstat output or raw go test -bench output.
      benchstat: read already-compared benchstat text or CSV.
      gotestbench: compare raw go test -bench sub-benchmarks, such as original vs enhanced.
  --verbose
      Include verdict reason and metric details in text output.
  --require outcomes
      Require comma-separated outcomes for exit 0, such as new-wins or new-wins,tie.
  -a file
      Raw benchmark file for side A.
  -b file
      Raw benchmark file for side B.
  --baseline name
      Baseline sub-benchmark name for gotestbench mode.
  --candidate name
      Candidate sub-benchmark name for gotestbench mode.
  --alpha value
      P-value threshold for statistical significance. Must be greater than 0 and at most 1. Default: 0.05.
  --min-delta value
      Minimum absolute delta percentage to treat as a practical difference. Must be non-negative. Default: %.1f.
  --complexity json
      Add one explicit benchmark-to-source complexity mapping. Repeatable.
  --complexity-config file
      Read versioned benchmark-to-source complexity mappings from one JSON file.

Hotspot options:
  --bench regexp
      Benchmark regexp for verdict hotspot. Default: .
  --benchtime duration|Nx
      Benchmark time or iteration count for verdict hotspot. Default: 1s.
  --count n
      Benchmark run count for verdict hotspot. Default: 1.
  --top n
      Number of candidates to report for verdict hotspot. Default: 3.
  --fast
      Profile CPU and memory in one benchmark pass instead of two.
      Halves the run time and lowers CPU accuracy.
  --format text|json
      Output format for verdict hotspot. Default: text.
`

func flagHelpText() string {
	return fmt.Sprintf(
		flagHelpTemplate,
		AppDescription,
		verdict.RawComparisonMinSamples,
		verdict.RecommendedRawSamples,
		strings.Join(helptopic.Topics(), ", "),
		verdict.DefaultMinDeltaPct,
	)
}
