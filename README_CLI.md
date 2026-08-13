# CLI Details

This guide explains `verdict` command output, thresholds, exit gates, and result meanings. See [Workflow Details](README_WORKFLOWS.md) for complete comparison examples.

## Help Topics

`verdict help` lists the built-in workflow topics, and `verdict help <topic>` prints one of them. Topics: `bootstrap`, `hotspot`, `benchstat`, `gotestbench`, and `results`. The topics carry the same guidance as the README files, so terminals, scripts, and AI agents can read it without leaving the CLI.

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

`verdict hotspot <package>` has its own text and JSON output. Hotspot JSON includes `schema_version`, a stable `classification`, a `signals` array naming every signal that qualified, and a `candidates` array with the runners-up, so tools do not need to parse human text. Each signal reports its `unit`, `flat`, `cum`, `flat_pct`, and `cum_pct`.

When `--complexity` or `--complexity-config` is present, verbose text and JSON add source-complexity details for each verdict. A mapped benchmark has `status: compared`, both source measurements, the normalized scores, and a direction. An unmapped benchmark has the auxiliary status `not-mapped` and keeps its benchmark-only outcome.

Compact text keeps the same format and shows only the final enriched outcome.

Example JSON:

```json
{
  "verdicts": [
    {
      "baseline_label": "old.txt",
      "benchmark": "ExampleFast-10",
      "candidate_label": "new.txt",
      "reason": "no statistically significant practical difference",
      "outcome": "tie",
      "metrics": [
        {
          "benchmark": "ExampleFast-10",
          "metric": "sec/op",
          "direction": "same",
          "delta_pct": 0,
          "p_value": 0.68,
          "significant": false
        }
      ]
    }
  ]
}
```

## Optional Source Complexity

Benchmark reports do not identify the Go functions that produced each side. Complexity is therefore opt-in and requires an exact mapping. A `go.mod` by itself does not enable this feature.

Use one inline JSON object per benchmark:

```sh
benchstat old.txt new.txt | verdict --complexity '{
  "benchmark": "ExampleFast-10",
  "baseline": {
    "kind": "git",
    "ref": "HEAD~1",
    "file": "pkg/fast.go",
    "symbol": "example.com/project/pkg.Fast"
  },
  "candidate": {
    "kind": "worktree",
    "file": "pkg/fast.go",
    "symbol": "example.com/project/pkg.Fast"
  }
}'
```

For multiple benchmarks, use a versioned config:

```json
{
  "version": 1,
  "benchmarks": [
    {
      "benchmark": "ExampleFast-10",
      "baseline": {
        "kind": "git",
        "ref": "HEAD~1",
        "file": "pkg/fast.go",
        "symbol": "example.com/project/pkg.Fast"
      },
      "candidate": {
        "kind": "worktree",
        "file": "pkg/fast.go",
        "symbol": "example.com/project/pkg.Fast"
      }
    }
  ]
}
```

Then run:

```sh
benchstat old.txt new.txt | verdict --complexity-config complexity.json
```

Source kinds are:

| Kind | Root |
| --- | --- |
| `worktree` | The nearest ancestor module containing `go.mod`. |
| `git` | The same module at the required `ref`, read as Git blobs without checkout. |
| `directory` | The required `root`, resolved from the current directory. The root must contain `go.mod`. |

The `file` is relative to the selected module. The `symbol` is the exact package-qualified function name, such as `example.com/project/pkg.Fast` or `example.com/project/pkg.(*Worker).Run`.

Inline mappings replace config mappings with the same benchmark. Duplicate benchmark names inside the config or among inline flags are errors. A mapping whose benchmark is absent from the parsed report is also an error.

Complexity uses `max(cyclomatic/10, cognitive/15)`. Lower is better. It joins the benchmark metrics under the same Pareto rule. It can turn a benchmark `tie` into a win or turn a benchmark win into a `trade-off`, but it never turns `inconclusive` benchmark evidence into a decisive outcome.

## Threshold Options

Use `--alpha` and `--min-delta` to control which changes count as improved or worsened:

```sh
benchstat old.txt new.txt | verdict --alpha 0.05 --min-delta 2.0
```

With this command, a metric must have `p <= 0.05` and at least `2.0%` change to count as improved or worsened.

The default practical threshold is `--min-delta 2.0`, so smaller statistically significant changes are treated as `tie`. Lower it only when very small changes are meaningful for your workload.

For raw benchmark input, p-values are approximate and become more trustworthy as sample counts rise.

When many benchmarks and metrics are compared at once, false positives become more likely. Treat mixed or surprising outcomes as a reason to rerun targeted benchmarks, increase sample counts, or raise `--min-delta`.

## Verdicts

`verdict` uses Pareto-style rules for each benchmark. In this guide, Pareto-superior means better in one or more metrics and not worse in any metric.

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
| `missing-baseline` | Raw go test -bench input did not find the baseline sub-benchmark. |
| `missing-candidate` | Raw go test -bench input did not find the candidate sub-benchmark. |
| `insufficient-samples` | Raw comparison found too few repeated samples. |
| `unsupported-metric` | Raw comparison did not find supported metrics to compare. |
| `malformed-benchmark` | Raw comparison could not parse the benchmark rows. |
| `ambiguous-labels` | Raw go test -bench input could not select one baseline/candidate pair from labels. |
| `ambiguous-benchmark` | A raw file contains more than one benchmark series. |

## Exit Status

| Exit code | Meaning |
| --- | --- |
| `0` | A verdict was produced, including `tie`, `trade-off`, and reported `inconclusive` outcomes. |
| `1` | An error stopped the verdict: invalid flags, unreadable or empty input, benchmark-set mismatch, or insufficient raw samples. |

Use `--require` when the exit code should encode allowed outcomes:

```sh
benchstat old.txt new.txt | verdict --require new-wins
```

With `--require new-wins`, the report is still written, but the command exits `1` unless every verdict is `new-wins`. Use comma-separated outcomes such as `--require new-wins,tie` when more than one outcome should pass.

When source complexity is requested, `--require` checks the final enriched outcomes.

## Related Documentation

- [go-verdict](README.md): installation and complete CLI help.
- [Workflow Details](README_WORKFLOWS.md): comparison examples and raw-sample guidance.
- [Library Usage](README_LIB.md): use the same evaluator from Go code.
