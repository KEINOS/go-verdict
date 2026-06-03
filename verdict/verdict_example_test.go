package verdict_test

import (
	"os"
	"strings"

	"github.com/KEINOS/go-verdict/verdict"
)

func ExampleParse() {
	// benchstat format example
	input := `name old time/op new time/op delta
Example-10 10.0ns 8.0ns -20.00% (p=0.001 n=10)
`

	reporter, err := verdict.Parse(strings.NewReader(input), verdict.NewOptions())
	if err != nil {
		panic(err)
	}

	err = reporter.WriteText(os.Stdout)
	if err != nil {
		panic(err)
	}
	//
	// Output:
	// Example-10: new wins
}

func ExampleParse_rawGoTestBench() {
	// raw go test -bench sub-benchmarks format example (explicit A/B input without statistical test results)
	input := strings.Repeat(
		"BenchmarkEncode/original-10 100 10 ns/op\n", verdict.RawComparisonMinSamples,
	) + strings.Repeat(
		"BenchmarkEncode/enhanced-10 100 5 ns/op\n", verdict.RawComparisonMinSamples,
	)

	opts := verdict.NewOptions()
	opts.Mode = verdict.ModeGoTestBench
	opts.Baseline = "original"
	opts.Candidate = "enhanced"

	reporter, err := verdict.Parse(strings.NewReader(input), opts)
	if err != nil {
		panic(err)
	}

	err = reporter.WriteText(os.Stdout)
	if err != nil {
		panic(err)
	}
	//
	// Output:
	// BenchmarkEncode: enhanced wins
}

func ExampleCompareRawFiles() {
	fast := strings.NewReader(
		// raw go test -bench sub-benchmarks format example
		strings.Repeat("BenchmarkFast-10 100 5 ns/op\n", verdict.RawComparisonMinSamples),
	)
	slow := strings.NewReader(
		// raw go test -bench sub-benchmarks format example
		strings.Repeat("BenchmarkSlow-10 100 10 ns/op\n", verdict.RawComparisonMinSamples),
	)

	reporter, err := verdict.CompareRawFiles(fast, slow, verdict.NewOptions())
	if err != nil {
		panic(err)
	}

	err = reporter.WriteText(os.Stdout)
	if err != nil {
		panic(err)
	}
	//
	// Output:
	// BenchmarkFast_vs_BenchmarkSlow: BenchmarkFast wins
}

func ExampleReport_WriteJSON() {
	// benchstat format example
	input := `name old time/op new time/op delta
Example-10 10.0ns 8.0ns -20.00% (p=0.001 n=10)
`

	reporter, err := verdict.Parse(strings.NewReader(input), verdict.NewOptions())
	if err != nil {
		panic(err)
	}

	err = reporter.WriteJSON(os.Stdout)
	if err != nil {
		panic(err)
	}
	//
	// Output:
	// {
	//   "verdicts": [
	//     {
	//       "benchmark": "Example-10",
	//       "outcome": "new-wins",
	//       "winner": "new",
	//       "baseline_label": "old",
	//       "candidate_label": "new",
	//       "metrics": [
	//         {
	//           "benchmark": "Example-10",
	//           "metric": "sec/op",
	//           "delta_pct": -20,
	//           "p_value": 0.001,
	//           "significant": true,
	//           "direction": "improved"
	//         }
	//       ],
	//       "reason": "new is Pareto-superior: better in one or more metrics and not worse in any metric"
	//     }
	//   ]
	// }
}
