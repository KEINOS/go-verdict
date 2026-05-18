# go-verdict

`verdict` is a small command for Go benchmarks. It reads benchmark results and prints an A/B verdict.

```sh
# Compare benchmark results before and after a change:
# (`BenchmarkMyHeavyFunc()` + 10 iterations in this case)
benchstat old.txt new.txt | verdict
MyHeavyFunc-10: tie
```

```sh
# Compare two alternatives in raw benchmark output:
% go test -run='^$' -bench=BenchmarkMyHeavyFunc -benchmem -count=8 ./your/package | verdict
BenchmarkMyHeavyFunc: enhanced wins
```

It is useful when you want a clear answer after changing code:

- Did the new version become faster?
- Did it become slower?
- Is the result only noise?
- Is there a trade-off between metrics?
- Is another function better for the same job?

## Contents

- [Features](#features)
- [Workflows](#workflows)
  - [Local Alternative Comparison](#local-alternative-comparison)
  - [Named File A/B Comparison](#named-file-ab-comparison)
  - [PR or Before/After Comparison](#pr-or-beforeafter-comparison)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Output Formats](#output-formats)
- [CLI Options](#cli-options)
- [Verdicts](#verdicts)
- [Inconclusive Results](#inconclusive-results)
- [Library Usage](#library-usage)
- [AI Agent Skill](#ai-agent-skill)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Features

- Supports modern `benchstat` CSV-style output.
- Supports text `benchstat` output that includes `p=`.
- Auto-detects raw `go test -bench` output for local alternative comparison.
- Compares two raw benchmark files with `-a` and `-b`.
- Prints one concise verdict line by default, with details available via `--verbose`.
- Uses both statistical significance and a practical delta threshold.
- Handles lower-is-better metrics such as `sec/op`, `ns/op`, `B/op`, and `allocs/op`.
- Handles higher-is-better metrics such as `MB/s`, `ops/s`, and other `/s` rates.
- Returns stable outcomes for CI and scripts.

## Workflows

`verdict` is designed around three common workflows.

### Local Alternative Comparison

Use this workflow for a quick local test before a PR. Use it when the original and alternative implementations are sub-benchmarks in the same test file.

Example benchmark:

```go
func BenchmarkMyHeavyFunc(b *testing.B) {
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
go test -run='^$' -bench='BenchmarkMyHeavyFunc' -benchmem -count=20 ./your/package > alternatives.txt
```

Then compare the two sub-benchmarks:

```sh
verdict < alternatives.txt
```

Auto mode first checks whether both `original` and `enhanced` exist under the
same parent benchmark. If both exist, it compares that pair.

Otherwise, auto mode compares one pair only when the parent benchmark has
exactly two sub-benchmark labels.

Use explicit labels when the raw benchmark output has more than two alternatives or non-default names:

```sh
verdict --mode alternatives --baseline original --candidate enhanced < alternatives.txt
```

The outcome meaning is from the new option side:

| Outcome | Meaning in alternative mode |
| --- | --- |
| `new-wins` | The candidate is better than the baseline. |
| `old-wins` | The baseline is better than the candidate. |
| `tie` | There is no statistically significant practical difference. |
| `trade-off` | Some metrics improved, but other metrics regressed. |
| `inconclusive` | The input does not contain enough comparable samples. |

Raw benchmark comparison supports `ns/op`, `B/op`, and `allocs/op` from `go test` output. It computes deltas and approximate p-values from repeated samples, then applies the same `--alpha`, `--min-delta`, and verdict rules as before/after comparison. It needs at least 3 samples per benchmark side; with fewer samples, the library report is `inconclusive` and the CLI prints an actionable error.

Run with enough samples. `-count=8` is accepted for quick local checks, but `-count=10` or more is recommended for stable results. If you use only `-count=2`, `verdict` asks you to run more samples:

```text
error: insufficient samples: need at least 3 samples per benchmark side; recommend -count=10 or more for stable results
```

The raw-sample p-value is a pragmatic normal approximation from the two sample means and variances. Very small accepted sample counts, such as 3 to 5 per side, can be useful for quick feedback but are weaker evidence than the recommended `-count=10` or more. Benchmark distributions can be noisy or non-normal, so treat close results as guidance to collect more samples.

### Named File A/B Comparison

Use this workflow when two benchmark functions have different names, but they do the same job.

For example, this is not a valid before/after `benchstat` comparison:

```sh
benchstat fast.txt slow.txt | verdict
```

If `fast.txt` contains `BenchmarkMyHeavyFuncFast` and `slow.txt` contains `BenchmarkMyHeavyFuncSlow`, the benchmark names are different. `benchstat` compares the same benchmark name before and after a change, so `verdict` errors.

In that case, use `-a` and `-b` to say: "compare these two raw benchmark files as A/B alternatives."

```sh
verdict -a fast.txt -b slow.txt
```

Example output:

```text
BenchmarkMyHeavyFuncFast_vs_BenchmarkMyHeavyFuncSlow: BenchmarkMyHeavyFuncFast wins
```

Each file should contain one benchmark series, collected with repeated samples:

```sh
go test -run='^$' -bench=BenchmarkMyHeavyFuncFast -count=10 ./your/package > fast.txt
go test -run='^$' -bench=BenchmarkMyHeavyFuncSlow -count=10 ./your/package > slow.txt
verdict -a fast.txt -b slow.txt
```

### PR or Before/After Comparison

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

## Requirements

- Go 1.26.3 or later, as declared in `go.mod`.
- `benchstat` for PR or before/after comparison.

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

Use before/after comparison when you already have separate old and new benchmark files:

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
Usage: verdict [command] [options]

Raw benchmark comparisons need at least 3 samples per benchmark side.
For stable results, run benchmarks with -count=10 or more.

Commands:
  skill
      Print the AI Agent skill text.

Options:
  --format text|json
      Output format. Default: text.

  --mode auto|benchstat|alternatives
      Input mode. Default: auto.

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
```

Example with a stricter practical threshold:

```sh
benchstat old.txt new.txt | verdict --alpha 0.05 --min-delta 2.0
```

With this command, a metric must have `p <= 0.05` and at least `2.0%` change to count as improved or worsened.

For raw benchmark input, p-values are approximate and become more trustworthy as sample counts rise.

## Verdicts

`go-verdict` uses Pareto-style rules for each benchmark. In this README,
Pareto-superior means better in one or more metrics and not worse in any metric.

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

Some inputs cannot produce a reliable verdict. Library reports use
`inconclusive` for these cases. The CLI also turns selected cases, such as
benchmark-set mismatch and insufficient raw samples, into actionable errors.

Known reason codes include:

| Reason code | Meaning |
| --- | --- |
| `missing-pvalue` | The comparison does not include a p-value. |
| `benchmark-set-mismatch` | The old and new benchmark sets are different. |
| `missing-baseline` | Raw alternatives input did not find the baseline sub-benchmark. |
| `missing-candidate` | Raw alternatives input did not find the candidate sub-benchmark. |
| `insufficient-samples` | Raw comparison found too few repeated samples. |
| `unsupported-metric` | Raw comparison did not find supported metrics to compare. |
| `malformed-benchmark` | Raw comparison could not parse the benchmark rows. |
| `ambiguous-alternatives` | Raw alternatives input could not select one baseline/candidate pair from labels. |
| `ambiguous-benchmark` | A raw file contains more than one benchmark series. |

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

`verdict.Options{}` uses the default alpha of `0.05`. `verdict.NewOptions()` returns the same safe defaults for callers that prefer explicit setup. If you set `Alpha`, use a finite value greater than `0` and at most `1`; `MinDeltaPct` must be finite and non-negative.

Parse raw alternatives from stdin by selecting explicit sub-benchmark labels:

```go
report, err := verdict.Parse(os.Stdin, verdict.Options{
 Mode:      "alternatives",
 Baseline:  "original",
 Candidate: "enhanced",
})
if err != nil {
 panic(err)
}
_ = report.WriteText(os.Stdout)
```

Compare two raw benchmark files as A/B alternatives:

```go
a, err := os.Open("fast.txt")
if err != nil {
 panic(err)
}
defer a.Close()

b, err := os.Open("slow.txt")
if err != nil {
 panic(err)
}
defer b.Close()

report, err := verdict.CompareRawFiles(a, b, verdict.NewOptions())
if err != nil {
 panic(err)
}
_ = report.WriteJSON(os.Stdout)
```

Use report outcomes directly in CI checks:

```go
report, err := verdict.Parse(os.Stdin, verdict.NewOptions())
if err != nil {
 panic(err)
}

for _, item := range report.Verdicts {
 if item.Outcome == verdict.OldWins || item.Outcome == verdict.TradeOff {
  os.Exit(1)
 }
}
```

## AI Agent Skill

To give an AI agent guidance for using `verdict`:

```sh
verdict skill > SKILL.md
```

The canonical source is [skill/verdict/SKILL.md](skill/verdict/SKILL.md).

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

See available development targets:

```sh
make help
```

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) for the safe development workflow,
architecture overview, fixture rules, and extension checklist.

## Project Layout

```text
cmd/verdict/       CLI entry point
verdict/           Parser, evaluator, and output writer
testdata/          Demo benchmarks and generated fixture files
Makefile           Fixture and end-to-end commands
```

## License

MIT License. See [LICENSE](LICENSE).
