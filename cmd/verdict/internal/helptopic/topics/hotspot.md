# Hotspot: Find The Function To Optimize

`verdict hotspot` runs the package benchmarks with CPU and allocation profiling, then suggests the first function to inspect.

## Usage

```sh
verdict hotspot ./your/package
verdict hotspot --bench BenchmarkParse --benchtime 500ms ./your/package
verdict hotspot --format json ./your/package
```

Options: `--bench regexp` (default `.`), `--benchtime duration|Nx` (default `1s`), `--count n` (default `1`), and `--format text|json` (default `text`).

Run the command from inside the target Go module. The package argument resolves like `go list`, so a path such as `./internal/parser` must exist relative to the current directory.

## Reading The Result

The suggestion line names one function and its classification:

- `cpu-hotspot`: the function dominates CPU time.
- `alloc-hotspot`: the function dominates allocated bytes.
- `mixed-hotspot`: the function dominates both; usually the best first target.
- `no-benchmark`: no benchmark workload ran. Bootstrap a benchmark first: `verdict help bootstrap`.
- `no-clear-hotspot`: no user-code function passed the profile thresholds; the cost may be spread out or live in the runtime.

`flat` is time or memory spent in the function itself. `cum` includes everything it calls. A high `cum` with a low `flat` marks a dispatcher or umbrella function: inspect its call tree instead of optimizing the function itself.

## Limits

- Hotspot finds inspection targets. It never proves a speedup and never makes a keep/reject decision.
- Do not fix the optimization target solely from one `inspect <function>` suggestion; check the call tree first.
- Results depend on the benchmark workload. A narrow benchmark produces a narrow, possibly misleading hotspot.

## Next

After optimizing a candidate, judge it with measured evidence: `verdict help benchstat`.
