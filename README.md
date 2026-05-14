# go-verdict

`verdict` command is a simple comparison tool for Go benchmarks. It reads the benchmark results from standard input and gives a simple verdict.

```sh
benchstat old.txt new.txt | verdict
```

```sh
% go test -run='^$' -bench=BenchmarkEnhance -benchmem -count=8 ./testdata | verdict
BenchmarkEnhance: enhanced wins
```

It is useful when you want a clear answer after changing code:

- Did the new version become faster?
- Did it become slower?
- Is the result only noise?
- Is there a trade-off between metrics?
- Is an alternative function better than the original function before making a PR?

## Features

- Supports modern `benchstat` CSV-style output.
- Supports text `benchstat` output that includes `p=`.
- Auto-detects raw `go test -bench` output for local alternative comparison.
- Prints one concise verdict line by default, with details available via `--verbose`.
- Uses both statistical significance and a practical delta threshold.
- Handles lower-is-better metrics such as `sec/op`, `ns/op`, `B/op`, and `allocs/op`.
- Handles higher-is-better metrics such as `MB/s`, `ops/s`, and other `/s` rates.
- Returns stable outcomes for CI and scripts.

## Workflows

`verdict` is designed around two comparison workflows.

### PR Comparison

Use this workflow when you compare benchmark results from two different code states, such as before and after a pull request.

Collect old benchmark results:

```sh
go test -run='^$' -bench=. -benchmem -count=20 > old.txt
```

Change your code, then collect new benchmark results:

```sh
go test -run='^$' -bench=. -benchmem -count=20 > new.txt
```

Compare them with `benchstat`, then pipe the result to `verdict`:

```sh
benchstat old.txt new.txt | verdict
```

If one side wins, `verdict` uses the file labels from the `benchstat` header:

```text
ExampleFast-10: new.txt wins
```

### Local Alternative Comparison

Use this workflow for quick PoC work before a PR. Use it when the original and alternative implementations can be benchmarked in the same test file.

Example benchmark:

```go
func BenchmarkEnhance(b *testing.B) {
 b.Run("original", func(b *testing.B) {
  for b.Loop() {
   ExampleOriginal()
  }
 })

 b.Run("enhanced", func(b *testing.B) {
  for b.Loop() {
   ExampleEnhanced()
  }
 })
}
```

Collect repeated benchmark samples:

```sh
go test -run='^$' -bench='BenchmarkEnhance' -benchmem -count=20 > alternatives.txt
```

Then compare the two sub-benchmarks:

```sh
verdict < alternatives.txt
```

Auto mode groups exactly two sub-benchmarks by their parent benchmark. For example, `BenchmarkEnhance/original-10` and `BenchmarkEnhance/enhanced-10` are compared as one pair under `BenchmarkEnhance`.

Use explicit labels when the raw benchmark output has more than two alternatives or non-default names:

```sh
verdict --mode alternatives --baseline original --candidate enhanced < alternatives.txt
```

The outcome meaning is from the candidate point of view:

| Outcome | Meaning in alternative mode |
| --- | --- |
| `new-wins` | The candidate is better than the baseline. |
| `old-wins` | The baseline is better than the candidate. |
| `tie` | There is no statistically significant practical difference. |
| `trade-off` | Some metrics improved, but other metrics regressed. |
| `inconclusive` | The input does not contain enough comparable samples. |

Raw benchmark comparison supports `ns/op`, `B/op`, and `allocs/op` from `go test` output. It computes deltas and p-values from repeated samples, then applies the same `--alpha`, `--min-delta`, and verdict rules as PR comparison. It requires repeated samples from `-count=N`; if there are not enough samples to compare, it returns `inconclusive`.

## Requirements

- Go 1.26.3 or later, as declared in `go.mod`.
- `benchstat` for PR comparison.

Install `benchstat` if you do not already have it:

```sh
go install golang.org/x/perf/cmd/benchstat@latest
```

## Installation

Install the `verdict` command with:

```sh
go install github.com/KEINOS/go-verdict/cmd/verdict@latest
```

Or build it from a local clone:

```sh
git clone https://github.com/KEINOS/go-verdict.git
cd go-verdict
go build -o ./dist/verdict ./cmd/verdict
```

## Quick Start

Use PR comparison when you already have separate old and new benchmark files:

```sh
benchstat old.txt new.txt | verdict
```

Example output:

```text
ExampleFast-10: tie
```

## Output Formats

Text output is the default:

```sh
benchstat old.txt new.txt | verdict --format text
```

Verbose text output includes the reason and metric-level details:

```sh
benchstat old.txt new.txt | verdict --verbose
```

JSON output is useful for CI, tools, and scripts:

```sh
benchstat old.txt new.txt | verdict --format json
```

Example JSON:

```json
{
  "verdicts": [
    {
      "benchmark": "ExampleFast-10",
      "outcome": "tie",
      "baseline_label": "old.txt",
      "candidate_label": "new.txt",
      "metrics": [
        {
          "benchmark": "ExampleFast-10",
          "metric": "sec/op",
          "delta_pct": 0,
          "p_value": 0.68,
          "significant": false,
          "direction": "same"
        }
      ],
      "reason": "no statistically significant practical difference"
    }
  ]
}
```

## CLI Options

```text
--format text|json
    Output format. Default: text.

--mode auto|benchstat|alternatives
    Input mode. Default: auto.

--verbose
    Include verdict reason and metric details in text output.

--baseline name
    Baseline sub-benchmark name for alternatives mode.

--candidate name
    Candidate sub-benchmark name for alternatives mode.

--alpha value
    P-value threshold for statistical significance. Default: 0.05.

--min-delta value
    Minimum absolute delta percentage to treat as a practical difference. Default: 0.0.
```

Example with a stricter practical threshold:

```sh
benchstat old.txt new.txt | verdict --alpha 0.05 --min-delta 2.0
```

With this command, a metric must have `p <= 0.05` and at least `2.0%` change to count as improved or worsened.

## Verdicts

`go-verdict` uses Pareto-style rules for each benchmark.

| Outcome | Meaning |
| --- | --- |
| `new-wins` | The new result has at least one significant improvement and no significant regression. |
| `old-wins` | The new result has at least one significant regression and no significant improvement. |
| `tie` | There is no statistically significant practical difference. |
| `trade-off` | Some metrics improved, but other metrics regressed. |
| `inconclusive` | The input does not contain enough comparable benchmark data. |

Each metric is classified as:

| Direction | Meaning |
| --- | --- |
| `improved` | The new value is better. |
| `worsened` | The new value is worse. |
| `same` | The change is not significant or not large enough. |

## Inconclusive Results

Some inputs cannot produce a reliable verdict. In that case, the output is `inconclusive`.

Known reason codes include:

| Reason code | Meaning |
| --- | --- |
| `missing-pvalue` | The comparison does not include a p-value. |
| `benchmark-set-mismatch` | The old and new benchmark sets are different. |
| `missing-baseline` | Alternative mode did not find the baseline sub-benchmark. |
| `missing-candidate` | Alternative mode did not find the candidate sub-benchmark. |
| `insufficient-samples` | Alternative mode found too few repeated samples. |
| `unsupported-metric` | Alternative mode did not find supported metrics to compare. |
| `malformed-benchmark` | Alternative mode could not parse the raw benchmark rows. |

## Library Usage

You can also use the parser and evaluator from Go code:

```go
package main

import (
 "os"

 "github.com/KEINOS/go-verdict/verdict"
)

func main() {
 report, err := verdict.Parse(os.Stdin, verdict.Options{
  Alpha:       0.05,
  MinDeltaPct: 0,
 })
 if err != nil {
  panic(err)
 }

 if err := report.WriteJSON(os.Stdout); err != nil {
  panic(err)
 }
}
```

## Development

Run tests:

```sh
go test ./...
```

Generate benchmark fixtures and run the end-to-end check:

```sh
make e2e
```

The `Makefile` also has a `data` target that regenerates files in `testdata/`.

## Project Layout

```text
cmd/verdict/       CLI entry point
verdict/           Parser, evaluator, and output writer
testdata/          Demo benchmarks and generated fixture files
Makefile           Fixture and end-to-end commands
```

## License

No license file is currently included in this repository.
