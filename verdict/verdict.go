// Package verdict parses benchstat and raw Go benchmark output into simple
// benchmark verdicts.
package verdict

import (
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser"
	"github.com/KEINOS/go-verdict/verdict/internal/benchparser/benchstat"
	"github.com/KEINOS/go-verdict/verdict/internal/benchparser/rawbench"
)

// Direction describes whether one metric improved, worsened, or stayed effectively the same.
type Direction string

const (
	// Improved means the new benchmark value is better than the old value.
	Improved Direction = "improved"
	// Worsened means the new benchmark value is worse than the old value.
	Worsened Direction = "worsened"
	// Same means the metric has no significant practical difference.
	Same Direction = "same"
)

// Outcome is the benchmark-level decision after all comparable metrics are evaluated.
type Outcome string

const (
	// NewWins means the new result has improvements and no regressions.
	NewWins Outcome = "new-wins"
	// OldWins means the new result has regressions and no improvements.
	OldWins Outcome = "old-wins"
	// Tie means no metric changed enough to matter.
	Tie Outcome = "tie"
	// TradeOff means at least one metric improved and at least one metric regressed.
	TradeOff Outcome = "trade-off"
	// Inconclusive means the input does not contain enough comparable data.
	Inconclusive Outcome = "inconclusive"
)

const (
	// ModeAuto detects benchstat or raw Go benchmark input.
	ModeAuto = "auto"
	// ModeBenchstat reads already compared benchstat text or CSV input.
	ModeBenchstat = "benchstat"
	// ModeGoTestBench reads raw Go benchmark rows from stdin.
	ModeGoTestBench = "gotestbench"
)

const (
	// ReasonMissingPValue identifies comparisons without a usable p-value.
	ReasonMissingPValue = "missing-pvalue"
	// ReasonBenchmarkSetMismatch identifies benchstat input with different benchmark sets.
	ReasonBenchmarkSetMismatch = "benchmark-set-mismatch"
	// ReasonMissingBaseline identifies raw input without the selected baseline label.
	ReasonMissingBaseline = "missing-baseline"
	// ReasonMissingCandidate identifies raw input without the selected candidate label.
	ReasonMissingCandidate = "missing-candidate"
	// ReasonInsufficientSamples identifies raw comparison input with too few repeated samples.
	ReasonInsufficientSamples = "insufficient-samples"
	// ReasonUnsupportedMetric identifies raw input without supported metrics to compare.
	ReasonUnsupportedMetric = "unsupported-metric"
	// ReasonMalformedBenchmark identifies raw input with malformed benchmark rows.
	ReasonMalformedBenchmark = "malformed-benchmark"
	// ReasonAmbiguousLabels identifies raw input where one baseline/candidate pair cannot be selected.
	ReasonAmbiguousLabels = "ambiguous-labels"
	// ReasonAmbiguousBenchmark identifies a raw benchmark file with more than one benchmark series.
	ReasonAmbiguousBenchmark = "ambiguous-benchmark"
)

// Options controls parser mode, labels, and the statistical and practical
// thresholds used by Parse and CompareRawFiles.
//
// Alpha defaults to 0.05 when left zero; non-zero Alpha values must be finite
// and greater than 0 and at most 1. MinDeltaPct defaults to DefaultMinDeltaPct
// when left zero; use NewOptions().WithMinDeltaPct(0) to count any statistically
// significant delta as practical. MinDeltaPct must be finite and non-negative.
//
// Mode accepts "", "auto", "benchstat", or "gotestbench". Empty mode is the
// same as "auto": Parse detects raw go test benchmark rows when possible and
// otherwise parses benchstat text or CSV input. "benchstat" disables raw input
// detection. "gotestbench" reads raw go test benchmark rows from stdin.
//
// Baseline and Candidate select raw go test -bench sub-benchmark labels. In
// "gotestbench" mode, empty labels default to "original" and "enhanced". In "auto" mode,
// empty labels allow Parse to prefer an original/enhanced pair when present or
// infer the only two labels under a parent benchmark. CompareRawFiles ignores
// Mode, Baseline, and Candidate because its labels come from the two files.
type Options struct {
	Baseline    string
	Candidate   string
	Mode        string
	Alpha       float64
	MinDeltaPct float64

	minDeltaPctSet bool
}

// Comparison is one parsed metric comparison for one benchmark.
type Comparison struct {
	BaselineLabel  string    `json:"-"`
	Benchmark      string    `json:"benchmark"`
	CandidateLabel string    `json:"-"`
	Metric         string    `json:"metric"`
	Direction      Direction `json:"direction"`
	DeltaPct       float64   `json:"delta_pct"`
	PValue         float64   `json:"p_value"`
	Significant    bool      `json:"significant"`
}

// BenchmarkVerdict is the final outcome for one benchmark name.
type BenchmarkVerdict struct {
	BaselineLabel  string       `json:"baseline_label,omitempty"`
	Benchmark      string       `json:"benchmark"`
	CandidateLabel string       `json:"candidate_label,omitempty"`
	Reason         string       `json:"reason"`
	ReasonCode     string       `json:"reason_code,omitempty"`
	Winner         string       `json:"winner,omitempty"`
	Outcome        Outcome      `json:"outcome"`
	Metrics        []Comparison `json:"metrics"`
}

// Report is the complete parse and evaluation result.
type Report struct {
	Verdicts []BenchmarkVerdict `json:"verdicts"`
}

const (
	// StatisticalMinSamples is the minimum needed for variance-based calculations.
	StatisticalMinSamples = 2
	// RawComparisonMinSamples is the minimum accepted sample count for raw benchmark comparisons.
	RawComparisonMinSamples = 3
	// RecommendedRawSamples is the recommended sample count for stable raw benchmark decisions.
	RecommendedRawSamples = 10
	// DefaultMinDeltaPct is the default practical-difference threshold.
	DefaultMinDeltaPct = 2.0
)

const (
	defaultAlpha           = 0.05
	defaultBaseline        = "original"
	defaultCandidate       = "enhanced"
	fallbackBaselineLabel  = "old"
	fallbackCandidateLabel = "new"
)

var (
	errReadingInput     = errors.New("reading benchstat input")
	errInvalidOptions   = errors.New("invalid options")
	errNoComparisonRows = benchstat.ErrNoComparisonRows
)

// Parse reads benchstat output or raw Go benchmark output and returns a
// benchmark verdict report.
func Parse(reader io.Reader, opts Options) (Report, error) {
	opts = normalizeOptions(opts)

	err := validateParseOptions(opts)
	if err != nil {
		return Report{}, err
	}

	input, err := io.ReadAll(reader)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %w", errReadingInput, err)
	}

	text := string(input)

	switch opts.Mode {
	case ModeGoTestBench:
		return parseGoTestBench(text, opts)
	case ModeBenchstat:
		return parseBenchstat(text, opts)
	default:
		if rawbench.LooksLikeGoTestBench(text) {
			return parseGoTestBench(text, opts)
		}

		return parseBenchstat(text, opts)
	}
}

// CompareRawFiles compares two raw go test benchmark result files as explicit A/B inputs.
func CompareRawFiles(aReader io.Reader, bReader io.Reader, opts Options) (Report, error) {
	opts = normalizeOptions(opts)

	err := validateOptions(opts)
	if err != nil {
		return Report{}, err
	}

	return compareRawFiles(aReader, bReader, opts)
}

func parseBenchstat(text string, opts Options) (Report, error) {
	result, err := benchstat.Parse(text)
	if err != nil {
		return Report{}, fmt.Errorf("parsing benchstat input: %w", err)
	}

	if result.InconclusiveReason != "" {
		if result.IncludeLabels {
			return labeledInconclusiveReport(
				benchstatReasonCode(result.InconclusiveReason),
				labelWithFallback(result.BaselineLabel, fallbackBaselineLabel),
				labelWithFallback(result.CandidateLabel, fallbackCandidateLabel),
			), nil
		}

		return inconclusiveReport(benchstatReasonCode(result.InconclusiveReason)), nil
	}

	return evaluate(benchstatComparisons(result.Comparisons, opts)), nil
}

func benchstatReasonCode(reason string) string {
	switch reason {
	case benchstat.ReasonBenchmarkSetMismatch:
		return ReasonBenchmarkSetMismatch
	case benchstat.ReasonMissingPValue:
		return ReasonMissingPValue
	default:
		return reason
	}
}

func labelWithFallback(label, fallback string) string {
	if label != "" {
		return label
	}

	return fallback
}

func benchstatComparisons(rows []benchparser.Comparison, opts Options) []Comparison {
	comparisons := make([]Comparison, 0, len(rows))

	for _, row := range rows {
		significant := row.PValue <= opts.Alpha &&
			math.Abs(row.DeltaPct) >= opts.MinDeltaPct &&
			!row.ApproxEqual

		comparisons = append(comparisons, Comparison{
			Benchmark:      row.Benchmark,
			Metric:         row.Metric,
			DeltaPct:       row.DeltaPct,
			PValue:         row.PValue,
			Significant:    significant,
			Direction:      classify(row.Metric, row.DeltaPct, significant),
			BaselineLabel:  displayLabelWithFallback(row.BaselineLabel, fallbackBaselineLabel),
			CandidateLabel: displayLabelWithFallback(row.CandidateLabel, fallbackCandidateLabel),
		})
	}

	return comparisons
}

// NewOptions returns default options for callers that prefer explicit setup.
// The zero value Options{} is also valid and uses the same defaults.
// Baseline and Candidate stay empty so auto mode can infer raw benchmark
// labels; gotestbench mode fills original/enhanced defaults when unset.
func NewOptions() Options {
	return Options{
		Alpha:          defaultAlpha,
		MinDeltaPct:    DefaultMinDeltaPct,
		Mode:           ModeAuto,
		Baseline:       "",
		Candidate:      "",
		minDeltaPctSet: true,
	}
}

// WithMinDeltaPct returns a copy of opts with an explicit practical-difference
// threshold. Use WithMinDeltaPct(0) when every statistically significant delta
// should count as practical, matching CLI use such as `verdict --min-delta 0`.
func (opts Options) WithMinDeltaPct(minDeltaPct float64) Options {
	opts.MinDeltaPct = minDeltaPct
	opts.minDeltaPctSet = true

	return opts
}

func normalizeOptions(opts Options) Options {
	if opts.Alpha == 0 {
		opts.Alpha = defaultAlpha
	}

	if opts.MinDeltaPct == 0 && !opts.minDeltaPctSet {
		opts.MinDeltaPct = DefaultMinDeltaPct
	}

	if opts.Mode == "" {
		opts.Mode = ModeAuto
	}

	if opts.Mode == ModeGoTestBench && opts.Baseline == "" {
		opts.Baseline = defaultBaseline
	}

	if opts.Mode == ModeGoTestBench && opts.Candidate == "" {
		opts.Candidate = defaultCandidate
	}

	return opts
}

func validateOptions(opts Options) error {
	switch {
	case math.IsNaN(opts.Alpha) || math.IsInf(opts.Alpha, 0) || opts.Alpha <= 0 || opts.Alpha > 1:
		return fmt.Errorf("%w: alpha must be greater than 0 and at most 1", errInvalidOptions)
	case math.IsNaN(opts.MinDeltaPct) || math.IsInf(opts.MinDeltaPct, 0) || opts.MinDeltaPct < 0:
		return fmt.Errorf("%w: min-delta must be finite and non-negative", errInvalidOptions)
	default:
		return nil
	}
}

func validateParseOptions(opts Options) error {
	err := validateOptions(opts)
	if err != nil {
		return err
	}

	switch opts.Mode {
	case ModeAuto, ModeBenchstat, ModeGoTestBench:
		return nil
	default:
		return fmt.Errorf("%w: unknown mode %q", errInvalidOptions, opts.Mode)
	}
}
