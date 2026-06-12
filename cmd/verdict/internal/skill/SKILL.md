---
name: verdict
description: Use for Go performance requests, including "optimize this Go code", when measured evidence should guide benchmark creation, hotspot discovery, before/after or A/B comparison, and keep/reject decisions, even if benchmarks must be bootstrapped first.
license: MIT
metadata:
  author: KEINOS and The go-verdict Contributors
  version: "1.1.0"
---

# Verdict

`verdict` turns Go benchmark results into one objective decision: `new-wins`, `old-wins`, `tie`, `trade-off`, or `inconclusive`. Use it as the keep/reject gate in every Go optimization loop, even when benchmarks are missing; bootstrap representative evidence first, then compare and decide.

If the commands are missing, install them first: `go install github.com/KEINOS/go-verdict/cmd/verdict@latest` and `go install golang.org/x/perf/cmd/benchstat@latest`.

Detailed guidance lives in the CLI, not in this skill. Fetch one topic only when you reach that step:

- `verdict help bootstrap`: no benchmark exists yet; create representative evidence first.
- `verdict help hotspot`: find which function to optimize.
- `verdict help benchstat`: judge a before/after edit of one implementation.
- `verdict help gotestbench`: judge two implementations compared in one run.
- `verdict help results`: interpret outcomes, the Pareto rule, and inconclusive results.

## Loop

1. Check benchmark readiness: benchmarks must cover the user-visible workflow with realistic inputs. If missing or too narrow, bootstrap one before writing candidate code (`verdict help bootstrap`).
2. Unsure where to start? Run `verdict hotspot ./pkg` to get the first function to inspect (`verdict help hotspot`).
3. Capture the baseline, then write the candidate while preserving the original implementation.
4. Run correctness tests first. Reject semantic regressions and unaccepted behavior caveats before measuring.
5. Measure with `-benchmem` and judge with exactly one matching `verdict` command (table below).
6. Decide from the verdict lines, check for benchmark overfitting, and report.

## Choose One Command

| Evidence shape | Command |
| --- | --- |
| One implementation edited over time | `benchstat old.txt new.txt \| verdict` |
| Two implementations in one run, as `/original` and `/enhanced` sub-benchmarks | `go test -run='^$' -bench=BenchmarkX -benchmem -count=10 ./pkg \| verdict` |
| Two existing raw files with different benchmark names | `verdict -a old.txt -b new.txt` |
| No target chosen yet | `verdict hotspot ./pkg` (finds targets; never proves a win) |

Use `-count=3` to `-count=5` for cheap exploration. Use `-count=10` or more for final decisions, noisy results, and after any inconclusive result. Decide from one chosen evidence path; if two `verdict` commands disagree, report no decision and explain the mismatch.

## Hard Rules

- Correctness gates come before verdict. `verdict` never proves behavior; keep semantic claims scoped to the tests actually run.
- Claim a win, regression, tie, or pass only after the exact `verdict` command ran. Otherwise report no measured verdict and no decision.
- Never decide from raw `benchstat` output, geomean, or manual reading. If `benchstat old.txt new.txt` succeeds, immediately pipe it to `verdict`.
- Keep only when every relevant verdict line favors the candidate. Any `old wins`, `tie`, or `trade-off` line means reject or revise, unless the user explicitly accepts each trade-off.
- Benchmark labels must mean real implementations: the baseline label must run the actual pre-change code. If both labels call the same edited function, fix the benchmark shape and recapture (`verdict help gotestbench`).
- If benchmark names, sub-benchmarks, inputs, or labels changed between captures, recapture both sides (`verdict help benchstat`).
- Insufficient-sample and benchmark-set-mismatch errors mean no decision; fix the setup or collect more samples, then rerun (`verdict help results`).
- Reject candidates that win by exploiting benchmark shape, flags, labels, fixture names, or unrepresentative benchmarks (overfitting).
- Before reporting, verify the working tree still contains the candidate that produced the measured result.

## Final Report Format

Use four lines:

```text
Verdict: <benchmark-or-file result from verdict>
Command: <exact command or pipeline that produced the verdict>
Decision: <keep, reject, revise, or no decision>
Caveat: <none, benchmark-readiness concern, or brief caveat>
```

If the command is still planned, or only raw `benchstat` ran, report `Verdict: No measured verdict available` and `Decision: No decision yet.` Recommend measurement with `verdict`, not only `go test -bench`.
