---
name: verdict
description: Use when an AI agent needs a benchmark-driven Go optimization loop, an objective keep-or-reject gate for Go performance candidates, a concise verdict over Go benchmark results, or an A/B comparison of raw Go benchmark files.
license: MIT
metadata:
  author: KEINOS and The go-verdict Contributors
  version: "1.0.0"
---

# Verdict

Use `verdict` to build benchmark evidence and decide keep/reject for Go performance work. `verdict` does not optimize code; it tells whether measured benchmark data favors original or candidate behavior.

Core rule: keep a performance candidate only when every relevant `verdict` line favors the candidate (`new-wins`, `<candidate> wins`, or `new.txt wins`), or when the user explicitly accepts each `trade-off`. Reject or revise when any relevant line favors old, is `tie`, or is `inconclusive`.

Supported commands:

- `verdict hotspot ./pkg`: find benchmark-covered code to inspect.
- `go test ... | verdict`: compare original and candidate paths in one run.
- `benchstat old.txt new.txt | verdict`: compare before/after edits.
- `verdict -a old.txt -b new.txt`: compare raw benchmark files.

## Evidence Rules

- Final before/after gate is `benchstat old.txt new.txt | verdict`, never raw `benchstat`, geomean, or manual judgment.
- Benchmark names, sub-benchmarks, inputs, and labels must match between `old.txt` and `new.txt`. If benchmark shape changes, recapture both sides.
- Do not claim verdict, pass, win, regression, allocation, or latency unless the exact reported command ran on current files.
- If `benchstat old.txt new.txt` succeeds, immediately pipe it to `verdict`.
- Before reporting, verify the tree contains the candidate that produced `new.txt`, not a temporary baseline or wrapper.
- If `verdict` did not run, report no measured verdict and no keep/reject decision.
- If `benchstat` reports mismatch or parsing fails, fix names/input before deciding.

## Benchmark Readiness

Before candidate code, ensure benchmarks cover intended workflow, realistic input, and useful call paths. If benchmarks are missing or narrow, bootstrap from tests, fixtures, README examples, CLI behavior, or E2E workflows.

Prefer public entrypoints, real input shapes, and per-scenario sub-benchmarks. Avoid convenience or trivial paths such as version output unless requested.

For CLI tools, benchmark the public runner with realistic stdin, args, and writers. If hotspot points at a dispatcher, inspect its call tree; optimize the dispatcher only if it is the real bottleneck.

Ask the maintainer only when workflow cannot be inferred or representative benchmarking needs out-of-scope behavior changes.

## Workflow Matrix

Hotspot, for inspection only:

```sh
verdict hotspot ./your/package
```

Hotspot does not prove speed or keep/reject. If no workload runs, add a representative benchmark first. Do not special-case benchmark args from `inspect <umbrella function>` alone.

Same-run A/B, for distinct paths in one benchmark:

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=3 ./your/package | verdict
```

Sub-benchmarks must share one parent, such as `BenchmarkMyFunc/original` and `/enhanced`. For custom labels:

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=3 ./your/package | verdict --mode gotestbench --baseline baseline --candidate candidate
```

Separate benchmark functions are usually not enough. If both labels call the same edited public function, use before/after files or create separate helpers.

Before/after, for editing one implementation:

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=3 ./your/package > old.txt
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=3 ./your/package > new.txt
benchstat old.txt new.txt | verdict
```

Use `-count=3` for first measured verdict. Increase to `-count=10+` only for noisy, inconclusive, or keep-worthy results needing stronger confidence.

Raw file A/B, for existing files or differing names:

```sh
verdict -a fast.txt -b slow.txt
```

## Optimization Loop

1. Check benchmark readiness; bootstrap if needed.
2. Pick hotspot, same-run A/B, before/after, or raw file comparison.
3. Preserve original as `original`, `baseline`, or `old.txt`.
4. Add candidate as `enhanced`, `candidate`, or `new.txt`.
5. Run correctness tests; reject semantic regressions.
6. Run benchmarks with `-benchmem`; compare with `verdict`; decide from verdict output.
7. Run overfitting check; verify tree contains candidate.
8. Report verdict line, command, decision, caveat.

## Interpret Results

- `new-wins`: keep candidate unless non-performance issue rejects it.
- `old-wins`: reject or try another candidate.
- `tie`: reject for pure performance work.
- `trade-off`: reject/revise unless user explicitly accepts regression.
- `inconclusive`: collect more samples or fix setup.

`verdict` uses Pareto aggregation: better in one or more metrics and worse in none. Keep only when every relevant line favors candidate, except accepted trade-offs. Exclude irrelevant sub-benchmarks before capture; do not dismiss `old.txt wins` after seeing it unless user accepts exclusion.

## Overfitting Check

Reject candidates that win by exploiting harness details:

- Special-casing benchmark flags, labels, fixture names, inputs, or arg order.
- Skipping required behavior only for benchmark shape.
- Optimizing unrepresentative benchmarks while real workflows stay unmeasured.

A verdict win is not enough when benchmark is too narrow or candidate is tailored to the harness.

## Final Report Format

Use four lines. No tables or raw logs unless asked:

```text
Verdict: <benchmark-or-file result from verdict>
Command: <exact command or pipeline that produced verdict>
Decision: <keep, reject, revise, or no decision>
Caveat: <none, benchmark-readiness concern, or brief caveat>
```

If command is planned, or if only raw `benchstat old.txt new.txt` ran, report `Verdict: No measured verdict available` and `Decision: No decision yet.`

When recommending next measurement, include the `verdict` pipeline or A/B command, not only `go test -bench`. Use `--verbose` for needed detail and `--format json` for tool consumption.
