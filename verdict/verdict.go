// Package verdict parses benchstat and raw Go benchmark output into simple
// benchmark verdicts.
package verdict

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
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

// Options controls the statistical and practical thresholds used by Parse.
// Alpha defaults to 0.05 when left zero; non-zero Alpha values must be finite
// and greater than 0 and at most 1. MinDeltaPct must be finite and non-negative.
type Options struct {
	Alpha       float64
	MinDeltaPct float64
	Mode        string
	Baseline    string
	Candidate   string
}

// Comparison is one parsed metric comparison for one benchmark.
//
//nolint:tagliatelle // JSON fields keep benchstat snake_case names.
type Comparison struct {
	Benchmark      string    `json:"benchmark"`
	Metric         string    `json:"metric"`
	DeltaPct       float64   `json:"delta_pct"`
	PValue         float64   `json:"p_value"`
	Significant    bool      `json:"significant"`
	Direction      Direction `json:"direction"`
	BaselineLabel  string    `json:"-"`
	CandidateLabel string    `json:"-"`
}

// BenchmarkVerdict is the final outcome for one benchmark name.
//
//nolint:tagliatelle // JSON fields keep benchstat snake_case names.
type BenchmarkVerdict struct {
	Benchmark      string       `json:"benchmark"`
	Outcome        Outcome      `json:"outcome"`
	Winner         string       `json:"winner,omitempty"`
	BaselineLabel  string       `json:"baseline_label,omitempty"`
	CandidateLabel string       `json:"candidate_label,omitempty"`
	Metrics        []Comparison `json:"metrics"`
	Reason         string       `json:"reason"`
	ReasonCode     string       `json:"reason_code,omitempty"`
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
)

const (
	defaultAlpha           = 0.05
	defaultBaseline        = "original"
	defaultCandidate       = "enhanced"
	fallbackBaselineLabel  = "old"
	fallbackCandidateLabel = "new"
	modeAuto               = "auto"
	modeAlternatives       = "alternatives"
	modeBenchstat          = "benchstat"
)

var (
	errReadingInput     = errors.New("reading benchstat input")
	errInvalidOptions   = errors.New("invalid options")
	errNoComparisonRows = errors.New("no benchstat comparison rows found")
)

// Parse reads benchstat output or raw Go benchmark output and returns a
// benchmark verdict report.
func Parse(reader io.Reader, opts Options) (Report, error) {
	opts = normalizeOptions(opts)

	err := validateOptions(opts)
	if err != nil {
		return Report{}, err
	}

	input, err := io.ReadAll(reader)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %w", errReadingInput, err)
	}

	text := string(input)

	switch opts.Mode {
	case modeAlternatives:
		return parseAlternatives(text, opts)
	case modeBenchstat:
		return parseBenchstat(text, opts)
	default:
		if looksLikeRawBenchmarkInput(text) {
			return parseAlternatives(text, opts)
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
	if strings.Contains(text, ",vs base,P") {
		return parseCSV(text, opts)
	}

	return parseText(text, opts)
}

// NewOptions returns default options for callers that prefer explicit setup.
// The zero value Options{} is also valid and uses the same defaults.
// Baseline and Candidate stay empty so auto mode can infer raw benchmark
// labels; alternatives mode fills original/enhanced defaults when unset.
func NewOptions() Options {
	return Options{
		Alpha:       defaultAlpha,
		MinDeltaPct: 0,
		Mode:        modeAuto,
		Baseline:    "",
		Candidate:   "",
	}
}

func normalizeOptions(opts Options) Options {
	if opts.Alpha == 0 {
		opts.Alpha = defaultAlpha
	}

	if opts.Mode == "" {
		opts.Mode = modeAuto
	}

	if opts.Mode == modeAlternatives && opts.Baseline == "" {
		opts.Baseline = defaultBaseline
	}

	if opts.Mode == modeAlternatives && opts.Candidate == "" {
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
