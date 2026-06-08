---
name: verdict
description: Use when an AI agent needs a benchmark-driven Go optimization loop, an objective keep-or-reject gate for Go performance candidates, a concise verdict over Go benchmark results, or an A/B comparison of raw Go benchmark files.
license: MIT
metadata:
  author: KEINOS and The go-verdict Contributors
  version: "1.0.0"
---

# Verdict

Use this skill to run a benchmark-driven optimization loop for Go code. `verdict` turns Go benchmark results into a concise A/B decision, so use it as the objective keep-or-reject gate for performance candidates.

Do not keep a candidate only because it looks faster in one raw run. Keep it only when every relevant `verdict` result favors the candidate side (`new-wins`, `<candidate> wins`, or the new file wins), or when the user explicitly accepts each `trade-off`. Reject or revisit it if any relevant result favors the old side, is a `tie`, or is `inconclusive`.

## Fast Path

When optimizing Go code:

1. Keep the original behavior available as `original`, `baseline`, or an old benchmark file.
2. Add the candidate behavior as `enhanced`, `candidate`, or a new benchmark file.
3. Run correctness tests against the candidate behavior first; reject semantic regressions before benchmarking unless the user changes the requirements.
4. Run repeated benchmarks with `-benchmem`; use at least 3 samples per side and prefer `-count=10` or more.
5. Send the benchmark comparison to `verdict` and use the actual `verdict` output as the decision.
6. Keep, reject, or revise the candidate based on the `verdict` outcome and the user's stated trade-offs.
7. Report only the verdict line, benchmark command, keep/reject decision, and a short caveat when needed.

For raw sub-benchmark comparisons, use `original` and `enhanced` as the default labels. For names such as `baseline` and `candidate`, include explicit labels in the first verdict command.

Only use a raw sub-benchmark comparison when both implementations are distinct call paths in the same benchmark run. If `original` and `enhanced` both call the same public function, that benchmark is not an A/B comparison after you edit the function; use before/after benchmark files instead, or create separate original and candidate helpers.

If you notice that the raw sub-benchmark setup is not a valid A/B comparison, do not present the raw `go test ... | verdict` pipeline as the planned verdict command. Show the before/after file workflow instead, or first show the benchmark/helper changes needed to make the raw pipeline valid.

If you did not run `verdict`, say that no measured verdict is available. Do not present an expected or likely result as `new-wins`, `old-wins`, `tie`, `trade-off`, or `inconclusive`, and do not make a keep/reject decision.

If you did not run tests or benchmarks, label commands as planned commands. Do not write that tests passed, correctness is preserved, benchmarks improved, a function produced expected output, or a benchmark should show a specific allocation or latency result unless you actually ran the check.

## Choose the Workflow

Use raw sub-benchmarks when the original and candidate can live in the same benchmark:

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=10 ./your/package | verdict
```

`verdict` reads benchmark comparison data from stdin in this workflow. Do not invent flags such as `verdict -bench` or positional arguments such as `verdict original enhanced`.

Use explicit labels when the sub-benchmark names are not `original` and `enhanced`:

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=10 ./your/package | verdict --mode gotestbench --baseline baseline --candidate candidate
```

Use before/after benchmark files when comparing a code change across two revisions:

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=10 ./your/package > old.txt
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=10 ./your/package > new.txt
benchstat old.txt new.txt | verdict
```

In this workflow, `verdict` may name the winning file, such as `new.txt wins` or `old.txt wins`. Treat the candidate as kept only when the new benchmark file wins, or when the user accepts a reported trade-off.

Use raw file A/B comparison when the benchmark function names differ:

```sh
verdict -a fast.txt -b slow.txt
```

## Raw Go Test Bench Input

- Auto mode compares `original` and `enhanced` by default.
- Use `--mode gotestbench --baseline <name> --candidate <name>` for non-default labels, unless each parent benchmark has exactly two unambiguous labels.
- Use at least 3 samples per side for raw benchmark comparisons.
- Prefer `-count=10` or more for stable raw benchmark decisions.

## Interpret Results

- `new-wins`: keep the candidate unless there is a non-performance reason to reject it.
- `old-wins`: reject the candidate or try another approach.
- `tie`: reject the candidate for pure performance tasks. Keep it only when the user already asked for readability, safety, or another non-performance goal.
- `trade-off`: reject or revise unless the user explicitly accepts the regression side in this task; do not infer acceptance from a large speedup.
- `inconclusive`: collect more samples, fix the benchmark setup, or report that no decision can be made.

`verdict` uses Pareto-style aggregation. Pareto-superior means better in one or more metrics and not worse in any metric.

When `verdict` prints multiple benchmark lines, decide per line first. Ignore only lines unrelated to the candidate change; otherwise overall keep requires every relevant line to favor the candidate side, except user-accepted trade-offs.

## Final Report Format

Use this four-line final report format. Do not add benchmark tables or raw logs unless the user asks for details:

```text
Verdict: <benchmark-or-file result from verdict>
Command: go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=10 ./your/package | verdict
Decision: <keep, reject, revise, or no decision>
Caveat: <none or brief caveat>
```

If the command is only planned, report `Verdict: No measured verdict available` and `Decision: No decision yet.`

When you used `verdict`, the reported command must include the full pipeline or comparison command that produced the verdict.

When you recommend the next benchmark command, include the `verdict` pipeline or A/B comparison command, not only the raw `go test -bench` command. For an edited single public function, report the before/after `benchstat old.txt new.txt | verdict` workflow unless you first create true same-run A/B helpers.

Use `--verbose` only when the reason or metric details matter. Use `--format json` when another tool will consume the result.

## Failure Handling

- If samples are insufficient, collect more benchmark runs before deciding.
- If benchmark sets differ in `benchstat`, use `verdict -a` and `-b` with raw files.
- Treat parse and scanner errors as hard input errors, not benchmark verdicts.
