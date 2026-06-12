# Before/After: Judge One Edited Implementation

Use this topic when one implementation is edited over time: capture old, apply the change, capture new, judge.

## Commands

```sh
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=10 ./your/package > old.txt
# apply the candidate change, then:
go test -run='^$' -bench=BenchmarkMyFunc -benchmem -count=10 ./your/package > new.txt
benchstat old.txt new.txt | verdict
```

The verdict line is the decision gate, for example `BenchmarkMyFunc: new wins`.

## Rules

- Capture both files from the same package path with the same benchmark names, sub-benchmarks, and inputs. If the shape changes, recapture both sides.
- Never decide from raw `benchstat` output, geomean, or manual reading. If `benchstat old.txt new.txt` succeeds, immediately pipe it to `verdict`.
- Run correctness tests before measuring. Reject semantic regressions regardless of the verdict.
- Use `-count=3` to `-count=5` for cheap exploration. Use `-count=10` or more for final decisions, close or noisy results, and after any inconclusive result.
- Keep the baseline real: `old.txt` must come from the actual pre-change implementation, not from an unreported intermediate candidate.
- Do not move old code into a side directory to benchmark it; compare the same package path across time.

## Troubleshooting

- `benchmark names differ`: the two files contain different benchmark functions. Fix the names, or compare true A/B files with `verdict -a old.txt -b new.txt`.
- `missing p-value`: the benchstat input has no usable statistics; rerun with more `-count` samples.
- Interpreting `tie`, `trade-off`, or `inconclusive`: see `verdict help results`.

## Next

Read the outcome and decide: `verdict help results`.
