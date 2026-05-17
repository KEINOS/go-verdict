---
name: verdict
description: Use when an AI agent needs to compare Go benchmark results with the verdict CLI, summarize benchstat output, judge local benchmark alternatives, or compare raw Go benchmark files as A/B inputs.
---

# Verdict

Use `verdict` to turn Go benchmark results into a concise A/B decision.

## When to Use

- Compare Go benchmark results before and after a code change.
- Judge local alternatives implemented as sub-benchmarks.
- Summarize `benchstat` output for a human or automation.
- Compare two raw Go benchmark files as A/B alternatives.

## Do Not Use

- Do not use this for non-Go benchmark data.
- Do not declare a winner when the input has too few samples.
- Do not reimplement the statistical decision model when the `verdict` CLI can read the data.

## Choose a Workflow

For local alternatives in raw benchmark output:

```sh
go test -run='^$' -bench=BenchmarkMyHeavyFunc -benchmem -count=10 ./your/package | verdict
```

For differently named benchmark functions in raw files:

```sh
verdict -a fast.txt -b slow.txt
```

For before/after benchmark results:

```sh
benchstat old.txt new.txt | verdict
```

## Recommended Procedure

1. Use at least 3 samples per side for raw benchmark comparisons.
2. Prefer `-count=10` or more for stable raw benchmark decisions.
3. Use default text output for short human-facing summaries.
4. Use `--verbose` when you need the reason and metric details.
5. Use `--format json` when another tool will consume the result.
6. Use `-a` and `-b` when benchmark function names differ.

## Raw Alternatives

- Auto mode compares `original` and `enhanced` when both labels exist under the same parent benchmark.
- If that default pair is absent, auto mode infers a pair only when the parent has exactly two labels.
- Use `--mode alternatives --baseline <name> --candidate <name>` for non-default labels or ambiguous inputs.

## Interpret Results

- `new-wins`: the candidate/new side is better overall.
- `old-wins`: the baseline/old side is better overall.
- `tie`: no statistically significant practical difference was found.
- `trade-off`: some metrics improved and other metrics regressed.
- `inconclusive`: the input does not contain enough comparable evidence.

`verdict` uses Pareto-style aggregation. Pareto-superior means better in one or
more metrics and not worse in any metric.

## Failure Handling

- If samples are insufficient, treat it as a CLI error and collect more benchmark runs.
- If benchmark sets differ in `benchstat`, treat it as a CLI error and use `verdict -a` and `-b` with raw files.
- Treat parse and scanner errors as hard input errors, not as benchmark verdicts.
