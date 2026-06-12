# Results: Read Verdict Output And Decide

## Outcomes

| Outcome | Meaning | Action |
| --- | --- | --- |
| `new-wins` | The candidate improves at least one metric and regresses none. | Keep, unless a non-performance issue rejects it. |
| `old-wins` | The candidate regresses with no improvement. | Reject or try another candidate. |
| `tie` | No metric changed enough to matter. | Reject for pure performance work. |
| `trade-off` | One metric improved while another regressed. | Reject or revise unless the user accepts the regression. |
| `inconclusive` | Not enough comparable data. | Fix the setup or collect more samples, then rerun. |

## Pareto Rule

`verdict` aggregates per-metric directions with a Pareto rule: a side wins only when it is better in at least one metric and worse in none. "Pareto-superior" means exactly that: no metric got worse.

A metric counts only when it is statistically significant (`p <= alpha`, default `0.05`) and practically different (`abs(delta_pct) >= min-delta`, default `2.0`). Lower is better for `sec/op`, `ns/op`, `B/op`, and `allocs/op`. Higher is better for `MB/s`, `GB/s`, and `ops/s`.

Mixed output across benchmarks is a measured result: report reject or revise instead of averaging wins or ignoring a losing line.

Many benchmarks and metrics increase false-positive risk. If a result is mixed, surprising, or close, rerun targeted benchmarks with more samples or a stricter `--min-delta`.

## CLI Errors That Mean "No Decision"

Some inconclusive states are reported as CLI errors. Treat them as no measured verdict and no keep/reject decision:

- `insufficient samples`: collect at least 3 samples per side; rerun with `-count=10` or more.
- `benchmark names differ`: the inputs compare different benchmark functions; fix names or use `verdict -a` and `-b`.

## Details On Demand

- `--verbose` adds the reason and per-metric details to text output.
- `--format json` emits machine-readable verdicts with `outcome`, `reason_code`, and per-metric rows for scripted loops.
- `--require new-wins` writes the report and exits non-zero unless every verdict is `new-wins`.
- `--alpha` and `--min-delta` tune the significance and practical-difference thresholds.
