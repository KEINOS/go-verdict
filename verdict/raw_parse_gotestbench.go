package verdict

import (
	"fmt"

	"github.com/KEINOS/go-verdict/verdict/internal/benchparser/rawbench"
)

func parseGoTestBench(input string, opts Options) (Report, error) {
	state, err := rawbench.ParseGoTestBench(input)
	if err != nil {
		return Report{}, fmt.Errorf("parsing raw go test -bench input: %w", err)
	}

	if !state.HasBenchmarkRows {
		return inconclusiveReport("malformed-benchmark"), nil
	}

	// Parsing only records raw samples and input-shape flags. Evaluation below
	// selects labels, checks sample counts, and applies shared verdict rules.
	report := evaluateGoTestBench(state, opts)
	if len(report.Verdicts) == 0 {
		return emptyGoTestBenchReport(state), nil
	}

	return report, nil
}
