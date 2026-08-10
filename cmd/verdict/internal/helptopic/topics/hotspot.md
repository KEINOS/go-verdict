# Hotspot: Find The Function To Optimize

`verdict hotspot` profiles the package benchmarks, reads the package sources, and suggests the first function to inspect.

## Usage

```sh
verdict hotspot ./your/package
verdict hotspot --bench BenchmarkParse --benchtime 500ms ./your/package
verdict hotspot --format json ./your/package
```

Options: `--bench regexp` (default `.`), `--benchtime duration|Nx` (default `1s`), `--count n` (default `1`), `--top n` (default `3`), `--format text|json` (default `text`), and `--fast`.

Run the command from inside the target Go module. The package argument resolves like `go list`, so a path such as `./internal/parser` must exist relative to the current directory.

By default the benchmarks run twice: once for the CPU profile and once for the memory profile. Collecting both in one run needs `-test.memprofilerate=1`, which inflates CPU samples along allocation paths and skews the ranking. `--fast` collapses the two passes into one, halves the run time, and says in the caveat that the CPU ranking is approximate.

## Signals

Five signals score every function. A signal counts when the function reaches its threshold.

- `cpu`: time spent in the function.
- `alloc-bytes`: bytes allocated. Large buffers dominate this one.
- `alloc-objects`: number of allocations. Many small objects dominate this one, and they are what pressures the garbage collector.
- `retained`: heap still live when the run ends. Only meaningful when the benchmark keeps state, such as a cache.
- `complexity`: cyclomatic and cognitive complexity of the source. This is a static estimate of where to look, never a measurement of cost.

The three memory views come from the same memory profile, so they cost no extra benchmark time.

## Reading The Result

The suggestion line names one function, its source position, and its classification:

- `hot-and-complex`: measured cost and complex source. Usually the best first target, because the cost is real and the code has room to change.
- `cpu-hotspot`: the function dominates CPU time.
- `alloc-hotspot`: the function dominates allocated bytes.
- `alloc-rate-hotspot`: the function dominates the allocation count. Look for many small allocations in a loop.
- `retention-hotspot`: the function dominates the live heap after the run.
- `mixed-hotspot`: the function dominates both CPU and memory.
- `complexity-hotspot`: no measured signal qualified, so this is a static estimate. Add a benchmark to replace the guess with evidence.
- `no-benchmark`: no benchmark workload ran and no function was complex enough to suggest. Bootstrap a benchmark: `verdict help bootstrap`.
- `no-clear-hotspot`: nothing passed any threshold. The cost may be spread out or live in the runtime.

`flat` is the cost of the function itself. `cum` includes everything it calls. A high `cum` with a low `flat` marks a dispatcher or umbrella function: inspect its call tree instead of optimizing the function itself.

## Ranking

Candidates are ranked by Pareto comparison across the five signals. A candidate stays in front while no other candidate matches or beats it on every signal, so a function that is hot in several ways outranks one that is hot in only one. Ties inside the front are settled by measured cost first, then by how many signals qualified, then by the strongest single signal.

`Also:` lists the runners-up. Use `--top n` to change how many candidates the report names.

## Limits

- Hotspot finds inspection targets. It never proves a speedup and never makes a keep/reject decision.
- Complexity is a heuristic. A complex function is worth reading; it is not proof that the function is slow. Never claim a win from a complexity score.
- Do not fix the optimization target solely from one `inspect <function>` suggestion; check the call tree first.
- Results depend on the benchmark workload. A narrow benchmark produces a narrow, possibly misleading hotspot.

## Next

After optimizing a candidate, judge it with measured evidence: `verdict help benchstat`.
