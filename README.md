# go-verdict

`verdict` turns Go benchmark results into a winner, tie, or trade-off.

Compare benchmark results before and after a change:

```shellsession
% benchstat old.txt new.txt | verdict
MyHeavyFunc-10: tie
```

Compare two alternatives in raw benchmark output:

```shellsession
% go test -run='^$' -bench=BenchmarkMyHeavyFunc -benchmem -count=8 ./your/package | verdict
BenchmarkMyHeavyFunc: enhanced wins
```

Use it when you need an objective keep-or-discard decision after trying to make Go code faster, more memory efficient, or less allocation-heavy. `verdict` keeps the decision focused on measured benchmark results.

The same output can guide local development, CI checks, scripts, and automated optimization loops.

It is useful when you want a clear answer after changing code:

- Did the new version become faster?
- Did it become slower?
- Is the result only noise?
- Is there a trade-off between metrics?
- Is another function better for the same job?

## Contents

- [Features](#features)
- [Workflows](#workflows)
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
- Flags mixed results as `trade-off` when one metric improves while another regresses.
- Returns stable outcomes for CI and scripts.

## Workflows

`verdict` is designed around three common comparison workflows: Alternative, Named File, and Before/After.

Each workflow produces a concise benchmark decision that can be used by humans, scripts, CI, or automated optimization.

Alternative compares sub-benchmarks in one raw `go test -bench` stream:

```sh
go test -run='^$' -bench='BenchmarkMyHeavyFunc' -benchmem -count=20 ./your/package | verdict
```

Named File compares two raw benchmark files that do the same job but use different benchmark names:

```sh
verdict -a fast.txt -b slow.txt
```

Before/After compares old and new benchmark results through `benchstat`:

```sh
benchstat old.txt new.txt | verdict
```

See [Workflow Details](README_WORKFLOWS.md) for examples, auto-detection rules, raw-sample requirements, and mismatch handling.

## Requirements

- Go 1.26.3 or later, as declared in `go.mod`.
- Recommended:
  - [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) for before/after comparison.

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
make build
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

```shellsession
% verdict --help
Turn Go benchmark results into a winner, tie, or trade-off.

Usage:
  verdict [command] [options]

Note:
  Raw benchmark comparisons need at least 3 samples per benchmark side.
  For stable results, run benchmarks with -count=10 or more.

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
```

Example with a stricter practical threshold:

```sh
benchstat old.txt new.txt | verdict --alpha 0.05 --min-delta 2.0
```

With this command, a metric must have `p <= 0.05` and at least `2.0%` change to count as improved or worsened.

For raw benchmark input, p-values are approximate and become more trustworthy as sample counts rise.

## Verdicts

`verdict` uses Pareto-style rules for each benchmark. In this README, Pareto-superior means better in one or more metrics and not worse in any metric.

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

Some inputs cannot produce a reliable verdict. Library reports use `inconclusive` for these cases. The CLI also turns selected cases, such as benchmark-set mismatch and insufficient raw samples, into actionable errors.

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

`verdict skill` prints guidance for agents that use `verdict` as an objective benchmark gate in Go optimization loops. This is optional; the core command is the same developer-facing benchmark decision tool.

To export the guidance:

```sh
verdict skill > SKILL.md
```

The canonical source is
[cmd/verdict/internal/skill/SKILL.md](cmd/verdict/internal/skill/SKILL.md).

## Development

Run tests:

```sh
go test ./...
```

Generate benchmark fixtures and run the end-to-end check:

```sh
make e2e
```

`make build` removes `./dist` before writing `./dist/verdict`, so repeated
E2E runs are safe even when a stale local binary already exists. The `Makefile`
also has cleanup and fixture targets:

```sh
make clean-dist      # remove ./dist
make clean-testdata  # remove generated testdata/*.txt files
make clean-all       # remove both generated fixture files and ./dist
make data            # regenerate benchmark fixtures in testdata/
```

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
