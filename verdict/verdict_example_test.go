package verdict_test

import (
	"os"
	"strings"

	"github.com/KEINOS/go-verdict/verdict"
)

func ExampleParse() {
	input := `name old time/op new time/op delta
Example-10 10.0ns 8.0ns -20.00% (p=0.001 n=10)
`

	report, err := verdict.Parse(strings.NewReader(input), verdict.Options{})
	if err != nil {
		panic(err)
	}

	if err := report.WriteText(os.Stdout); err != nil {
		panic(err)
	}

	// Output:
	// Example-10: new wins
}

func ExampleParse_rawAlternatives() {
	input := strings.Repeat("BenchmarkEncode/original-10 100 10 ns/op\n", verdict.RawComparisonMinSamples) +
		strings.Repeat("BenchmarkEncode/enhanced-10 100 5 ns/op\n", verdict.RawComparisonMinSamples)

	report, err := verdict.Parse(strings.NewReader(input), verdict.Options{
		Mode:      "alternatives",
		Baseline:  "original",
		Candidate: "enhanced",
	})
	if err != nil {
		panic(err)
	}

	if err := report.WriteText(os.Stdout); err != nil {
		panic(err)
	}

	// Output:
	// BenchmarkEncode: enhanced wins
}

func ExampleCompareRawFiles() {
	fast := strings.NewReader(
		strings.Repeat("BenchmarkFast-10 100 5 ns/op\n", verdict.RawComparisonMinSamples),
	)
	slow := strings.NewReader(
		strings.Repeat("BenchmarkSlow-10 100 10 ns/op\n", verdict.RawComparisonMinSamples),
	)

	report, err := verdict.CompareRawFiles(fast, slow, verdict.Options{})
	if err != nil {
		panic(err)
	}

	if err := report.WriteText(os.Stdout); err != nil {
		panic(err)
	}

	// Output:
	// BenchmarkFast_vs_BenchmarkSlow: BenchmarkFast wins
}

func ExampleReport_WriteJSON() {
	input := `name old time/op new time/op delta
Example-10 10.0ns 8.0ns -20.00% (p=0.001 n=10)
`

	report, err := verdict.Parse(strings.NewReader(input), verdict.Options{})
	if err != nil {
		panic(err)
	}

	if err := report.WriteJSON(os.Stdout); err != nil {
		panic(err)
	}

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
