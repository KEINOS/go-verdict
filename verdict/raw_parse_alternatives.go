package verdict

import (
	"fmt"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser/rawbench"
)

func parseAlternatives(input string, opts Options) (Report, error) {
	state, err := rawbench.ParseAlternatives(input)
	if err != nil {
		return Report{}, fmt.Errorf("parsing raw alternatives input: %w", err)
	}

	if !state.HasBenchmarkRows {
		return inconclusiveReport("malformed-benchmark"), nil
	}

	// Parsing only records raw samples and input-shape flags. Evaluation below
	// selects labels, checks sample counts, and applies shared verdict rules.
	report := evaluateAlternatives(state, opts)
	if len(report.Verdicts) == 0 {
		return emptyAlternativeReport(state), nil
	}

	return report, nil
}
