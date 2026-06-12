# go-verdict

Objective benchmark decisions for Go optimization loops.

Use `verdict` when CI, scripts, or AI agents need a repeatable keep-or-reject gate from Go benchmark evidence.

- Compare benchmark results before and after a change:

    ```shellsession
    % benchstat old.txt new.txt | verdict
    MyHeavyFunc-10: tie
    ```

- Compare two alternatives in raw benchmark output:

    ```shellsession
    % go test -run='^$' -bench=BenchmarkMyHeavyFunc -benchmem -count=8 ./your/package | verdict
    BenchmarkMyHeavyFunc: enhanced wins
    ```

- Not sure where to start? Let the benchmark profiles point at the first function to inspect:

    ```shellsession
    % verdict hotspot ./your/package
    ./your/package: inspect example.com/yourmod/yourpkg.CountASCIIWords (cpu-hotspot; cpu flat 85.7%, cpu cum 90.5%)
    Next: Optimize a candidate, then judge before/after benchmark results with verdict.
    ```

`verdict` helps you answer key questions after changing code:

- Did the candidate clear the benchmark gate?
- Is the result a regression, tie, or trade-off?
- Is the difference large enough to matter?
- Is another implementation better?

Useful for terminal workflows, CI checks, scripts, and automated optimization loops.

## Contents

- [Features](#features)
- [Usage](#usage)
- [Requirements](#requirements)
- [Installation](#installation)
- [AI Agent Skill](#ai-agent-skill)
- [Documentation](#documentation)
- [License](#license)

## Features

- Supports modern `benchstat` CSV-style output.
- Supports text `benchstat` output that includes `p=`.
- Auto-detects raw `go test -bench` output for local alternative comparison.
- Compares two raw benchmark files with `-a` and `-b`.
- Prints one concise verdict line by default, with details available via `--verbose`.
- Uses both statistical significance and a practical delta threshold.
- Handles lower-is-better metrics such as `sec/op`, `ns/op`, `B/op`, and `allocs/op`.
- Handles higher-is-better metrics such as `MB/s`, `ops/s`, and other `/s` rates.
- Flags mixed results as `trade-off` when one metric improves while another regresses.
- Returns stable outcomes for CI, scripts, and AI agents.
- Can require specific outcomes for exit-code gates.
- Explains each workflow step on demand with `verdict help <topic>`.

## Usage

```shellsession
% verdict --help
Objective benchmark decisions for Go optimization loops.

Usage:
  verdict [command] [options]

Note:
  Raw benchmark comparisons need at least 3 samples per benchmark side.
  For stable results, run benchmarks with -count=10 or more.

Commands:
  help [topic]
      Print workflow help. Topics: bootstrap, hotspot, benchstat, gotestbench, results.
  hotspot <package>
      Suggest the first function to inspect from benchmark CPU and allocation profiles.
  skill
      Print the AI Agent skill text.
  version
      Print the command version.

Options:
  --format text|json
      Output format. Default: text.
  -v, --version
      Print the command version.
  --mode auto|benchstat|gotestbench
      Input mode. Default: auto.
      auto: detect benchstat output or raw go test -bench output.
      benchstat: read already-compared benchstat text or CSV.
      gotestbench: compare raw go test -bench sub-benchmarks, such as original vs enhanced.
  --verbose
      Include verdict reason and metric details in text output.
  --require outcomes
      Require comma-separated outcomes for exit 0, such as new-wins or new-wins,tie.
  -a file
      Raw benchmark file for side A.
  -b file
      Raw benchmark file for side B.
  --baseline name
      Baseline sub-benchmark name for gotestbench mode.
  --candidate name
      Candidate sub-benchmark name for gotestbench mode.
  --alpha value
      P-value threshold for statistical significance. Must be greater than 0 and at most 1. Default: 0.05.
  --min-delta value
      Minimum absolute delta percentage to treat as a practical difference. Must be non-negative. Default: 2.0.

Hotspot options:
  --bench regexp
      Benchmark regexp for verdict hotspot. Default: .
  --benchtime duration|Nx
      Benchmark time or iteration count for verdict hotspot. Default: 1s.
  --count n
      Benchmark run count for verdict hotspot. Default: 1.
  --format text|json
      Output format for verdict hotspot. Default: text.
```

## Requirements

- Go 1.26.3 or later, as declared in `go.mod`.
- Recommended:
  - [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) for before/after comparison.

    Install `benchstat` if you do not already have it:

    ```sh
    go install golang.org/x/perf/cmd/benchstat@latest
    ```

## Installation

Install the `verdict` command with:

```sh
go install github.com/KEINOS/go-verdict/cmd/verdict@latest
```

Or build it from a local clone:

```sh
git clone https://github.com/KEINOS/go-verdict.git
cd go-verdict
make build
```

## AI Agent Skill

`verdict skill` prints guidance for agents that use `verdict` as an objective benchmark gate in Go optimization loops. The same command also serves CI and script automation.

The skill is intentionally compact. It keeps decision rules in context and routes detailed mechanics to `verdict help <topic>`, so agents with small context windows fetch guidance only when needed.

Export the canonical guidance into the directory your agent loads skills from. For example, for Claude Code:

```sh
mkdir -p .claude/skills/verdict
verdict skill > .claude/skills/verdict/SKILL.md
```

The canonical source is [cmd/verdict/internal/skill/SKILL.md](cmd/verdict/internal/skill/SKILL.md).

## Documentation

- [CLI Details](README_CLI.md): output formats, thresholds, verdicts, and inconclusive results.
- [Workflow Details](README_WORKFLOWS.md): Alternative, Named File, and Before/After comparison examples.
- [Library Usage](README_LIB.md): use the parser and evaluator from Go code.
- [Contributing](CONTRIBUTING.md): development workflow, architecture, and project layout.

## License

MIT License. See [LICENSE](LICENSE).
