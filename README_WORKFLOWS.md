# Workflow Details

`verdict` supports one hotspot Scout workflow and three comparison Judge workflows: Alternative, Named File, and Before/After.

Auto mode is the default input mode. It detects whether stdin contains `benchstat` output or raw `go test -bench` output, then selects the matching comparison workflow when the input is unambiguous.

The same guidance is available from the CLI without opening this file: `verdict help` lists the workflow topics, and `verdict help <topic>` prints one of them, such as `verdict help benchstat`.

## Hotspot Scout

Use this workflow before choosing what to optimize.

```sh
verdict hotspot ./your/package
```

The Scout command runs existing benchmarks for one package, collects CPU and allocation profiles in a temporary directory, and suggests the first user-code function to inspect. It does not decide whether a code change is faster. After changing code, use one of the Judge workflows below to compare before and after benchmark results.

If no benchmark workload runs, `verdict hotspot` explains that state and exits without changing user code. Add a `BenchmarkXxx` function or pass `--bench` so the workload covers the code you want to inspect. `verdict help bootstrap` explains how to create a representative benchmark when none exists.

## Alternative Comparison

Use this workflow for a quick local test before actually changing code.

Use it when the original and alternative implementations are sub-benchmarks in the same test file.

Example benchmark:

```go
func BenchmarkMyHeavyFunc(b *testing.B) {
    b.Run("original", func(b *testing.B) {
        for b.Loop() {
            ExampleOriginal()
        }
    })

    b.Run("enhanced", func(b *testing.B) {
        for b.Loop() {
            ExampleEnhanced()
        }
    })
}
```

Collect repeated benchmark samples:

```sh
go test -run='^$' -bench='BenchmarkMyHeavyFunc' -benchmem -count=20 ./your/package > gotestbench.txt
```

Then compare the two sub-benchmarks:

```sh
verdict < gotestbench.txt
```

Auto mode first checks whether both `original` and `enhanced` exist under the same parent benchmark. If both exist, it compares that pair.

Otherwise, auto mode compares one pair only when the parent benchmark has exactly two sub-benchmark labels.

Use explicit labels when the [raw benchmark output](https://go.googlesource.com/proposal/+/master/design/14313-benchmark-format.md) has more than two alternatives or non-default names:

```sh
verdict --mode gotestbench --baseline original --candidate enhanced < gotestbench.txt
```

The outcome meaning is from the new option side:

| Outcome | Meaning in alternative mode |
| --- | --- |
| `new-wins` | The candidate is better than the baseline. |
| `old-wins` | The baseline is better than the candidate. |
| `tie` | There is no statistically significant practical difference. |
| `trade-off` | Some metrics improved, but other metrics regressed. |
| `inconclusive` | The input does not contain enough comparable samples. |

Raw benchmark comparison supports `ns/op`, `B/op`, and `allocs/op` from `go test` output. It computes deltas and approximate `p`-values from repeated samples, then applies the same `--alpha`, `--min-delta`, and verdict rules as before/after comparison. It needs at least 3 samples per benchmark side; with fewer samples, the library report is `inconclusive` and the CLI prints an actionable error.

Run with enough samples. `-count=8` is accepted for quick local checks, but `-count=10` or more is recommended for stable results. If you use only `-count=2`, `verdict` asks you to run more samples:

```text
error: insufficient samples: need at least 3 samples per benchmark side; recommend -count=10 or more for stable results
```

The raw-sample `p`-value is a pragmatic normal approximation from the two sample means and variances. Very small accepted sample counts, such as 3 to 5 per side, can be useful for quick feedback but are weaker evidence than the recommended `-count=10` or more. Benchmark distributions can be noisy or non-normal, so treat close results as guidance to collect more samples.

## Named File Comparison

Use this workflow when two benchmark functions have different names, but they do the same job.

Each file should contain one benchmark series, collected with repeated samples:

```sh
go test -run='^$' -bench=BenchmarkMyHeavyFuncFast -count=10 ./your/package > fast.txt
go test -run='^$' -bench=BenchmarkMyHeavyFuncSlow -count=10 ./your/package > slow.txt
verdict -a fast.txt -b slow.txt
```

If `fast.txt` contains `BenchmarkMyHeavyFuncFast` and `slow.txt` contains `BenchmarkMyHeavyFuncSlow`, the benchmark names are different. `benchstat` compares the same benchmark name before and after a change, so `verdict` errors.

For example, this is not a valid before/after `benchstat` comparison:

```shellsession
$ benchstat fast.txt slow.txt | verdict
error: inconclusive: benchmark names differ
benchstat compares the same benchmark before and after a change.
To compare two different benchmark functions as A/B alternatives, pass the raw benchmark files:
  verdict -a fast.txt -b slow.txt
```

In that case, use `-a` and `-b` to say: "compare these two raw benchmark files as A/B alternatives."

```shellsession
$ verdict -a fast.txt -b slow.txt
BenchmarkMyHeavyFuncFast_vs_BenchmarkMyHeavyFuncSlow: BenchmarkMyHeavyFuncFast wins
```

## Before/After Comparison

Use this workflow when you compare benchmark results from two different code states, such as before and after edits.

Collect old benchmark results:

```sh
go test -run='^$' -bench=. -benchmem -count=20 > old.txt
```

Change your code or checkout a target branch, then collect new benchmark results:

```sh
go test -run='^$' -bench=. -benchmem -count=20 > new.txt
```

Compare them with `benchstat`, then pipe the result to `verdict`:

```sh
benchstat old.txt new.txt | verdict
```

If one side wins, `verdict` uses the file labels from the `benchstat` header:

```text
ExampleFast-10: new.txt wins
```

> [!INFO]
> `verdict` parses the [Go benchmark format](https://go.googlesource.com/proposal/+/master/design/14313-benchmark-format.md) and `benchstat` output. Therefore, you can use it with any tool that produces compatible output.
> For example, you can also compare the speed of the Go compiler using [compilebench](https://pkg.go.dev/golang.org/x/tools/cmd/compilebench) and [toolstash](https://pkg.go.dev/golang.org/x/tools/cmd/toolstash).
>
> ```sh
> compilebench -count 10 -compile $(toolstash -n compile) >old.txt
> compilebench -count 10 >new.txt
> benchstat old.txt new.txt | verdict
> ```
