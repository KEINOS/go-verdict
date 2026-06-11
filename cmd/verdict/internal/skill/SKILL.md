---
name: verdict
description: Use for Go performance requests, including "optimize this Go code", when measured evidence should guide benchmark creation, hotspot discovery, before/after or A/B comparison, and keep/reject decisions, even if benchmarks must be bootstrapped first.
license: MIT
metadata:
  author: KEINOS and The go-verdict Contributors
  version: "1.0.0"
---

# Verdict

Use `verdict` command to guide Go optimization with benchmark evidence and keep/reject decisions. Use this skill even when benchmarks are missing; first bootstrap representative evidence, then compare results and decide whether a candidate survives.

Keep only when every relevant `verdict` line favors candidate (`new-wins`, `<candidate> wins`, `new.txt wins`), or user accepts each `trade-off`. Reject/revise when any line favors old, is `tie`, or is `inconclusive`.

Correctness gates come before verdict. `verdict` does not prove behavior. If candidate replaces parser/library semantics or public behavior, preserve behavior or add edge tests. When tests are only representative, report semantic confidence as scoped, not exhaustive.

## Capabilities

- `verdict hotspot ./pkg`: find benchmark-covered code.
- `go test ... | verdict`: judge same-run A/B.
- `benchstat old.txt new.txt | verdict`: judge before/after.
- `verdict -a old.txt -b new.txt`: escape hatch for raw benchmark files.

## Evidence Rules

- Use command matching evidence shape; do not replace before/after with raw A/B when `benchstat old.txt new.txt` works.
- Decide from one chosen evidence path. If multiple `verdict` commands disagree, report revise/no decision and explain the mismatch.
- Before/after gate is `benchstat old.txt new.txt | verdict`, never raw `benchstat`, geomean, or manual judgment.
- Keep benchmark names, sub-benchmarks, inputs, and labels stable between `old.txt` and `new.txt`. If shape changes, recapture both.
- Labels named `original`, `baseline`, `enhanced`, or `candidate` must mean real implementations. If they only repeat the edited public function, fix the benchmark shape before baseline capture; do not hide the issue by filtering one misleading label.
- Capture before/after from same package path; do not move old code to `bench-old`.
- Claim verdict, pass, win, regression, allocation, or latency only after exact command ran.
- If `benchstat old.txt new.txt` succeeds, immediately pipe it to `verdict`.
- Before report, verify tree still contains candidate that produced `new.txt`.
- Do not keep candidate with unaccepted semantic caveat, even when `verdict` favors it.
- Keep semantic claims scoped to the tests or probes actually run.
- If `verdict` did not run, report no measured verdict and no keep/reject decision.
- Some inconclusive states are CLI errors, such as insufficient samples or benchmark-set mismatch. Treat those as no measured verdict and no decision; fix the setup or collect more samples, then rerun.
- If `benchstat` mismatch or parse fails, fix names/input before deciding.

## Benchmark Readiness

Before candidate code, ensure benchmarks cover the user-visible workflow, realistic inputs, and the path being optimized. If benchmarks are missing or too narrow, do not stop; bootstrap one from tests, fixtures, README examples, CLI behavior, or E2E workflows before writing candidate code.

Prefer public entrypoints, real input, and per-scenario sub-benchmarks. Avoid trivial or convenient paths unless the user asked for that exact path.

For CLI tools, benchmark public runner with realistic stdin, args, and writers. If hotspot points at dispatcher, inspect call tree.

Ask maintainer only when workflow cannot be inferred or benchmark needs out-of-scope behavior change.

## Choose Command

Use hotspot only to find inspection targets:

```sh
verdict hotspot ./your/package
```

Hotspot does not prove speed or keep/reject. If no workload runs, add benchmark first. Do not fix the optimization target solely from a hotspot `inspect <function>` suggestion; inspect the call tree first, especially for dispatchers or umbrella functions.

Use same-run A/B only when one benchmark parent runs distinct implementations. Exploration example:

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=3 ./your/package | verdict
```

Sub-benchmarks must share one parent, such as `BenchmarkMyFunc/original` and `/enhanced`. Use `gotestbench` labels for different names:

`original` or baseline must be the actual pre-change implementation, or an explicitly accepted previous candidate. Do not label an unreported intermediate candidate as `original`.

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=3 ./your/package | verdict --mode gotestbench --baseline baseline --candidate candidate
```

Separate benchmark functions are usually not enough. If both labels call same edited function, use before/after files or separate helpers.

Use before/after for one implementation edited across time. Exploration example:

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=3 ./your/package > old.txt
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=3 ./your/package > new.txt
benchstat old.txt new.txt | verdict
```

If old/new files contain `/original` and `/enhanced` but both call edited function, they are not same-run A/B proof. Rename them to scenario labels or remove the split before capture; if already captured, recapture both sides.

For AI-agent exploration, first use `-count=3` to `-count=5` to avoid expensive false starts. For final keep/reject decisions, insufficient-sample errors, close/noisy results, inconclusive results, or mixed trade-offs, run `-count=10` or more.

Use raw file A/B only for existing files, differing names, or data that cannot validly pass through `benchstat old.txt new.txt | verdict`:

```sh
verdict -a fast.txt -b slow.txt
```

## Minimal Loop

1. Check benchmark readiness and benchmark labels/call paths; bootstrap or fix misleading benchmark shape before baseline capture.
2. Choose command by evidence shape.
3. Preserve original and add candidate.
4. Run correctness tests; reject semantic regressions and unaccepted caveats.
5. Run `-benchmem`; compare with `verdict`; decide from verdict lines.
6. Check overfitting and verify tree contains candidate.
7. Report verdict line, command, decision, caveat.

## Results

- `new-wins`: keep candidate unless non-performance issue rejects it.
- `old-wins`: reject or try another candidate.
- `tie`: reject for pure performance work.
- `trade-off`: reject/revise unless user accepts regression.
- `inconclusive`: collect more samples or fix setup.

`verdict` uses Pareto aggregation: better in one or more metrics and worse in none. Keep only when every relevant line favors candidate, except accepted trade-offs. Mixed output is a measured result: report revise/reject instead of continuing silently or averaging wins. Exclude irrelevant sub-benchmarks before capture; do not dismiss `old.txt wins` after seeing it.

## Overfitting

Reject candidates that win by exploiting harness details:

- Special-casing benchmark flags, labels, fixture names, inputs, or arg order.
- Skipping required behavior only for benchmark shape.
- Optimizing unrepresentative benchmarks while real workflows stay unmeasured.

A verdict win is not enough when benchmark is narrow or candidate is harness-tailored.

## Final Report Format

Use four lines:

```text
Verdict: <benchmark-or-file result from verdict>
Command: <exact command or pipeline that produced verdict>
Decision: <keep, reject, revise, or no decision>
Caveat: <none, benchmark-readiness concern, or brief caveat>
```

If command is planned, or only raw `benchstat old.txt new.txt` ran, report `Verdict: No measured verdict available` and `Decision: No decision yet.`

Recommend measurement with `verdict`, not only `go test -bench`.
