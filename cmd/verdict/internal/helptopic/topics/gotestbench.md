# Same-Run A/B: Judge Two Implementations In One Run

Use this topic when one benchmark parent runs two real implementations as sub-benchmarks, such as `BenchmarkParse/original` and `BenchmarkParse/enhanced`.

## Commands

```sh
go test -run='^$' -bench=BenchmarkParse -benchmem -count=10 ./your/package | verdict
```

Auto mode prefers the labels `original` (baseline) and `enhanced` (candidate). It can also infer the pair when exactly two labels share one parent. For other labels, select them explicitly:

```sh
go test -run='^$' -bench=BenchmarkParse -benchmem -count=10 ./your/package | verdict --mode gotestbench --baseline base --candidate fast
```

## Rules

- Each label must call a real, distinct implementation. If both labels call the same edited function, the comparison proves nothing; use before/after files instead: `verdict help benchstat`.
- The baseline label must be the actual pre-change implementation, or an explicitly accepted previous candidate.
- Raw comparisons need at least 3 samples per side. Use `-count=10` or more for final decisions.
- Run correctness tests for the candidate before measuring.

## Raw File A/B

For two existing raw benchmark files whose benchmark names differ, skip benchstat and compare them directly:

```sh
verdict -a fileA.txt -b fileB.txt
```

Use this only for existing files, differing names, or data that cannot validly pass through `benchstat old.txt new.txt | verdict`.

## Next

Read the outcome and decide: `verdict help results`.
