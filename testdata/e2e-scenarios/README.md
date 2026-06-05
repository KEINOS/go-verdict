# E2E Test Scenarios

This directory contains E2E test scenarios for the `go-verdict` project.

Each YAML file represents a specific test scenario. The Go test harness runs the built `verdict` binary from the repository root, so relative argument paths and `stdin_file` paths are repository-root relative.

Run all scenarios through the Makefile:

```sh
make e2e
```

To call the Go test harness directly, build first and pass the `e2e` build tag plus the required environment variables:

```sh
make build data
VERDICT_BIN="$(pwd)/dist/verdict" \
VERDICT_E2E_SCENARIOS_DIR="$(pwd)/testdata/e2e-scenarios" \
go test -tags=e2e -race ./tools/e2e/...
```

## YAML Formats

```yaml
name: <test scenario name>
timeout: <timeout duration, e.g., "30s">

cases:
  - name: <test case name>
    args: ["<command-line arguments for verdict>"]
    stdin: "<inline stdin text>"
    stdin_file: "<repository-root-relative fixture path>"
    want:
      exit_code: <expected exit code, e.g., 0>
      stdout:
        contains:
          - "<expected output part match>"
          - "<another expected output part match>"
        equals: "<expected exact output>"
      stderr:
        contains:
          - "<expected error output part match>"
        equals: "<expected exact error output>"

  - name: help
    args: ["-h"]
    want:
      exit_code: 0
      stdout:
        contains:
          - "Turn Go benchmark results into a winner, tie, or trade-off."
      stderr:
        equals: ""
```

Use either `stdin` or `stdin_file` in a case, not both. Prefer `stdin_file` for benchmark fixtures so large generated inputs stay outside YAML files.
