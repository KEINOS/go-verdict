# go-verdict

Turn Go benchmark results into a winner, tie, or trade-off.

Use `verdict` when you need an objective keep-or-discard decision for your code enhancements.

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

`verdict` helps you answer key questions after changing code:

- Is it faster?
- Is it slower?
- Is the difference just noise?
- Is there a trade-off between metrics?
- Is another function better?

Useful for local development, CI checks, scripts, and automated optimization loops.

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
- Returns stable outcomes for CI and scripts.

## Usage

```shellsession
% verdict --help
Turn Go benchmark results into a winner, tie, or trade-off.

Usage:
  verdict [command] [options]

Note:
  Raw benchmark comparisons need at least 3 samples per benchmark side.
  For stable results, run benchmarks with -count=10 or more.

Commands:
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
      Minimum absolute delta percentage to treat as a practical difference. Must be non-negative. Default: 0.0.
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

`verdict skill` prints guidance for agents that use `verdict` as an objective benchmark gate in Go optimization loops. This is optional; the core command is the same developer-facing benchmark decision tool.

Export the canonical guidance with:

```sh
verdict skill > SKILL.md
```

The canonical source is [cmd/verdict/internal/skill/SKILL.md](cmd/verdict/internal/skill/SKILL.md).

## Documentation

- [CLI Details](README_CLI.md): output formats, thresholds, verdicts, and inconclusive results.
- [Workflow Details](README_WORKFLOWS.md): Alternative, Named File, and Before/After comparison examples.
- [Library Usage](README_LIB.md): use the parser and evaluator from Go code.
- [Contributing](CONTRIBUTING.md): development workflow, architecture, and project layout.

## License

MIT License. See [LICENSE](LICENSE).
