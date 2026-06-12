# CLI Details

This guide explains `verdict` command output, thresholds, and result meanings. See [Workflow Details](README_WORKFLOWS.md) for complete comparison examples.

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

`verdict hotspot <package>` has its own text and JSON output. Hotspot JSON includes `schema_version`, `classification`, and a stable `reason` field so tools do not need to parse human text.

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

## Threshold Options

Use `--alpha` and `--min-delta` to control which changes count as improved or worsened:

```sh
benchstat old.txt new.txt | verdict --alpha 0.05 --min-delta 2.0
```

With this command, a metric must have `p <= 0.05` and at least `2.0%` change to count as improved or worsened.

For raw benchmark input, p-values are approximate and become more trustworthy as sample counts rise.

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

The exit code does not encode the outcome. For CI gates that must fail on `old-wins` or `trade-off`, parse the `--format json` output or use the library as shown in [Library Usage](README_LIB.md).

## Related Documentation

- [go-verdict](README.md): installation and complete CLI help.
- [Workflow Details](README_WORKFLOWS.md): comparison examples and raw-sample guidance.
- [Library Usage](README_LIB.md): use the same evaluator from Go code.
